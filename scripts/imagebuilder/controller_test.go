package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const controllerTestJob = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const controllerTestRevision = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const controllerSecret = "SECRET-MARKER"

func testController(t *testing.T, handler http.HandlerFunc) (*imageController, *bytes.Buffer) {
	t.Helper()
	if handler == nil {
		handler = func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"data":{"name":"imagebuilder1","status":"running"}}`)
		}
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	output := &bytes.Buffer{}
	control, err := newImageController(map[string]string{"IMAGEBUILDER_PROXMOX_TOKEN": "test!power=" + controllerSecret, "IMAGEBUILDER_PROXMOX_URL": server.URL, "IMAGEBUILDER_PROXMOX_CA_FILE": ca, "IMAGEBUILDER_VMID": "100", "PKR_VAR_user_password": controllerSecret}, output)
	if err != nil {
		t.Fatal(err)
	}
	moment := time.Unix(1000, 0)
	control.now = func() time.Time { return moment }
	control.sleep = func(duration time.Duration) { moment = moment.Add(duration) }
	return control, output
}

func controllerJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
func controllerResult(value any) controllerSSH {
	return func(context.Context, []string, []string, []byte) ([]byte, int, error) {
		return controllerJSON(value), 0, nil
	}
}
func requireControllerError(t *testing.T, err error, expected string) {
	t.Helper()
	if err == nil || err.Error() != expected {
		t.Fatalf("expected fixed error %s, got %v", expected, err)
	}
}

func TestControllerArgsSourceAndOrdering(t *testing.T) {
	for _, arguments := range [][]string{
		{"run", "ubuntu26-base", "--revision", controllerTestRevision, "--publish", "--detach"},
		{"run", "--publish", "--revision=" + controllerTestRevision, "ubuntu26-base", "--detach"},
	} {
		parsed, err := parseControllerArgs(arguments)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.target != "ubuntu26-base" || parsed.revision != controllerTestRevision || !parsed.publish || !parsed.detach {
			t.Fatal("lost CLI flags")
		}
	}
	for _, arguments := range [][]string{
		{"run", "ubuntu26-base"}, {"run", "ubuntu26-base", "--revision", controllerTestRevision, "--branch", "main"},
		{"run", "ubuntu26-base", "--revision", controllerTestRevision, "--publish=" + controllerSecret},
		{"run", "ubuntu26-base", "--revision", controllerTestRevision, "--unknown", controllerSecret},
		{"shutdown", controllerSecret}, {"maintenance", "maybe"}, {"status", "--follow"},
	} {
		_, err := parseControllerArgs(arguments)
		requireControllerError(t, err, "invalid_arguments")
		if strings.Contains(err.Error(), controllerSecret) {
			t.Fatal("argument echoed")
		}
	}
}

func TestControllerInvalidInputsBeforeWake(t *testing.T) {
	cases := []struct {
		args []string
		code string
	}{
		{[]string{"run", "ubuntu26-base", "--revision", "main"}, "invalid_revision"},
		{[]string{"run", "ubuntu26-base", "--branch", "../main"}, "invalid_branch"},
		{[]string{"run", "ubuntu26-base", "--branch", "main\n" + controllerSecret}, "invalid_branch"},
		{[]string{"run", "-invalid", "--revision", controllerTestRevision}, "invalid_target"},
		{[]string{"logs", "invalid"}, "invalid_job_id"},
		{[]string{"logs", controllerTestJob, "--after", "-1"}, "invalid_log_cursor"},
		{[]string{"logs", controllerTestJob, "--after", "9223372036854775808"}, "invalid_log_cursor"},
	}
	for _, test := range cases {
		output := &bytes.Buffer{}
		err := ControllerMain(test.args, map[string]string{}, output)
		requireControllerError(t, err, test.code)
		if output.Len() != 0 {
			t.Fatal("invalid input produced output")
		}
	}
}

func TestControllerResponseProjection(t *testing.T) {
	raw := map[string]any{"ok": true, "phase": "idle", "credentials": controllerSecret, "records": []any{map[string]any{"sequence": int64(1), "time": int64(1), "step": "packer-init", "event": "started", "raw": controllerSecret}}}
	safe, err := controllerSafeResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(controllerJSON(safe)), controllerSecret) {
		t.Fatal("unapproved fields leaked")
	}
	for _, data := range []map[string]any{
		{"ok": false, "error": controllerSecret}, {"ok": true, "phase": controllerSecret},
		{"ok": true, "events": []any{map[string]any{"phase": "failed", "code": controllerSecret, "time": int64(1)}}},
		{"ok": true, "created": true}, {"ok": true, "cursor": -1}, {"ok": true, "revision": "main"},
		{"ok": true, "records": []any{map[string]any{"sequence": 1, "time": 1, "step": "checkout", "event": "started", "source": controllerSecret}}},
	} {
		_, err := controllerSafeResponse(data)
		requireControllerError(t, err, "invalid_builder_response")
	}
}

func TestControllerSSHSecretBoundary(t *testing.T) {
	control, output := testController(t, nil)
	control.env["HOME"] = "/nonsecret/home"
	control.env["PATH"] = "/usr/bin"
	control.env["SSH_AUTH_SOCK"] = "/tmp/auth.sock"
	control.env["UNRELATED_SECRET"] = controllerSecret
	var capturedInput []byte
	control.ssh = func(ctx context.Context, args, environment []string, input []byte) ([]byte, int, error) {
		if strings.Contains(strings.Join(args, " "), controllerSecret) || strings.Contains(strings.Join(environment, " "), controllerSecret) {
			t.Fatal("secret escaped stdin")
		}
		for _, option := range []string{"ForwardAgent=no", "SendEnv=-*", "StrictHostKeyChecking=yes", "BatchMode=yes"} {
			if !strings.Contains(strings.Join(args, " "), option) {
				t.Fatal("missing SSH isolation option")
			}
		}
		if len(environment) != 3 {
			t.Fatal("unexpected forwarded environment")
		}
		if _, exists := ctx.Deadline(); !exists {
			t.Fatal("SSH has no timeout")
		}
		capturedInput = input
		return []byte("not-json " + controllerSecret), 1, nil
	}
	_, err := control.remote(map[string]any{"op": "submit", "credentials": map[string]string{"PKR_VAR_user_password": controllerSecret}})
	requireControllerError(t, err, "builder_unreachable")
	if !bytes.Contains(capturedInput, []byte(controllerSecret)) {
		t.Fatal("credentials missing from stdin")
	}
	if strings.Contains(err.Error(), controllerSecret) || strings.Contains(output.String(), controllerSecret) {
		t.Fatal("secret diagnostics leaked")
	}
}

func TestControllerProxmoxIdentityAndRedirect(t *testing.T) {
	for _, status := range []string{"running", "stopped"} {
		calls := 0
		control, _ := testController(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "PVEAPIToken=test!power="+controllerSecret {
				t.Error("missing authorization")
			}
			calls++
			if strings.HasSuffix(r.URL.Path, "/qemu") {
				io.WriteString(w, `{"data":[{"name":"other","vmid":101},{"name":"imagebuilder1","vmid":100}]}`)
			} else {
				fmt.Fprintf(w, `{"data":{"name":"imagebuilder1","status":%q}}`, status)
			}
		})
		control.vmid = ""
		power, err := control.power()
		if err != nil || power != status || calls != 2 || control.vmid != "100" {
			t.Fatalf("resolve failed: %v", err)
		}
	}
	control, _ := testController(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":{"name":"another-vm","status":"running","secret":"SECRET-MARKER"}}`)
	})
	_, err := control.power()
	requireControllerError(t, err, "unexpected_vm_identity")
	received := false
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received = true; io.WriteString(w, `{"data":{}}`) }))
	defer target.Close()
	redirect, _ := testController(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	})
	_, err = redirect.api("qemu", false)
	requireControllerError(t, err, "proxmox_request_failed")
	if received {
		t.Fatal("redirect followed with credentials")
	}
}

func TestControllerProxmoxRejectsMalformedAndOversized(t *testing.T) {
	for _, raw := range []string{`{"data":{}} {"data":{}}`, `[]`, `{"missing":"SECRET-MARKER"}`, strings.Repeat("x", 1048577)} {
		control, _ := testController(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, raw) })
		_, err := control.api("qemu", false)
		requireControllerError(t, err, "invalid_proxmox_response")
	}
}

func TestControllerStatusStoppedDoesNotWake(t *testing.T) {
	control, _ := testController(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatal("status changed power")
		}
		io.WriteString(w, `{"data":{"name":"imagebuilder1","status":"stopped"}}`)
	})
	control.ssh = func(context.Context, []string, []string, []byte) ([]byte, int, error) {
		t.Fatal("stopped status opened SSH")
		return nil, 1, nil
	}
	parsed, _ := parseControllerArgs([]string{"status"})
	result, err := control.execute(parsed)
	if err != nil || result["power"] != "stopped" {
		t.Fatal("stopped status failed")
	}
}

func TestControllerWakeReadinessAndRace(t *testing.T) {
	starts, reads := 0, 0
	control, _ := testController(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/start") {
			starts++
			if r.Method != http.MethodPost {
				t.Fatal("start is not POST")
			}
			io.WriteString(w, `{"data":"task"}`)
			return
		}
		reads++
		status := "stopped"
		if reads > 1 {
			status = "running"
		}
		fmt.Fprintf(w, `{"data":{"name":"imagebuilder1","status":%q}}`, status)
	})
	attempts := 0
	control.ssh = func(_ context.Context, _ []string, _ []string, input []byte) ([]byte, int, error) {
		request, _ := controllerDecode(input)
		if request["op"] != "ready" {
			t.Fatal("missing readiness")
		}
		attempts++
		if attempts == 1 {
			return nil, 1, controlFail("builder_unreachable")
		}
		return controllerJSON(map[string]any{"ok": true, "phase": "ready"}), 0, nil
	}
	result, err := control.wake(true)
	if err != nil || result["phase"] != "ready" || starts != 1 || attempts != 2 {
		t.Fatalf("wake failed: %v", err)
	}
}

func TestControllerFatalReadinessDoesNotRetry(t *testing.T) {
	control, _ := testController(t, nil)
	calls := 0
	control.ssh = func(context.Context, []string, []string, []byte) ([]byte, int, error) {
		calls++
		return []byte(`{"ok":false,"error":"kvm_unavailable"}`), 1, nil
	}
	_, err := control.wake(true)
	requireControllerError(t, err, "kvm_unavailable")
	if calls != 1 {
		t.Fatal("fatal readiness retried")
	}
}

func TestControllerDiagnosticReadiness(t *testing.T) {
	for _, arguments := range [][]string{{"logs", controllerTestJob}, {"maintenance", "on"}, {"start"}} {
		control, _ := testController(t, nil)
		requests := []map[string]any{}
		control.ssh = func(_ context.Context, _ []string, _ []string, input []byte) ([]byte, int, error) {
			request, _ := controllerDecode(input)
			requests = append(requests, request)
			return []byte(`{"ok":true,"phase":"idle"}`), 0, nil
		}
		args, _ := parseControllerArgs(arguments)
		_, err := control.execute(args)
		if err != nil {
			t.Fatal(err)
		}
		if len(requests) == 0 || requests[0]["op"] != "ready" || requests[0]["build_ready"] != false {
			t.Fatal("diagnostic startup required healthy build environment")
		}
	}
}

func TestControllerBranchFreezesRevisionAndFiltersCredentials(t *testing.T) {
	control, output := testController(t, nil)
	control.env["SEAWEEDFS_S3_SECRET_KEY"] = controllerSecret
	requests := []map[string]any{}
	control.ssh = func(_ context.Context, _ []string, _ []string, input []byte) ([]byte, int, error) {
		request, _ := controllerDecode(input)
		requests = append(requests, request)
		switch request["op"] {
		case "ready":
			return []byte(`{"ok":true,"phase":"ready"}`), 0, nil
		case "resolve":
			return controllerJSON(map[string]any{"ok": true, "branch": "fix/packer", "revision": controllerTestRevision}), 0, nil
		default:
			return controllerJSON(map[string]any{"ok": true, "job_id": controllerTestJob, "phase": "preparing"}), 0, nil
		}
	}
	args, _ := parseControllerArgs([]string{"run", "ubuntu26-base", "--branch", "fix/packer", "--detach"})
	_, err := control.execute(args)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 || requests[1]["op"] != "resolve" || requests[2]["revision"] != controllerTestRevision || requests[2]["branch"] != "fix/packer" || requests[2]["publish"] != false {
		t.Fatal("branch not resolved once before submit")
	}
	credentials := requests[2]["credentials"].(map[string]any)
	if len(credentials) != 1 || credentials["PKR_VAR_user_password"] != controllerSecret {
		t.Fatal("unpublished build forwarded S3 credentials")
	}
	if !strings.Contains(output.String(), controllerTestRevision) || strings.Contains(output.String(), controllerSecret) {
		t.Fatal("source freeze or secret projection failed")
	}
}

func TestControllerBranchMismatchNeverSubmits(t *testing.T) {
	control, _ := testController(t, nil)
	submits := 0
	control.ssh = func(_ context.Context, _ []string, _ []string, input []byte) ([]byte, int, error) {
		request, _ := controllerDecode(input)
		if request["op"] == "submit" {
			submits++
		}
		if request["op"] == "resolve" {
			return controllerJSON(map[string]any{"ok": true, "branch": "other", "revision": controllerTestRevision}), 0, nil
		}
		return []byte(`{"ok":true}`), 0, nil
	}
	args, _ := parseControllerArgs([]string{"run", "ubuntu26-base", "--branch", "main"})
	_, err := control.execute(args)
	requireControllerError(t, err, "invalid_builder_response")
	if submits != 0 {
		t.Fatal("unresolved branch submitted")
	}
}

func TestControllerPublishCredentialsBeforeWake(t *testing.T) {
	control, _ := testController(t, func(http.ResponseWriter, *http.Request) { t.Fatal("missing secrets woke VM") })
	args, _ := parseControllerArgs([]string{"run", "ubuntu26-base", "--revision", controllerTestRevision, "--publish"})
	_, err := control.execute(args)
	requireControllerError(t, err, "missing_publish_credentials")
}

func TestControllerFollowDrainsPagesAndAwaitsFinish(t *testing.T) {
	control, output := testController(t, nil)
	after := []int64{}
	pageIndex := 0
	pages := []map[string]any{
		{"ok": true, "job_id": controllerTestJob, "phase": "succeeded", "finished": false, "cursor": int64(1), "has_more": true, "records": []any{map[string]any{"sequence": int64(1), "time": int64(1), "step": "packer-init", "event": "started"}}},
		{"ok": true, "job_id": controllerTestJob, "phase": "succeeded", "finished": false, "cursor": int64(2), "has_more": false, "records": []any{map[string]any{"sequence": int64(2), "time": int64(2), "step": "packer-init", "event": "completed", "duration_ms": int64(10)}}},
		{"ok": true, "job_id": controllerTestJob, "phase": "succeeded", "finished": true, "cursor": int64(2), "has_more": false, "records": []any{}},
	}
	control.ssh = func(_ context.Context, _ []string, _ []string, input []byte) ([]byte, int, error) {
		request, _ := controllerDecode(input)
		cursor, _ := controllerInteger(request["after"])
		after = append(after, cursor)
		if pageIndex >= len(pages) {
			t.Fatal("follow never finished")
		}
		raw := controllerJSON(pages[pageIndex])
		pageIndex++
		return raw, 0, nil
	}
	result, err := control.followLogs(controllerTestJob, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result["finished"] != true || !reflect.DeepEqual(after, []int64{0, 1, 2}) || len(strings.Split(strings.TrimSpace(output.String()), "\n")) != 2 {
		t.Fatal("follow finalization or pagination failed")
	}
}

func TestControllerFollowRetentionGapAndInvalidCursors(t *testing.T) {
	control, output := testController(t, nil)
	control.ssh = controllerResult(map[string]any{"ok": true, "job_id": controllerTestJob, "phase": "succeeded", "finished": true, "cursor": int64(100), "has_more": false, "truncated": true, "records": []any{map[string]any{"sequence": int64(100), "time": int64(1), "step": "packer-build", "event": "completed"}}})
	result, err := control.followLogs(controllerTestJob, 0)
	if err != nil || result["cursor"] != int64(100) {
		t.Fatal("retention gap rejected")
	}
	if !strings.Contains(output.String(), `"truncated":true`) {
		t.Fatal("retention gap not reported")
	}
	control.ssh = controllerResult(map[string]any{"ok": true, "job_id": controllerTestJob, "phase": "succeeded", "finished": true, "cursor": int64(100), "has_more": false, "records": []any{map[string]any{"sequence": int64(100), "time": int64(1), "step": "packer-build", "event": "completed"}}})
	_, err = control.followLogs(controllerTestJob, 100)
	requireControllerError(t, err, "invalid_builder_response")
}

func TestControllerDefaultRunFollowsAndReturnsJobFailure(t *testing.T) {
	control, output := testController(t, nil)
	control.ssh = func(_ context.Context, _ []string, _ []string, input []byte) ([]byte, int, error) {
		request, _ := controllerDecode(input)
		switch request["op"] {
		case "ready":
			return []byte(`{"ok":true,"phase":"ready"}`), 0, nil
		case "submit":
			return controllerJSON(map[string]any{"ok": true, "job_id": controllerTestJob, "phase": "preparing"}), 0, nil
		default:
			return controllerJSON(map[string]any{"ok": true, "job_id": controllerTestJob, "phase": "failed", "finished": true, "cursor": int64(0), "has_more": false, "records": []any{}}), 0, nil
		}
	}
	args, _ := parseControllerArgs([]string{"run", "ubuntu26-base", "--revision", controllerTestRevision})
	_, err := control.execute(args)
	requireControllerError(t, err, "job_failed")
	if !strings.Contains(output.String(), `"phase":"failed"`) || strings.Contains(output.String(), controllerSecret) {
		t.Fatal("failure summary unsafe or absent")
	}
}

func TestControllerShutdownIsGracefulAndWaits(t *testing.T) {
	powers := 0
	control, _ := testController(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatal("controller used hypervisor force stop")
		}
		powers++
		status := "running"
		if powers > 1 {
			status = "stopped"
		}
		fmt.Fprintf(w, `{"data":{"name":"imagebuilder1","status":%q}}`, status)
	})
	control.ssh = func(_ context.Context, _ []string, _ []string, input []byte) ([]byte, int, error) {
		request, _ := controllerDecode(input)
		if request["op"] != "shutdown" {
			t.Fatal("unexpected remote operation")
		}
		return []byte(`{"ok":true,"phase":"stopping"}`), 0, nil
	}
	args, _ := parseControllerArgs([]string{"shutdown"})
	result, err := control.execute(args)
	if err != nil || result["power"] != "stopped" {
		t.Fatal("shutdown failed")
	}
}

func TestControllerBoundedSSHBuffer(t *testing.T) {
	buffer := controllerLimitedBuffer{limit: 10}
	input := bytes.Repeat([]byte("x"), 100)
	count, err := buffer.Write(input)
	if err != nil || count != 100 || len(buffer.data) != 10 {
		t.Fatal("child output not bounded and drained")
	}
}

func TestControllerRemoteErrorsAreValueFree(t *testing.T) {
	control, output := testController(t, nil)
	control.ssh = func(context.Context, []string, []string, []byte) ([]byte, int, error) {
		return nil, 1, fmt.Errorf("unexpected process failure: %s", controllerSecret)
	}
	_, err := control.remote(map[string]any{"op": "status"})
	requireControllerError(t, err, "builder_unreachable")
	if strings.Contains(output.String(), controllerSecret) {
		t.Fatal("unexpected error exposed")
	}
	control.ssh = controllerResult(map[string]any{"ok": false, "error": controllerSecret})
	_, err = control.remote(map[string]any{"op": "status"})
	requireControllerError(t, err, "invalid_builder_response")
}

func TestControllerPublishRetryDoesNotRequireBuildPassword(t *testing.T) {
	control, _ := testController(t, nil)
	delete(control.env, "PKR_VAR_user_password")
	for _, key := range []string{"SEAWEEDFS_S3_ENDPOINT", "SEAWEEDFS_S3_ACCESS_KEY", "SEAWEEDFS_S3_SECRET_KEY"} {
		control.env[key] = controllerSecret
	}
	var submitted map[string]any
	control.ssh = func(_ context.Context, _ []string, _ []string, input []byte) ([]byte, int, error) {
		request, _ := controllerDecode(input)
		if request["op"] == "ready" {
			return []byte(`{"ok":true,"phase":"ready"}`), 0, nil
		}
		submitted = request
		return controllerJSON(map[string]any{"ok": true, "job_id": controllerTestJob, "phase": "preparing"}), 0, nil
	}
	args, err := parseControllerArgs([]string{"publish-retry", controllerTestJob, "--detach"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := control.execute(args)
	if err != nil || result["job_id"] != controllerTestJob {
		t.Fatalf("publish retry failed: %v", err)
	}
	if submitted["op"] != "publish-retry" || submitted["job_id"] != controllerTestJob {
		t.Fatal("wrong retry operation")
	}
	values := submitted["credentials"].(map[string]any)
	if len(values) != 3 {
		t.Fatal("retry credential scope wrong")
	}
}

func TestControllerSSHProcessDiscardsRawStderr(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, exit, err := controllerSSHCommand(ctx, []string{"/bin/sh", "-c", `printf '{"ok":true}'; printf 'SECRET-MARKER' >&2; exit 1`}, []string{"PATH=/usr/bin:/bin"}, nil)
	if err != nil || exit != 1 || string(output) != `{"ok":true}` {
		t.Fatal("SSH process did not separate raw stderr")
	}
}

func TestControllerCustomCAErrorDoesNotExposePath(t *testing.T) {
	_, err := newImageController(map[string]string{"IMAGEBUILDER_PROXMOX_TOKEN": "test", "IMAGEBUILDER_PROXMOX_CA_FILE": filepath.Join(t.TempDir(), controllerSecret)}, io.Discard)
	requireControllerError(t, err, "invalid_proxmox_ca")
}
