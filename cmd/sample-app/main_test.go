package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSampleAppEndpoints(t *testing.T) {
	cfg := AppConfig{
		EnvironmentName: "pr-42",
		Namespace:       "preview-pr-42",
		PRNumber:        "42",
		Repository:      "devenes/gke-ephemeral-environment-platform",
		CommitSHA:       "abcdef123456",
		Developer:       "devenes",
		Hostname:        "pod-xyz",
		Version:         "v1.0.0",
		GitCommit:       "abcdef1",
		BuildTime:       "2026-10-24T00:00:00Z",
		CustomEnv:       map[string]string{"FEATURE_FLAG": "true"},
	}

	handler := NewHandler(cfg)

	tests := []struct {
		name       string
		path       string
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:       "root path",
			path:       "/",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp RootResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("unmarshal root response: %v", err)
				}
				if resp.EnvironmentName != "pr-42" || resp.Namespace != "preview-pr-42" || resp.Status != "running" {
					t.Fatalf("unexpected root response: %+v", resp)
				}
				if resp.CustomEnv["FEATURE_FLAG"] != "true" {
					t.Fatalf("expected custom env FEATURE_FLAG=true, got %+v", resp.CustomEnv)
				}
			},
		},
		{
			name:       "prefixed root path",
			path:       "/preview/pr-42",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp RootResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("unmarshal root response: %v", err)
				}
				if resp.EnvironmentName != "pr-42" {
					t.Fatalf("expected pr-42, got %s", resp.EnvironmentName)
				}
			},
		},
		{
			name:       "healthz direct",
			path:       "/healthz",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp HealthResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("unmarshal healthz: %v", err)
				}
				if resp.Status != "ok" || resp.Check != "liveness" {
					t.Fatalf("unexpected healthz: %+v", resp)
				}
			},
		},
		{
			name:       "readyz prefixed",
			path:       "/preview/pr-42/readyz",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp HealthResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("unmarshal readyz: %v", err)
				}
				if resp.Status != "ready" || resp.Check != "readiness" {
					t.Fatalf("unexpected readyz: %+v", resp)
				}
			},
		},
		{
			name:       "version endpoint",
			path:       "/version",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp VersionResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("unmarshal version: %v", err)
				}
				if resp.Version != "v1.0.0" || resp.EnvironmentName != "pr-42" {
					t.Fatalf("unexpected version: %+v", resp)
				}
			},
		},
		{
			name:       "unknown endpoint returns 404",
			path:       "/does-not-exist",
			wantStatus: http.StatusNotFound,
			checkBody:  func(t *testing.T, body []byte) {},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("GET %s: got status %d, want %d", tc.path, rec.Code, tc.wantStatus)
			}
			tc.checkBody(t, rec.Body.Bytes())
		})
	}
}
