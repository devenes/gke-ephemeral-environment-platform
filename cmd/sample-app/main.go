package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"
)

var (
	// Version and GitCommit can be overridden via -ldflags at build time.
	Version   = "v0.1.0"
	GitCommit = "dev"
	BuildTime = "unknown"
)

// AppConfig holds environment metadata injected by the preview-controller.
type AppConfig struct {
	EnvironmentName string            `json:"environmentName"`
	Namespace       string            `json:"namespace"`
	PRNumber        string            `json:"prNumber,omitempty"`
	Repository      string            `json:"repository,omitempty"`
	CommitSHA       string            `json:"commitSHA,omitempty"`
	Developer       string            `json:"developer,omitempty"`
	Hostname        string            `json:"hostname"`
	Version         string            `json:"version"`
	GitCommit       string            `json:"gitCommit"`
	BuildTime       string            `json:"buildTime"`
	CustomEnv       map[string]string `json:"customEnv,omitempty"`
}

// RootResponse is returned by GET / (and GET /preview/<env>/).
type RootResponse struct {
	Service         string            `json:"service"`
	Status          string            `json:"status"`
	EnvironmentName string            `json:"environmentName"`
	Namespace       string            `json:"namespace"`
	PRNumber        string            `json:"prNumber,omitempty"`
	Repository      string            `json:"repository,omitempty"`
	CommitSHA       string            `json:"commitSHA,omitempty"`
	Developer       string            `json:"developer,omitempty"`
	Hostname        string            `json:"hostname"`
	Version         string            `json:"version"`
	GitCommit       string            `json:"gitCommit"`
	RequestPath     string            `json:"requestPath"`
	Timestamp       string            `json:"timestamp"`
	CustomEnv       map[string]string `json:"customEnv,omitempty"`
}

// HealthResponse is returned by GET /healthz and GET /readyz.
type HealthResponse struct {
	Status          string `json:"status"`
	Check           string `json:"check"`
	EnvironmentName string `json:"environmentName"`
	Timestamp       string `json:"timestamp"`
}

// VersionResponse is returned by GET /version.
type VersionResponse struct {
	Service         string `json:"service"`
	EnvironmentName string `json:"environmentName"`
	Version         string `json:"version"`
	GitCommit       string `json:"gitCommit"`
	BuildTime       string `json:"buildTime"`
	CommitSHA       string `json:"commitSHA,omitempty"`
}

func loadConfigFromEnv() AppConfig {
	hostname, _ := os.Hostname()
	envName := getenvDefault("PREVIEW_ENV_NAME", "local-dev")
	ns := getenvDefault("PREVIEW_NAMESPACE", "default")
	ver := getenvDefault("APP_VERSION", Version)
	commit := getenvDefault("PREVIEW_COMMIT_SHA", GitCommit)

	custom := make(map[string]string)
	var keys []string
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], "APP_CUSTOM_") {
			keys = append(keys, parts[0])
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		custom[strings.TrimPrefix(k, "APP_CUSTOM_")] = os.Getenv(k)
	}

	return AppConfig{
		EnvironmentName: envName,
		Namespace:       ns,
		PRNumber:        os.Getenv("PREVIEW_PR_NUMBER"),
		Repository:      os.Getenv("PREVIEW_REPOSITORY"),
		CommitSHA:       commit,
		Developer:       os.Getenv("PREVIEW_DEVELOPER"),
		Hostname:        hostname,
		Version:         ver,
		GitCommit:       GitCommit,
		BuildTime:       BuildTime,
		CustomEnv:       custom,
	}
}

func getenvDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// normalizePreviewPath strips an optional /preview/<env-name> prefix so the application
// works seamlessly whether accessed directly at / or through path-based routing before/after URLRewrite.
func normalizePreviewPath(rawPath string) string {
	p := rawPath
	if p == "" {
		return "/"
	}
	for strings.HasPrefix(p, "//") {
		p = strings.TrimPrefix(p, "/")
	}
	if strings.HasPrefix(p, "/preview/") {
		rest := strings.TrimPrefix(p, "/preview/")
		slashIdx := strings.IndexByte(rest, '/')
		if slashIdx == -1 {
			return "/"
		}
		p = rest[slashIdx:]
	}
	if p == "" {
		return "/"
	}
	return p
}

func NewHandler(cfg AppConfig) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		normalized := normalizePreviewPath(r.URL.Path)
		switch normalized {
		case "/healthz":
			writeJSON(w, http.StatusOK, HealthResponse{
				Status:          "ok",
				Check:           "liveness",
				EnvironmentName: cfg.EnvironmentName,
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
			})
			return
		case "/readyz":
			writeJSON(w, http.StatusOK, HealthResponse{
				Status:          "ready",
				Check:           "readiness",
				EnvironmentName: cfg.EnvironmentName,
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
			})
			return
		case "/version":
			writeJSON(w, http.StatusOK, VersionResponse{
				Service:         "preview-sample-app",
				EnvironmentName: cfg.EnvironmentName,
				Version:         cfg.Version,
				GitCommit:       cfg.GitCommit,
				BuildTime:       cfg.BuildTime,
				CommitSHA:       cfg.CommitSHA,
			})
			return
		case "/":
			writeJSON(w, http.StatusOK, RootResponse{
				Service:         "preview-sample-app",
				Status:          "running",
				EnvironmentName: cfg.EnvironmentName,
				Namespace:       cfg.Namespace,
				PRNumber:        cfg.PRNumber,
				Repository:      cfg.Repository,
				CommitSHA:       cfg.CommitSHA,
				Developer:       cfg.Developer,
				Hostname:        cfg.Hostname,
				Version:         cfg.Version,
				GitCommit:       cfg.GitCommit,
				RequestPath:     r.URL.Path,
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
				CustomEnv:       cfg.CustomEnv,
			})
			return
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error":           "not_found",
				"path":            r.URL.Path,
				"normalizedPath":  normalized,
				"environmentName": cfg.EnvironmentName,
			})
		}
	})

	return mux
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func main() {
	var (
		probeURL     string
		probeTimeout time.Duration
	)
	flag.StringVar(&probeURL, "probe-url", "", "Optional target HTTP URL to probe once and exit (used for network isolation verification)")
	flag.DurationVar(&probeTimeout, "probe-timeout", 4*time.Second, "Timeout when --probe-url is specified")
	flag.Parse()

	if strings.TrimSpace(probeURL) != "" {
		client := &http.Client{Timeout: probeTimeout}
		resp, err := client.Get(probeURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "probe failed for %s: %v\n", probeURL, err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("probe status=%d url=%s body=%s\n", resp.StatusCode, probeURL, strings.TrimSpace(string(body)))
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			os.Exit(0)
		}
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := loadConfigFromEnv()
	port := getenvDefault("PORT", "8080")

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           NewHandler(cfg),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("starting preview sample application",
			"addr", srv.Addr,
			"environment", cfg.EnvironmentName,
			"namespace", cfg.Namespace,
			"version", cfg.Version,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server exited with error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down preview sample application")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
