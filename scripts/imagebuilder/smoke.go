package main

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func workerSmokeConfiguration(id, image, public string) (Object, string, error) {
	if !validJobID(id) || !workerPublicKey.MatchString(public) {
		return nil, "", fail("invalid_request")
	}
	marker := "/var/tmp/imagebuilder-smoke-" + id
	user := Object{"name": "imagecheck", "shell": "/bin/sh", "ssh_authorized_keys": []string{public}}
	configuration := Object{"users": []Object{user}, "ssh_pwauth": false, "runcmd": []string{"touch " + marker}}
	check := "test -f " + marker
	// FreeBSD BASIC-CLOUDINIT uses nuageinit without sudo or Python cloud-init.
	if !strings.HasPrefix(filepath.Base(image), "freebsd-") {
		user["sudo"], user["lock_passwd"] = "ALL=(ALL) NOPASSWD:ALL", true
		check = "sudo -n cloud-init status --wait && systemctl is-active --quiet qemu-guest-agent && " + check
	}
	return configuration, check, nil
}

func (r *Runtime) workerSmoke(id, image string, env map[string]string) error {
	r.CurrentStep = "smoke-boot"
	if r.SmokeHook != nil {
		return r.SmokeHook(id, image, env)
	}
	directory := r.WorkerPath(id)
	overlay, seed := filepath.Join(r.JobDir(id), "smoke.qcow2"), filepath.Join(directory, "seed.img")
	agentSocket := filepath.Join(directory, "agent.sock")
	configuration, check, err := workerSmokeConfiguration(id, image, env["PKR_VAR_ssh_pubkey"])
	if err != nil {
		return err
	}
	data, err := json.Marshal(configuration)
	if err != nil || os.WriteFile(filepath.Join(directory, "user-data"), append([]byte("#cloud-config\n"), data...), 0600) != nil {
		return fail("invalid_state")
	}
	if err := writeJSON(filepath.Join(directory, "meta-data"), Object{"instance-id": id}, 0600); err != nil {
		return err
	}
	defer os.Remove(overlay)
	defer os.Remove(seed)
	// Sequential smoke boots reuse the job directory, including the Unix socket path.
	if err := os.Remove(agentSocket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail("smoke_boot_failed")
	}
	defer os.Remove(agentSocket)
	if _, err := r.Invoke([]string{"cloud-localds", seed, filepath.Join(directory, "user-data"), filepath.Join(directory, "meta-data")}, CommandOptions{Env: env}); err != nil {
		return err
	}
	if _, err := r.Invoke([]string{"qemu-img", "create", "-f", "qcow2", "-F", "qcow2", "-b", image, overlay}, CommandOptions{Env: env}); err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fail("smoke_boot_failed")
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()
	process := exec.Command(commandPath("qemu-system-x86_64", env), "-enable-kvm", "-cpu", "host", "-m", "2048", "-smp", "2",
		"-display", "none", "-serial", "none", "-monitor", "none",
		"-drive", "file="+overlay+",format=qcow2,if=virtio", "-drive", "file="+seed+",format=raw,media=cdrom,readonly=on",
		"-netdev", "user,id=net0,hostfwd=tcp:127.0.0.1:"+port+"-:22", "-device", "virtio-net-pci,netdev=net0",
		"-device", "virtio-serial-pci", "-chardev", "socket,path="+agentSocket+",server=on,wait=off,id=agent",
		"-device", "virtserialport,chardev=agent,name=org.qemu.guest_agent.0")
	process.Env = commandEnv(env)
	if err := process.Start(); err != nil {
		return fail("smoke_boot_failed")
	}
	done := make(chan error, 1)
	go func() { done <- process.Wait() }()
	defer func() {
		_ = process.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			_ = process.Process.Kill()
			<-done
		}
	}()
	deadline := time.Now().Add(workerSeconds(r.Cfg.SmokeTimeoutSeconds))
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			done <- err
			return fail("smoke_boot_failed")
		default:
		}
		_, err := r.Invoke([]string{"ssh", "-F", "/dev/null", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5",
			"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-i", filepath.Join(directory, "id_ed25519"),
			"-p", port, "imagecheck@127.0.0.1", check}, CommandOptions{Env: env, Timeout: 15 * time.Second})
		if err == nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fail("smoke_timeout")
}
