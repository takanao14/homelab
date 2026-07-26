package main

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestListWorkloads(t *testing.T) {
	var gotSelector string
	api := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		gotSelector = request.URL.Query().Get("labelSelector")
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{
			"items": [
				{"metadata":{"namespace":"vllm","name":"vllm"},"spec":{"replicas":1},"status":{}},
				{"metadata":{"namespace":"comfyui","name":"comfyui"},"spec":{"replicas":0},"status":{"readyReplicas":0}},
				{"metadata":{"namespace":"ollama","name":"ollama"},"spec":{},"status":{"readyReplicas":1}}
			]
		}`)
	}))
	defer api.Close()

	client := testKubeClient(t, api.URL)
	workloads, err := client.listWorkloads(context.Background())
	if err != nil {
		t.Fatalf("listWorkloads() error = %v", err)
	}
	if gotSelector != labelSelector {
		t.Errorf("labelSelector = %q, want %q", gotSelector, labelSelector)
	}
	if got := []string{workloads[0].Name, workloads[1].Name, workloads[2].Name}; strings.Join(got, ",") != "comfyui,ollama,vllm" {
		t.Errorf("workload order = %v, want [comfyui ollama vllm]", got)
	}
	if workloads[1].Desired != 1 {
		t.Errorf("default replicas = %d, want 1", workloads[1].Desired)
	}
}

func TestListWorkloadsRejectsEmptyAndDuplicateNames(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "empty", body: `{"items":[]}`, want: "no deployment labelled"},
		{
			name: "duplicate",
			body: `{"items":[
				{"metadata":{"namespace":"a","name":"ollama"},"spec":{"replicas":0}},
				{"metadata":{"namespace":"b","name":"ollama"},"spec":{"replicas":0}}
			]}`,
			want: "ambiguous",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				_, _ = io.WriteString(response, test.body)
			}))
			defer api.Close()

			client := testKubeClient(t, api.URL)
			_, err := client.listWorkloads(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("listWorkloads() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestScaleRequest(t *testing.T) {
	var gotMethod, gotPath, gotContentType string
	var gotBody map[string]map[string]int32
	api := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		gotMethod = request.Method
		gotPath = request.URL.Path
		gotContentType = request.Header.Get("Content-Type")
		if err := json.NewDecoder(request.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		response.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	client := testKubeClient(t, api.URL)
	if err := client.scale(context.Background(), "ollama", "ollama", 1); err != nil {
		t.Fatalf("scale() error = %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/apis/apps/v1/namespaces/ollama/deployments/ollama/scale" {
		t.Errorf("path = %q", gotPath)
	}
	if gotContentType != "application/merge-patch+json" {
		t.Errorf("Content-Type = %q", gotContentType)
	}
	if gotBody["spec"]["replicas"] != 1 {
		t.Errorf("replicas = %d, want 1", gotBody["spec"]["replicas"])
	}
}

func TestHTTPSReadsTokenForEveryRequest(t *testing.T) {
	var mu sync.Mutex
	var authorizations []string
	api := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mu.Lock()
		authorizations = append(authorizations, request.Header.Get("Authorization"))
		mu.Unlock()
		_, _ = io.WriteString(response, `{"items":[{"metadata":{"namespace":"ollama","name":"ollama"},"spec":{"replicas":0}}]}`)
	}))
	defer api.Close()

	tempDir := t.TempDir()
	tokenFile := filepath.Join(tempDir, "token")
	caFile := filepath.Join(tempDir, "ca.crt")
	writeTestFile(t, tokenFile, []byte("first-token\n"))
	certificate := api.Certificate()
	writeTestFile(t, caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}))

	parsed, err := url.Parse(api.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &kubeClient{
		apiURL:    parsed,
		tokenFile: tokenFile,
		caFile:    caFile,
		timeout:   time.Second,
	}
	if _, err := client.listWorkloads(context.Background()); err != nil {
		t.Fatalf("first listWorkloads() error = %v", err)
	}
	writeTestFile(t, tokenFile, []byte("second-token\n"))
	if _, err := client.listWorkloads(context.Background()); err != nil {
		t.Fatalf("second listWorkloads() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"Bearer first-token", "Bearer second-token"}
	if fmt.Sprint(authorizations) != fmt.Sprint(want) {
		t.Errorf("Authorization headers = %v, want %v", authorizations, want)
	}
}

func TestKubernetesAPIError(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Error(response, "forbidden detail", http.StatusForbidden)
	}))
	defer api.Close()

	client := testKubeClient(t, api.URL)
	_, err := client.listWorkloads(context.Background())
	if err == nil || !strings.Contains(err.Error(), "403 Forbidden") {
		t.Fatalf("listWorkloads() error = %v, want 403 status", err)
	}
}

func TestNewKubeClientFromEnv(t *testing.T) {
	t.Setenv("KUBE_API_URL", "http://127.0.0.1:8001")
	t.Setenv("KUBE_TOKEN_FILE", "")
	t.Setenv("KUBE_CA_FILE", "")

	client, err := newKubeClientFromEnv()
	if err != nil {
		t.Fatalf("newKubeClientFromEnv() error = %v", err)
	}
	if client.apiURL.String() != "http://127.0.0.1:8001" {
		t.Errorf("apiURL = %q", client.apiURL)
	}
	if client.tokenFile != "" || client.caFile != "" {
		t.Errorf("local proxy credential paths = %q, %q; want empty", client.tokenFile, client.caFile)
	}
}

func testKubeClient(t *testing.T, rawURL string) *kubeClient {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return &kubeClient{apiURL: parsed, timeout: time.Second}
}

func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}
