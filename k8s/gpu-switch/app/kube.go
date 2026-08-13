package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	labelSelector    = "homelab/gpu-switchable=true"
	defaultTokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	defaultCAFile    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

type workload struct {
	Namespace string
	Name      string
	Desired   int32
	Ready     int32
}

type kubeAPI interface {
	listWorkloads(context.Context) ([]workload, error)
	scale(context.Context, string, string, int32) error
}

type kubeClient struct {
	apiURL    *url.URL
	tokenFile string
	caFile    string
	timeout   time.Duration
}

type transportBody struct {
	io.ReadCloser
	transport *http.Transport
}

func (b *transportBody) Close() error {
	err := b.ReadCloser.Close()
	b.transport.CloseIdleConnections()
	return err
}

func newKubeClientFromEnv() (*kubeClient, error) {
	apiURL := os.Getenv("KUBE_API_URL")
	if apiURL == "" {
		host := os.Getenv("KUBERNETES_SERVICE_HOST")
		port := os.Getenv("KUBERNETES_SERVICE_PORT")
		if host == "" || port == "" {
			return nil, errors.New("KUBE_API_URL or KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be set")
		}
		apiURL = "https://" + host + ":" + port
	}

	parsed, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("parse Kubernetes API URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("Kubernetes API URL must use http or https, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, errors.New("Kubernetes API URL must include a host")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")

	return &kubeClient{
		apiURL:    parsed,
		tokenFile: envOrDefault("KUBE_TOKEN_FILE", defaultTokenFile),
		caFile:    envOrDefault("KUBE_CA_FILE", defaultCAFile),
		timeout:   10 * time.Second,
	}, nil
}

func envOrDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}

func (k *kubeClient) listWorkloads(ctx context.Context) ([]workload, error) {
	query := url.Values{"labelSelector": []string{labelSelector}}
	path := "/apis/apps/v1/deployments?" + query.Encode()

	response, err := k.do(ctx, http.MethodGet, path, "", nil)
	if err != nil {
		return nil, fmt.Errorf("list switchable deployments: %w", err)
	}
	defer response.Body.Close()

	var result struct {
		Items []struct {
			Metadata struct {
				Namespace string `json:"namespace"`
				Name      string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				Replicas *int32 `json:"replicas"`
			} `json:"spec"`
			Status struct {
				ReadyReplicas int32 `json:"readyReplicas"`
			} `json:"status"`
		} `json:"items"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode deployment list: %w", err)
	}

	workloads := make([]workload, 0, len(result.Items))
	for _, item := range result.Items {
		desired := int32(1)
		if item.Spec.Replicas != nil {
			desired = *item.Spec.Replicas
		}
		workloads = append(workloads, workload{
			Namespace: item.Metadata.Namespace,
			Name:      item.Metadata.Name,
			Desired:   desired,
			Ready:     item.Status.ReadyReplicas,
		})
	}
	sort.Slice(workloads, func(i, j int) bool {
		if workloads[i].Name == workloads[j].Name {
			return workloads[i].Namespace < workloads[j].Namespace
		}
		return workloads[i].Name < workloads[j].Name
	})

	if len(workloads) == 0 {
		return nil, fmt.Errorf("no deployment labelled %s exists", labelSelector)
	}
	for i := 1; i < len(workloads); i++ {
		if workloads[i-1].Name == workloads[i].Name {
			return nil, fmt.Errorf(
				"deployment name %q is ambiguous between namespaces %q and %q",
				workloads[i].Name,
				workloads[i-1].Namespace,
				workloads[i].Namespace,
			)
		}
	}
	return workloads, nil
}

func (k *kubeClient) scale(ctx context.Context, namespace, name string, replicas int32) error {
	payload := struct {
		Spec map[string]int32 `json:"spec"`
	}{
		Spec: map[string]int32{"replicas": replicas},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode scale request: %w", err)
	}

	path := fmt.Sprintf(
		"/apis/apps/v1/namespaces/%s/deployments/%s/scale",
		url.PathEscape(namespace),
		url.PathEscape(name),
	)
	response, err := k.do(ctx, http.MethodPatch, path, "application/merge-patch+json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("scale %s/%s to %d: %w", namespace, name, replicas, err)
	}
	response.Body.Close()
	return nil
}

func (k *kubeClient) do(
	ctx context.Context,
	method string,
	path string,
	contentType string,
	body io.Reader,
) (*http.Response, error) {
	requestURL := *k.apiURL
	requestURL.Path = strings.TrimRight(k.apiURL.Path, "/") + strings.SplitN(path, "?", 2)[0]
	if parts := strings.SplitN(path, "?", 2); len(parts) == 2 {
		requestURL.RawQuery = parts[1]
	}

	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}

	// Re-read the projected token and CA after rotation. A shared transport must
	// move token reads into a RoundTripper; per-request TLS is acceptable here.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if k.apiURL.Scheme == "https" {
		token, err := os.ReadFile(k.tokenFile)
		if err != nil {
			return nil, fmt.Errorf("read service account token: %w", err)
		}
		request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))

		caPEM, err := os.ReadFile(k.caFile)
		if err != nil {
			return nil, fmt.Errorf("read service account CA: %w", err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("service account CA file contains no certificates")
		}
		transport.TLSClientConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    roots,
		}
	}

	client := &http.Client{Transport: transport, Timeout: k.timeout}
	response, err := client.Do(request)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	response.Body = &transportBody{ReadCloser: response.Body, transport: transport}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer response.Body.Close()
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf(
			"Kubernetes API returned %s: %s",
			response.Status,
			strings.TrimSpace(string(message)),
		)
	}
	return response, nil
}
