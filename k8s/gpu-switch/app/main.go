package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed web/index.html
var embeddedWeb embed.FS

type stateResponse struct {
	Name    string `json:"name"`
	Desired int32  `json:"desired"`
	Ready   int32  `json:"ready"`
	State   string `json:"state"`
}

type server struct {
	kube     kubeAPI
	switchMu sync.Mutex
	web      http.Handler
}

func newServer(kube kubeAPI) (*server, error) {
	webRoot, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		return nil, fmt.Errorf("open embedded web assets: %w", err)
	}
	return &server{
		kube: kube,
		web:  http.FileServer(http.FS(webRoot)),
	}, nil
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/switch", s.handleSwitch)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/", s.handleWeb)
	return securityHeaders(mux)
}

func (s *server) handleState(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(response, http.MethodGet)
		return
	}

	workloads, err := s.kube.listWorkloads(request.Context())
	if err != nil {
		log.Printf("state discovery failed: %v", err)
		writeError(response, http.StatusBadGateway, "Kubernetes API request failed")
		return
	}

	states := make([]stateResponse, 0, len(workloads))
	for _, item := range workloads {
		state := "stopped"
		if item.Desired > 0 {
			state = "starting"
			if item.Ready > 0 {
				state = "running"
			}
		}
		states = append(states, stateResponse{
			Name:    item.Name,
			Desired: item.Desired,
			Ready:   item.Ready,
			State:   state,
		})
	}
	writeJSON(response, http.StatusOK, states)
}

func (s *server) handleSwitch(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		methodNotAllowed(response, http.MethodPost)
		return
	}
	if request.Header.Get("Sec-Fetch-Site") != "same-origin" {
		writeError(response, http.StatusForbidden, "cross-origin requests are not allowed")
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(response, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	request.Body = http.MaxBytesReader(response, request.Body, 1024)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var payload struct {
		Target json.RawMessage `json:"target"`
	}
	if err := decoder.Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, "invalid JSON request")
		return
	}
	if err := requireJSONEOF(decoder); err != nil {
		writeError(response, http.StatusBadRequest, "request must contain one JSON object")
		return
	}
	if payload.Target == nil {
		writeError(response, http.StatusBadRequest, "target is required")
		return
	}

	var target *string
	if !bytes.Equal(bytes.TrimSpace(payload.Target), []byte("null")) {
		var name string
		if err := json.Unmarshal(payload.Target, &name); err != nil || strings.TrimSpace(name) == "" {
			writeError(response, http.StatusBadRequest, "target must be a workload name or null")
			return
		}
		target = &name
	}

	s.switchMu.Lock()
	defer s.switchMu.Unlock()

	workloads, err := s.kube.listWorkloads(request.Context())
	if err != nil {
		log.Printf("switch discovery failed: %v", err)
		writeError(response, http.StatusBadGateway, "Kubernetes API request failed")
		return
	}

	var selected *workload
	if target != nil {
		for i := range workloads {
			if workloads[i].Name == *target {
				selected = &workloads[i]
				break
			}
		}
		if selected == nil {
			writeError(response, http.StatusBadRequest, "unknown switchable workload")
			return
		}
	}

	for _, item := range workloads {
		if err := s.kube.scale(request.Context(), item.Namespace, item.Name, 0); err != nil {
			log.Printf("scale-down failed for %s/%s: %v", item.Namespace, item.Name, err)
			writeError(response, http.StatusBadGateway, "Kubernetes API scale request failed")
			return
		}
	}
	if selected != nil {
		if err := s.kube.scale(request.Context(), selected.Namespace, selected.Name, 1); err != nil {
			log.Printf("scale-up failed for %s/%s: %v", selected.Namespace, selected.Name, err)
			writeError(response, http.StatusBadGateway, "Kubernetes API scale request failed")
			return
		}
	}

	writeJSON(response, http.StatusAccepted, struct {
		Target *string `json:"target"`
	}{Target: target})
}

func (s *server) handleHealth(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(response, http.MethodGet)
		return
	}
	response.Header().Set("Content-Type", "text/plain; charset=utf-8")
	response.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(response, "ok\n")
}

func (s *server) handleWeb(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		methodNotAllowed(response, http.MethodGet, http.MethodHead)
		return
	}
	if request.URL.Path != "/" {
		http.NotFound(response, request)
		return
	}
	s.web.ServeHTTP(response, request)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("extra JSON value")
	}
	return err
}

func methodNotAllowed(response http.ResponseWriter, allowed ...string) {
	response.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(response, http.StatusMethodNotAllowed, "method not allowed")
}

func writeError(response http.ResponseWriter, status int, message string) {
	writeJSON(response, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(status)
	if err := json.NewEncoder(response).Encode(value); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set(
			"Content-Security-Policy",
			"default-src 'none'; connect-src 'self'; img-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'",
		)
		response.Header().Set("Referrer-Policy", "no-referrer")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(response, request)
	})
}

func main() {
	kube, err := newKubeClientFromEnv()
	if err != nil {
		log.Fatalf("configure Kubernetes client: %v", err)
	}
	app, err := newServer(kube)
	if err != nil {
		log.Fatalf("configure server: %v", err)
	}

	httpServer := &http.Server{
		Addr:              ":8080",
		Handler:           app.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownSignal.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown: %v", err)
		}
	}()

	log.Printf("listening on %s", httpServer.Addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
}
