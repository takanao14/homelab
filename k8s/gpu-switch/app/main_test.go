package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type scaleCall struct {
	namespace string
	name      string
	replicas  int32
}

type fakeKube struct {
	mu            sync.Mutex
	workloads     []workload
	listErr       error
	listCalls     int
	scaleCalls    []scaleCall
	failScaleCall int
	scaleDelay    time.Duration
	activeScales  int
	maxScales     int
}

func (f *fakeKube) listWorkloads(context.Context) ([]workload, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]workload(nil), f.workloads...), nil
}

func (f *fakeKube) scale(_ context.Context, namespace, name string, replicas int32) error {
	f.mu.Lock()
	f.activeScales++
	if f.activeScales > f.maxScales {
		f.maxScales = f.activeScales
	}
	f.scaleCalls = append(f.scaleCalls, scaleCall{namespace: namespace, name: name, replicas: replicas})
	callNumber := len(f.scaleCalls)
	f.mu.Unlock()

	time.Sleep(f.scaleDelay)

	f.mu.Lock()
	defer f.mu.Unlock()
	f.activeScales--
	if f.failScaleCall == callNumber {
		return errors.New("injected scale failure")
	}
	return nil
}

func testHandler(t *testing.T, kube kubeAPI) http.Handler {
	t.Helper()
	app, err := newServer(kube)
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	return app.handler()
}

func TestState(t *testing.T) {
	kube := &fakeKube{workloads: []workload{
		{Namespace: "ollama", Name: "ollama", Desired: 1, Ready: 1},
		{Namespace: "vllm", Name: "vllm", Desired: 1, Ready: 0},
		{Namespace: "comfyui", Name: "comfyui", Desired: 0, Ready: 0},
	}}
	request := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	response := httptest.NewRecorder()

	testHandler(t, kube).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body)
	}
	var states []stateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &states); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	got := []string{states[0].State, states[1].State, states[2].State}
	want := []string{"running", "starting", "stopped"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("state[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestStateFailure(t *testing.T) {
	kube := &fakeKube{listErr: errors.New("API unavailable")}
	request := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	response := httptest.NewRecorder()

	testHandler(t, kube).ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadGateway)
	}
}

func TestSwitchOrder(t *testing.T) {
	kube := &fakeKube{workloads: []workload{
		{Namespace: "comfyui", Name: "comfyui"},
		{Namespace: "ollama", Name: "ollama"},
	}}
	request := switchRequest(`{"target":"ollama"}`)
	response := httptest.NewRecorder()

	testHandler(t, kube).ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusAccepted, response.Body)
	}
	want := []scaleCall{
		{namespace: "comfyui", name: "comfyui", replicas: 0},
		{namespace: "ollama", name: "ollama", replicas: 0},
		{namespace: "ollama", name: "ollama", replicas: 1},
	}
	assertScaleCalls(t, kube.scaleCalls, want)
}

func TestSwitchOff(t *testing.T) {
	kube := &fakeKube{workloads: []workload{
		{Namespace: "comfyui", Name: "comfyui"},
		{Namespace: "ollama", Name: "ollama"},
	}}
	request := switchRequest(`{"target":null}`)
	response := httptest.NewRecorder()

	testHandler(t, kube).ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusAccepted, response.Body)
	}
	if len(kube.scaleCalls) != 2 {
		t.Fatalf("scale calls = %d, want 2", len(kube.scaleCalls))
	}
	for _, call := range kube.scaleCalls {
		if call.replicas != 0 {
			t.Errorf("replicas = %d, want 0", call.replicas)
		}
	}
}

func TestSwitchStopsAfterScaleDownFailure(t *testing.T) {
	kube := &fakeKube{
		workloads: []workload{
			{Namespace: "comfyui", Name: "comfyui"},
			{Namespace: "ollama", Name: "ollama"},
		},
		failScaleCall: 1,
	}
	request := switchRequest(`{"target":"ollama"}`)
	response := httptest.NewRecorder()

	testHandler(t, kube).ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadGateway)
	}
	if len(kube.scaleCalls) != 1 {
		t.Fatalf("scale calls = %d, want 1", len(kube.scaleCalls))
	}
}

func TestSwitchRejectsUnknownTargetBeforeScaling(t *testing.T) {
	kube := &fakeKube{workloads: []workload{{Namespace: "ollama", Name: "ollama"}}}
	request := switchRequest(`{"target":"unknown"}`)
	response := httptest.NewRecorder()

	testHandler(t, kube).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if len(kube.scaleCalls) != 0 {
		t.Fatalf("scale calls = %d, want 0", len(kube.scaleCalls))
	}
}

func TestSwitchRequestValidation(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
		fetchSite   string
		wantStatus  int
	}{
		{name: "cross origin", body: `{"target":null}`, contentType: "application/json", fetchSite: "cross-site", wantStatus: http.StatusForbidden},
		{name: "missing fetch metadata", body: `{"target":null}`, contentType: "application/json", wantStatus: http.StatusForbidden},
		{name: "wrong content type", body: `{"target":null}`, contentType: "text/plain", fetchSite: "same-origin", wantStatus: http.StatusUnsupportedMediaType},
		{name: "missing target", body: `{}`, contentType: "application/json", fetchSite: "same-origin", wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"target":null,"replicas":1}`, contentType: "application/json", fetchSite: "same-origin", wantStatus: http.StatusBadRequest},
		{name: "empty target", body: `{"target":""}`, contentType: "application/json", fetchSite: "same-origin", wantStatus: http.StatusBadRequest},
		{name: "extra value", body: `{"target":null}{}`, contentType: "application/json", fetchSite: "same-origin", wantStatus: http.StatusBadRequest},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kube := &fakeKube{workloads: []workload{{Namespace: "ollama", Name: "ollama"}}}
			request := httptest.NewRequest(http.MethodPost, "/api/switch", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			if test.fetchSite != "" {
				request.Header.Set("Sec-Fetch-Site", test.fetchSite)
			}
			response := httptest.NewRecorder()

			testHandler(t, kube).ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body)
			}
			if len(kube.scaleCalls) != 0 {
				t.Fatalf("scale calls = %d, want 0", len(kube.scaleCalls))
			}
		})
	}
}

func TestSwitchesAreSerialized(t *testing.T) {
	kube := &fakeKube{
		workloads: []workload{
			{Namespace: "comfyui", Name: "comfyui"},
			{Namespace: "ollama", Name: "ollama"},
		},
		scaleDelay: 5 * time.Millisecond,
	}
	handler := testHandler(t, kube)
	start := make(chan struct{})
	results := make(chan int, 2)

	for _, target := range []string{"comfyui", "ollama"} {
		go func(target string) {
			<-start
			request := switchRequest(`{"target":"` + target + `"}`)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			results <- response.Code
		}(target)
	}
	close(start)
	for range 2 {
		if status := <-results; status != http.StatusAccepted {
			t.Errorf("status = %d, want %d", status, http.StatusAccepted)
		}
	}
	if kube.maxScales != 1 {
		t.Fatalf("concurrent scale calls = %d, want 1", kube.maxScales)
	}
	if kube.listCalls != 2 {
		t.Fatalf("list calls = %d, want 2", kube.listCalls)
	}
}

func TestHealthDoesNotCallKubernetes(t *testing.T) {
	kube := &fakeKube{listErr: errors.New("must not be called")}
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	testHandler(t, kube).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if kube.listCalls != 0 {
		t.Fatalf("list calls = %d, want 0", kube.listCalls)
	}
}

func TestMethodGuards(t *testing.T) {
	handler := testHandler(t, &fakeKube{})
	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/state"},
		{method: http.MethodGet, path: "/api/switch"},
		{method: http.MethodPost, path: "/healthz"},
		{method: http.MethodPost, path: "/"},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
			}
			if response.Header().Get("Allow") == "" {
				t.Fatal("Allow header is missing")
			}
		})
	}
}

func TestWebIsEmbedded(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	testHandler(t, &fakeKube{}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "<title>GPU Switch</title>") {
		t.Fatal("embedded page does not contain title")
	}
	if response.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("Content-Security-Policy header is missing")
	}
}

func switchRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/switch", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	return request
}

func assertScaleCalls(t *testing.T, got, want []scaleCall) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("scale calls = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("scale call[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
