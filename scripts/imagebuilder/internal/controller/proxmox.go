package controller

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
)

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
			return nil, protocol.Fail(rule.code)
		}
	}
	if control.token == "" || strings.ContainsAny(control.token, "\n\r\x00") {
		return nil, protocol.Fail("missing_proxmox_token")
	}
	if control.vmid != "" && !regexp.MustCompile(`^[1-9][0-9]{2,8}$`).MatchString(control.vmid) {
		return nil, protocol.Fail("invalid_vmid")
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, protocol.Fail("invalid_proxmox_ca")
	}
	if caPath, exists := environment["IMAGEBUILDER_PROXMOX_CA_FILE"]; exists {
		pem, err := os.ReadFile(caPath)
		if err != nil || !roots.AppendCertsFromPEM(pem) {
			return nil, protocol.Fail("invalid_proxmox_ca")
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	control.client = &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(request *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	return control, nil
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
		return nil, protocol.Fail("proxmox_request_failed")
	}
	request.Header.Set("Authorization", "PVEAPIToken="+c.token)
	response, err := c.client.Do(request)
	if err != nil {
		return nil, protocol.Fail("proxmox_request_failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, protocol.Fail("proxmox_request_failed")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1048577))
	if err != nil {
		return nil, protocol.Fail("proxmox_request_failed")
	}
	if len(raw) > 1048576 {
		return nil, protocol.Fail("invalid_proxmox_response")
	}
	data, err := controllerDecode(raw)
	if err != nil {
		return nil, protocol.Fail("invalid_proxmox_response")
	}
	value, exists := data["data"]
	if !exists {
		return nil, protocol.Fail("invalid_proxmox_response")
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
			return "", protocol.Fail("invalid_proxmox_response")
		}
		var matches []map[string]any
		for _, item := range array {
			if candidate, ok := item.(map[string]any); ok && candidate["name"] == "imagebuilder1" {
				matches = append(matches, candidate)
			}
		}
		if len(matches) != 1 {
			return "", protocol.Fail("builder_not_found")
		}
		vmid, ok := protocol.Int(matches[0]["vmid"])
		if !ok || vmid < 100 || vmid > 999999999 {
			return "", protocol.Fail("invalid_proxmox_response")
		}
		c.vmid = strconv.FormatInt(vmid, 10)
	}
	raw, err := c.api("qemu/"+c.vmid+"/status/current", false)
	if err != nil {
		return "", err
	}
	data, ok := raw.(map[string]any)
	if !ok || data["name"] != "imagebuilder1" {
		return "", protocol.Fail("unexpected_vm_identity")
	}
	status, ok := data["status"].(string)
	if !ok || (status != "running" && status != "stopped") {
		return "", protocol.Fail("invalid_proxmox_response")
	}
	return status, nil
}
