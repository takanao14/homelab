package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// All controller failures use fixed codes shared with the runtime protocol.
func controlFail(code string) error { return fail(code) }

type controllerArgs struct {
	command, target, revision, branch, jobID, enabled string
	publish, detach, follow, keepRunning              bool
	after                                             int64
}

func parseControllerArgs(args []string) (controllerArgs, error) {
	result := controllerArgs{}
	if len(args) == 0 {
		return result, controlFail("invalid_arguments")
	}
	result.command = args[0]
	positionals := []string{}
	seen := map[string]bool{}
	for index := 1; index < len(args); index++ {
		value := args[index]
		if !strings.HasPrefix(value, "--") {
			positionals = append(positionals, value)
			continue
		}
		flag, argument, hasValue := strings.Cut(value, "=")
		if seen[flag] {
			return result, controlFail("invalid_arguments")
		}
		seen[flag] = true
		takesValue := flag == "--revision" || flag == "--branch" || flag == "--after"
		if takesValue && !hasValue {
			index++
			if index >= len(args) {
				return result, controlFail("invalid_arguments")
			}
			argument = args[index]
		}
		if !takesValue && hasValue {
			return result, controlFail("invalid_arguments")
		}
		switch flag {
		case "--revision":
			if result.command != "run" {
				return result, controlFail("invalid_arguments")
			}
			result.revision = argument
		case "--branch":
			if result.command != "run" {
				return result, controlFail("invalid_arguments")
			}
			result.branch = argument
		case "--publish":
			if result.command != "run" {
				return result, controlFail("invalid_arguments")
			}
			result.publish = true
		case "--detach":
			if result.command != "run" && result.command != "publish-retry" {
				return result, controlFail("invalid_arguments")
			}
			result.detach = true
		case "--follow":
			if result.command != "logs" {
				return result, controlFail("invalid_arguments")
			}
			result.follow = true
		case "--after":
			if result.command != "logs" {
				return result, controlFail("invalid_arguments")
			}
			number, err := strconv.ParseInt(argument, 10, 64)
			if err != nil || number < 0 {
				return result, controlFail("invalid_log_cursor")
			}
			result.after = number
		case "--keep-running":
			if result.command != "start" {
				return result, controlFail("invalid_arguments")
			}
			result.keepRunning = true
		default:
			return result, controlFail("invalid_arguments")
		}
	}
	switch result.command {
	case "run":
		if len(positionals) != 1 || seen["--revision"] == seen["--branch"] {
			return result, controlFail("invalid_arguments")
		}
		result.target = positionals[0]
		if !targetPattern.MatchString(result.target) {
			return result, controlFail("invalid_target")
		}
		if seen["--revision"] && !revisionPattern.MatchString(result.revision) {
			return result, controlFail("invalid_revision")
		}
		if seen["--branch"] && !controllerValidBranch(result.branch) {
			return result, controlFail("invalid_branch")
		}
	case "logs", "publish-retry":
		if len(positionals) != 1 {
			return result, controlFail("invalid_arguments")
		}
		result.jobID = positionals[0]
		if !jobPattern.MatchString(result.jobID) {
			return result, controlFail("invalid_job_id")
		}
	case "status":
		if len(positionals) > 1 {
			return result, controlFail("invalid_arguments")
		}
		if len(positionals) == 1 {
			result.jobID = positionals[0]
			if !jobPattern.MatchString(result.jobID) {
				return result, controlFail("invalid_job_id")
			}
		}
	case "maintenance":
		if len(positionals) != 1 || (positionals[0] != "on" && positionals[0] != "off") {
			return result, controlFail("invalid_arguments")
		}
		result.enabled = positionals[0]
	case "start", "shutdown":
		if len(positionals) != 0 {
			return result, controlFail("invalid_arguments")
		}
	default:
		return result, controlFail("invalid_arguments")
	}
	return result, nil
}

// HTTP responses and SSH output are decoded and projected onto the public protocol.
func controllerDecode(raw []byte) (map[string]any, error) {
	var data map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil || data == nil {
		return nil, controlFail("invalid_builder_response")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, controlFail("invalid_builder_response")
	}
	return data, nil
}

type controllerSSH func(context.Context, []string, []string, []byte) ([]byte, int, error)
type imageController struct {
	env                                map[string]string
	url, node, token, host, user, vmid string
	client                             *http.Client
	ssh                                controllerSSH
	now                                func() time.Time
	sleep                              func(time.Duration)
	out                                io.Writer
}

func newImageController(environ map[string]string, out io.Writer) (*imageController, error) {
	environment := map[string]string{}
	for key, value := range environ {
		environment[key] = value
	}
	get := func(key, fallback string) string {
		if value, exists := environment[key]; exists {
			return value
		}
		return fallback
	}
	control := &imageController{env: environment, url: strings.TrimRight(get("IMAGEBUILDER_PROXMOX_URL", "https://192.168.10.10:8006"), "/"), node: get("IMAGEBUILDER_NODE", "pve"), token: get("IMAGEBUILDER_PROXMOX_TOKEN", ""), host: get("IMAGEBUILDER_SSH_HOST", "192.168.20.25"), user: get("IMAGEBUILDER_SSH_USER", "imagebuilder-control"), vmid: get("IMAGEBUILDER_VMID", ""), out: out, now: time.Now, sleep: time.Sleep, ssh: controllerSSHCommand}
	for _, rule := range []struct{ value, pattern, code string }{
		{control.url, `^https://[a-zA-Z0-9.-]+(?::[0-9]+)?$`, "invalid_proxmox_url"},
		{control.node, `^[a-zA-Z0-9-]+$`, "invalid_node"},
		{control.host, `^[A-Za-z0-9.-]+$`, "invalid_ssh_host"},
		{control.user, `^[a-z_][a-z0-9_-]*$`, "invalid_ssh_user"},
	} {
		if !regexp.MustCompile(rule.pattern).MatchString(rule.value) {
			return nil, controlFail(rule.code)
		}
	}
	if control.token == "" || strings.ContainsAny(control.token, "\n\r\x00") {
		return nil, controlFail("missing_proxmox_token")
	}
	if control.vmid != "" && !regexp.MustCompile(`^[1-9][0-9]{2,8}$`).MatchString(control.vmid) {
		return nil, controlFail("invalid_vmid")
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, controlFail("invalid_proxmox_ca")
	}
	if caPath, exists := environment["IMAGEBUILDER_PROXMOX_CA_FILE"]; exists {
		pem, err := os.ReadFile(caPath)
		if err != nil || !roots.AppendCertsFromPEM(pem) {
			return nil, controlFail("invalid_proxmox_ca")
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	control.client = &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(request *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	return control, nil
}

func controllerSSHCommand(ctx context.Context, args, environment []string, input []byte) ([]byte, int, error) {
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.Env = environment
	command.Stdin = bytes.NewReader(input)
	output := &controllerLimitedBuffer{limit: 262145}
	command.Stdout = output
	command.Stderr = io.Discard
	err := command.Run()
	if len(output.data) > 262144 {
		return nil, 1, controlFail("invalid_builder_response")
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return output.data, exit.ExitCode(), nil
		}
		return nil, 1, controlFail("builder_unreachable")
	}
	return output.data, 0, nil
}

type controllerLimitedBuffer struct {
	data  []byte
	limit int
}

func (b *controllerLimitedBuffer) Write(data []byte) (int, error) {
	size := len(data)
	if remaining := b.limit - len(b.data); remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		b.data = append(b.data, data...)
	}
	return size, nil
}

func (c *imageController) api(suffix string, post bool) (any, error) {
	method := http.MethodGet
	var body io.Reader
	if post {
		method = http.MethodPost
		body = strings.NewReader("")
	}
	request, err := http.NewRequest(method, c.url+"/api2/json/nodes/"+c.node+"/"+suffix, body)
	if err != nil {
		return nil, controlFail("proxmox_request_failed")
	}
	request.Header.Set("Authorization", "PVEAPIToken="+c.token)
	response, err := c.client.Do(request)
	if err != nil {
		return nil, controlFail("proxmox_request_failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, controlFail("proxmox_request_failed")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1048577))
	if err != nil {
		return nil, controlFail("proxmox_request_failed")
	}
	if len(raw) > 1048576 {
		return nil, controlFail("invalid_proxmox_response")
	}
	data, err := controllerDecode(raw)
	if err != nil {
		return nil, controlFail("invalid_proxmox_response")
	}
	value, exists := data["data"]
	if !exists {
		return nil, controlFail("invalid_proxmox_response")
	}
	return value, nil
}

func (c *imageController) power() (string, error) {
	if c.vmid == "" {
		data, err := c.api("qemu", false)
		if err != nil {
			return "", err
		}
		array, ok := data.([]any)
		if !ok {
			return "", controlFail("invalid_proxmox_response")
		}
		var matches []map[string]any
		for _, item := range array {
			if candidate, ok := item.(map[string]any); ok && candidate["name"] == "imagebuilder1" {
				matches = append(matches, candidate)
			}
		}
		if len(matches) != 1 {
			return "", controlFail("builder_not_found")
		}
		vmid, ok := controllerInteger(matches[0]["vmid"])
		if !ok || vmid < 100 || vmid > 999999999 {
			return "", controlFail("invalid_proxmox_response")
		}
		c.vmid = strconv.FormatInt(vmid, 10)
	}
	raw, err := c.api("qemu/"+c.vmid+"/status/current", false)
	if err != nil {
		return "", err
	}
	data, ok := raw.(map[string]any)
	if !ok || data["name"] != "imagebuilder1" {
		return "", controlFail("unexpected_vm_identity")
	}
	status, ok := data["status"].(string)
	if !ok || (status != "running" && status != "stopped") {
		return "", controlFail("invalid_proxmox_response")
	}
	return status, nil
}

func (c *imageController) remote(request map[string]any) (map[string]any, error) {
	args := []string{"ssh", "-o", "BatchMode=yes", "-o", "ForwardAgent=no", "-o", "SendEnv=-*", "-o", "ConnectTimeout=10", "-o", "StrictHostKeyChecking=yes", "-T", c.user + "@" + c.host, "sudo -n /usr/local/libexec/imagebuilder control"}
	environment := []string{}
	for _, key := range []string{"HOME", "PATH", "SSH_AUTH_SOCK"} {
		if value, exists := c.env[key]; exists {
			environment = append(environment, key+"="+value)
		}
	}
	input, err := json.Marshal(request)
	if err != nil {
		return nil, controlFail("invalid_arguments")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	raw, status, err := c.ssh(ctx, args, environment, input)
	if err != nil {
		if err.Error() == "invalid_builder_response" {
			return nil, controlFail("invalid_builder_response")
		}
		return nil, controlFail("builder_unreachable")
	}
	if len(raw) > 262144 {
		return nil, controlFail("invalid_builder_response")
	}
	data, err := controllerDecode(raw)
	if err != nil {
		return nil, controlFail("builder_unreachable")
	}
	safe, err := controllerSafeResponse(data)
	if err != nil {
		return nil, err
	}
	if status != 0 {
		return nil, controlFail("builder_request_failed")
	}
	return safe, nil
}

func (c *imageController) wake(buildReady bool) (map[string]any, error) {
	deadline := c.now().Add(900 * time.Second)
	last := "builder_start_timeout"
	for c.now().Before(deadline) {
		power, err := c.power()
		if err != nil {
			return nil, err
		}
		if power == "stopped" {
			if _, err = c.api("qemu/"+c.vmid+"/status/start", true); err != nil {
				power, checkErr := c.power()
				if checkErr != nil {
					return nil, checkErr
				}
				if power != "running" {
					return nil, err
				}
			}
		}
		request := map[string]any{"op": "ready"}
		if !buildReady {
			request["build_ready"] = false
		}
		response, err := c.remote(request)
		if err == nil {
			return response, nil
		}
		last = err.Error()
		switch last {
		case "builder_unreachable", "internal_error", "stopping", "work_disk_not_mounted", "command_failed", "command_timeout":
		default:
			return nil, err
		}
		c.sleep(5 * time.Second)
	}
	return nil, controlFail(last)
}

func (c *imageController) secrets(publish bool) (map[string]string, error) {
	values := map[string]string{}
	for _, key := range []string{"PKR_VAR_user_password", "SEAWEEDFS_S3_ENDPOINT", "SEAWEEDFS_S3_ACCESS_KEY", "SEAWEEDFS_S3_SECRET_KEY", "SEAWEEDFS_CLOUD_IMAGES_BUCKET"} {
		if value := c.env[key]; value != "" {
			values[key] = value
		}
	}
	if publish {
		for _, key := range []string{"SEAWEEDFS_S3_ENDPOINT", "SEAWEEDFS_S3_ACCESS_KEY", "SEAWEEDFS_S3_SECRET_KEY"} {
			if values[key] == "" {
				return nil, controlFail("missing_publish_credentials")
			}
		}
	} else {
		for key := range values {
			if key != "PKR_VAR_user_password" {
				delete(values, key)
			}
		}
	}
	return values, nil
}

func (c *imageController) emit(value map[string]any) error {
	if err := json.NewEncoder(c.out).Encode(value); err != nil {
		return controlFail("controller_failed")
	}
	return nil
}

func (c *imageController) followLogs(identifier string, after int64) (map[string]any, error) {
	deadline := c.now().Add(14700 * time.Second)
	for c.now().Before(deadline) {
		page, err := c.remote(map[string]any{"op": "logs", "job_id": identifier, "after": after})
		if err != nil {
			return nil, err
		}
		if page["job_id"] != identifier {
			return nil, controlFail("unexpected_job_identity")
		}
		cursor, valid := controllerInteger(page["cursor"])
		hasMore, validMore := page["has_more"].(bool)
		if !valid || cursor < after || !validMore {
			return nil, controlFail("invalid_builder_response")
		}
		if truncated, _ := page["truncated"].(bool); truncated {
			if err := c.emit(map[string]any{"ok": true, "job_id": identifier, "truncated": true, "cursor": after}); err != nil {
				return nil, err
			}
		}
		records, _ := page["records"].([]any)
		for _, raw := range records {
			record, ok := raw.(map[string]any)
			if !ok {
				return nil, controlFail("invalid_builder_response")
			}
			sequence, valid := controllerInteger(record["sequence"])
			if !valid || sequence <= after {
				return nil, controlFail("invalid_builder_response")
			}
			result := map[string]any{"job_id": identifier}
			for key, value := range record {
				result[key] = value
			}
			if err := c.emit(result); err != nil {
				return nil, err
			}
			after = sequence
		}
		if cursor != after {
			return nil, controlFail("invalid_builder_response")
		}
		if hasMore {
			if len(records) == 0 {
				return nil, controlFail("invalid_builder_response")
			}
			continue
		}
		if finished, _ := page["finished"].(bool); finished {
			if !controllerTerminal(page["phase"]) {
				return nil, controlFail("invalid_builder_response")
			}
			result := map[string]any{}
			for _, key := range []string{"ok", "job_id", "phase", "code", "finished", "cursor", "branch", "revision", "publication_blocked"} {
				if value, exists := page[key]; exists {
					result[key] = value
				}
			}
			return result, nil
		}
		c.sleep(2 * time.Second)
	}
	return nil, controlFail("log_wait_timeout")
}

func (c *imageController) waitStopped() (map[string]any, error) {
	deadline := c.now().Add(900 * time.Second)
	for c.now().Before(deadline) {
		state, err := c.power()
		if err != nil {
			return nil, err
		}
		if state == "stopped" {
			return map[string]any{"ok": true, "power": "stopped"}, nil
		}
		c.sleep(5 * time.Second)
	}
	return nil, controlFail("shutdown_timeout")
}

func (c *imageController) execute(args controllerArgs) (map[string]any, error) {
	if args.command == "status" || args.command == "shutdown" {
		power, err := c.power()
		if err != nil {
			return nil, err
		}
		if power == "stopped" {
			return map[string]any{"ok": true, "power": "stopped"}, nil
		}
		request := map[string]any{"op": args.command}
		if args.jobID != "" {
			request["job_id"] = args.jobID
		}
		result, err := c.remote(request)
		if err != nil {
			return nil, err
		}
		if args.command == "status" {
			return result, nil
		}
		return c.waitStopped()
	}
	var request map[string]any
	if args.command == "run" || args.command == "publish-retry" {
		values, err := c.secrets(args.publish || args.command == "publish-retry")
		if err != nil {
			return nil, err
		}
		request = map[string]any{"credentials": values}
		if args.command == "run" {
			request["op"] = "submit"
			request["revision"] = args.revision
			request["target"] = args.target
			request["publish"] = args.publish
		} else {
			request["op"] = "publish-retry"
			request["job_id"] = args.jobID
		}
	}
	if _, err := c.wake(args.command == "run" || args.command == "publish-retry"); err != nil {
		return nil, err
	}
	switch args.command {
	case "start":
		if args.keepRunning {
			return c.remote(map[string]any{"op": "maintenance", "enabled": true})
		}
		return map[string]any{"ok": true, "power": "running"}, nil
	case "maintenance":
		return c.remote(map[string]any{"op": "maintenance", "enabled": args.enabled == "on"})
	case "logs":
		if args.follow {
			return c.followLogs(args.jobID, args.after)
		}
		return c.remote(map[string]any{"op": "logs", "job_id": args.jobID, "after": args.after})
	}
	if args.command == "run" && args.branch != "" {
		resolved, err := c.remote(map[string]any{"op": "resolve", "branch": args.branch})
		if err != nil {
			return nil, err
		}
		revision, ok := resolved["revision"].(string)
		if resolved["branch"] != args.branch || !ok || !revisionPattern.MatchString(revision) {
			return nil, controlFail("invalid_builder_response")
		}
		request["revision"] = revision
		request["branch"] = args.branch
		if err := c.emit(resolved); err != nil {
			return nil, err
		}
	}
	result, err := c.remote(request)
	if err != nil {
		return nil, err
	}
	identifier, valid := result["job_id"].(string)
	if !valid || !jobPattern.MatchString(identifier) {
		return nil, controlFail("invalid_builder_response")
	}
	if args.detach {
		return result, nil
	}
	if err := c.emit(result); err != nil {
		return nil, err
	}
	state, err := c.followLogs(identifier, 0)
	if err != nil {
		return nil, err
	}
	if state["phase"] != "succeeded" {
		if err := c.emit(state); err != nil {
			return nil, err
		}
		return nil, controlFail("job_failed")
	}
	return state, nil
}

// ControllerMain returns only fixed diagnostic errors; callers decide exit status.
func ControllerMain(args []string, environ map[string]string, out io.Writer) error {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			_, err := io.WriteString(out, "Usage: image-build.sh <command>\n  run <target> (--revision <sha> | --branch <name>) [--publish] [--detach]\n  status [job-id]\n  logs <job-id> [--follow] [--after <sequence>]\n  publish-retry <job-id> [--detach]\n  start [--keep-running]\n  maintenance <on|off>\n  shutdown\n")
			if err != nil {
				return fail("controller_failed")
			}
			return nil
		}
	}
	parsed, err := parseControllerArgs(args)
	if err != nil {
		return err
	}
	control, err := newImageController(environ, out)
	if err != nil {
		return err
	}
	result, err := control.execute(parsed)
	if err != nil {
		return err
	}
	return control.emit(result)
}

func controllerInteger(value any) (int64, bool) { return num(value) }
func controllerValidBranch(value string) bool   { _, err := validateBranch(value); return err == nil }
func controllerTerminal(value any) bool         { text, ok := value.(string); return ok && terminalSet[text] }

func controllerSafeResponse(data map[string]any) (map[string]any, error) {
	ok, valid := data["ok"].(bool)
	if !valid {
		return nil, controlFail("invalid_builder_response")
	}
	if !ok {
		code, valid := data["error"].(string)
		if !valid || !codeSet[code] {
			return nil, controlFail("invalid_builder_response")
		}
		return nil, controlFail(code)
	}
	result := map[string]any{"ok": true}
	for _, key := range []string{"maintenance", "publication_blocked", "publish", "finished", "truncated", "has_more"} {
		if raw, exists := data[key]; exists {
			value, valid := raw.(bool)
			if !valid {
				return nil, controlFail("invalid_builder_response")
			}
			result[key] = value
		}
	}
	if raw, exists := data["branch"]; exists {
		branch, err := validateBranch(raw)
		if err != nil {
			return nil, controlFail("invalid_builder_response")
		}
		result["branch"] = branch
	}
	for _, rule := range []struct{ key, pattern string }{
		{"job_id", `^[0-9a-f]{32}$`}, {"revision", `^(?:[0-9a-f]{40}|[0-9a-f]{64})$`},
		{"target", `^[a-z][a-z0-9-]{0,63}$`}, {"sha256", `^[0-9a-f]{64}$`},
	} {
		if raw, exists := data[rule.key]; exists {
			value, valid := raw.(string)
			if !valid || !regexp.MustCompile(rule.pattern).MatchString(value) {
				return nil, controlFail("invalid_builder_response")
			}
			result[rule.key] = value
		}
	}
	if raw, exists := data["phase"]; exists {
		value, valid := raw.(string)
		if !valid || (!phaseSet[value] && value != "idle" && value != "ready" && value != "stopping") {
			return nil, controlFail("invalid_builder_response")
		}
		result["phase"] = value
	}
	if raw, exists := data["code"]; exists {
		value, valid := raw.(string)
		if !valid || !codeSet[value] {
			return nil, controlFail("invalid_builder_response")
		}
		result["code"] = value
	}
	if raw, exists := data["created"]; exists {
		value, valid := controllerInteger(raw)
		if !valid {
			return nil, controlFail("invalid_builder_response")
		}
		result["created"] = value
	}
	if raw, exists := data["events"]; exists {
		events, valid := raw.([]any)
		if !valid || len(events) > 100 {
			return nil, controlFail("invalid_builder_response")
		}
		safeEvents := []any{}
		for _, item := range events {
			event, valid := item.(map[string]any)
			if !valid {
				return nil, controlFail("invalid_builder_response")
			}
			phase, phaseValid := event["phase"].(string)
			code, codeValid := event["code"].(string)
			stamp, stampValid := controllerInteger(event["time"])
			if !phaseValid || !phaseSet[phase] || !codeValid || !codeSet[code] || !stampValid {
				return nil, controlFail("invalid_builder_response")
			}
			safeEvents = append(safeEvents, map[string]any{"phase": phase, "code": code, "time": stamp})
		}
		result["events"] = safeEvents
	}
	if raw, exists := data["cursor"]; exists {
		value, valid := controllerInteger(raw)
		if !valid || value < 0 {
			return nil, controlFail("invalid_builder_response")
		}
		result["cursor"] = value
	}
	if raw, exists := data["records"]; exists {
		records, valid := raw.([]any)
		if !valid || len(records) > 256 {
			return nil, controlFail("invalid_builder_response")
		}
		projected := []any{}
		for _, item := range records {
			record, valid := item.(map[string]any)
			if !valid {
				return nil, controlFail("invalid_builder_response")
			}
			safe, err := validateLogRecord(record)
			if err != nil {
				return nil, controlFail("invalid_builder_response")
			}
			projected = append(projected, safe)
		}
		result["records"] = projected
	}
	return result, nil
}
