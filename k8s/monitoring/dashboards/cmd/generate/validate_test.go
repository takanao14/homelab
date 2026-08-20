package main

import (
	"strings"
	"testing"
)

func TestValidateDashboardJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		data        string
		wantErr     bool
		wantErrText string
	}{
		{
			name: "explicit thresholds",
			data: `{"panels":[{"title":"Healthy","options":{"colorMode":"background"},"fieldConfig":{"defaults":{"thresholds":{"steps":[{"color":"blue"}]}}}}]}`,
		},
		{
			name: "color disabled",
			data: `{"panels":[{"title":"Plain","options":{"colorMode":"none"},"fieldConfig":{"defaults":{}}}]}`,
		},
		{
			name: "color mode absent",
			data: `{"panels":[{"title":"Graph","options":{},"fieldConfig":{"defaults":{}}}]}`,
		},
		{
			name:        "thresholds absent",
			data:        `{"panels":[{"title":"Missing","options":{"colorMode":"value"},"fieldConfig":{"defaults":{}}}]}`,
			wantErr:     true,
			wantErrText: `audit: panel "Missing" uses colorMode "value" without threshold steps`,
		},
		{
			name:        "threshold steps empty",
			data:        `{"panels":[{"title":"Empty","options":{"colorMode":"background"},"fieldConfig":{"defaults":{"thresholds":{"steps":[]}}}}]}`,
			wantErr:     true,
			wantErrText: `audit: panel "Empty" uses colorMode "background" without threshold steps`,
		},
		{
			name:        "nested row panel",
			data:        `{"panels":[{"title":"Summary","panels":[{"title":"Nested","options":{"colorMode":"background"},"fieldConfig":{"defaults":{}}}]}]}`,
			wantErr:     true,
			wantErrText: `audit: panel "Nested" uses colorMode "background" without threshold steps`,
		},
		{
			name:        "invalid JSON",
			data:        `{`,
			wantErr:     true,
			wantErrText: "audit: decode generated JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateDashboardJSON("audit", []byte(tt.data))
			if tt.wantErr && err == nil {
				t.Fatal("validateDashboardJSON() error = nil, want an error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateDashboardJSON() unexpected error: %v", err)
			}
			if tt.wantErrText != "" && !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("validateDashboardJSON() error = %q, want substring %q", err, tt.wantErrText)
			}
		})
	}
}
