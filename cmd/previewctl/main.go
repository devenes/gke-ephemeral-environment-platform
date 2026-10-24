package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/devenes/gke-ephemeral-environment-platform/pkg/apis/platform/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func buildClient() (client.Client, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	return client.New(cfg, client.Options{Scheme: scheme})
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, cl client.Client) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage(stdout)
		return nil
	}

	subcmd := args[0]
	subArgs := reorderPositionalArgsLast(args[1:])

	if cl == nil {
		var err error
		cl, err = buildClient()
		if err != nil {
			return err
		}
	}

	switch subcmd {
	case "create", "apply":
		return runCreate(ctx, cl, subArgs, stdout)
	case "get", "status":
		return runGet(ctx, cl, subArgs, stdout)
	case "list", "ls":
		return runList(ctx, cl, subArgs, stdout)
	case "delete", "rm":
		return runDelete(ctx, cl, subArgs, stdout)
	case "dashboard":
		return runDashboard(ctx, cl, subArgs, stdout)
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", subcmd)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `previewctl — CLI for the GKE Ephemeral Environment Platform

Usage:
  previewctl <command> [flags]

Commands:
  create      Create or update a PreviewEnvironment resource
  get         Inspect detailed status, URL, TTL, and conditions of an environment
  list        List all PreviewEnvironments with phase, URL, and remaining TTL
  delete      Delete a PreviewEnvironment and optionally wait for cleanup
  dashboard   Start a local-only (127.0.0.1) read-only HTML status dashboard

Examples:
  previewctl create --name pr-42 --image us-west4-docker.pkg.dev/enes-turan-demo/gke-ephemeral-env/sample-app:v0.1.0 --pr 42 --ttl 1800 --wait
  previewctl get pr-42
  previewctl list
  previewctl delete pr-42 --wait
`)
}

func runCreate(ctx context.Context, cl client.Client, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	var (
		filePath   string
		name       string
		image      string
		developer  string
		repo       string
		commitSHA  string
		prNumber   int
		ttlSeconds int
		replicas   int
		port       int
		cpuReq     string
		cpuLim     string
		memReq     string
		memLim     string
		envFlags   string
		waitReady  bool
		timeout    time.Duration
	)

	fs.StringVar(&filePath, "f", "", "Path to a PreviewEnvironment YAML/JSON manifest file")
	fs.StringVar(&filePath, "file", "", "Path to a PreviewEnvironment YAML/JSON manifest file")
	fs.StringVar(&name, "name", "", "Environment name (required, DNS-1035 label)")
	fs.StringVar(&image, "image", "", "Container image in allowed Artifact Registry (required)")
	fs.StringVar(&developer, "developer", "devenes", "Developer identifier")
	fs.StringVar(&repo, "repo", "devenes/gke-ephemeral-environment-platform", "GitHub owner/repository")
	fs.StringVar(&commitSHA, "commit", "", "Git commit SHA")
	fs.IntVar(&prNumber, "pr", 0, "Pull Request number")
	fs.IntVar(&ttlSeconds, "ttl", 3600, "Time-to-live in seconds (30..86400)")
	fs.IntVar(&replicas, "replicas", 1, "Pod replica count (1..3)")
	fs.IntVar(&port, "port", 8080, "Application container port (1024..65535)")
	fs.StringVar(&cpuReq, "cpu-request", "50m", "Per-pod CPU request")
	fs.StringVar(&cpuLim, "cpu-limit", "200m", "Per-pod CPU limit")
	fs.StringVar(&memReq, "memory-request", "64Mi", "Per-pod memory request")
	fs.StringVar(&memLim, "memory-limit", "128Mi", "Per-pod memory limit")
	fs.StringVar(&envFlags, "env", "", "Comma-separated KEY=VALUE custom environment variables")
	fs.BoolVar(&waitReady, "wait", false, "Wait until the environment reaches Ready or Failed phase")
	fs.DurationVar(&timeout, "timeout", 4*time.Minute, "Timeout when --wait is enabled")

	if err := fs.Parse(args); err != nil {
		return err
	}

	var desiredSpec v1alpha1.PreviewEnvironmentSpec
	if filePath != "" {
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read manifest %q: %w", filePath, err)
		}
		var fromFile v1alpha1.PreviewEnvironment
		if err := yaml.Unmarshal(raw, &fromFile); err != nil {
			return fmt.Errorf("unmarshal manifest %q: %w", filePath, err)
		}
		name = fromFile.Name
		desiredSpec = fromFile.Spec
		if desiredSpec.EnvironmentName == "" {
			desiredSpec.EnvironmentName = name
		}
	} else {
		if name == "" && fs.NArg() > 0 {
			name = fs.Arg(0)
		}
		if name == "" {
			return fmt.Errorf("--name is required")
		}
		if image == "" {
			return fmt.Errorf("--image is required")
		}

		var envVars []v1alpha1.EnvVar
		if strings.TrimSpace(envFlags) != "" {
			for _, pair := range strings.Split(envFlags, ",") {
				kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
				if len(kv) != 2 {
					return fmt.Errorf("invalid --env entry %q (expected KEY=VALUE)", pair)
				}
				envVars = append(envVars, v1alpha1.EnvVar{Name: kv[0], Value: kv[1]})
			}
		}

		desiredSpec = v1alpha1.PreviewEnvironmentSpec{
			EnvironmentName: name,
			Developer:       developer,
			PRNumber:        int32(prNumber),
			Repository:      repo,
			CommitSHA:       commitSHA,
			Image:           image,
			TTLSeconds:      int32(ttlSeconds),
			Replicas:        int32(replicas),
			Port:            int32(port),
			Resources: v1alpha1.ResourceSpec{
				CPURequest:    cpuReq,
				CPULimit:      cpuLim,
				MemoryRequest: memReq,
				MemoryLimit:   memLim,
			},
			Env: envVars,
		}
	}

	var existing v1alpha1.PreviewEnvironment
	err := cl.Get(ctx, types.NamespacedName{Name: name}, &existing)
	if apierrors.IsNotFound(err) {
		pe := &v1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: desiredSpec,
		}
		if err := cl.Create(ctx, pe); err != nil {
			return fmt.Errorf("create PreviewEnvironment %q: %w", name, err)
		}
		fmt.Fprintf(out, "Created PreviewEnvironment %q\n", name)
	} else if err != nil {
		return err
	} else {
		existing.Spec = desiredSpec
		if err := cl.Update(ctx, &existing); err != nil {
			return fmt.Errorf("update PreviewEnvironment %q: %w", name, err)
		}
		fmt.Fprintf(out, "Updated PreviewEnvironment %q\n", name)
	}

	if !waitReady {
		return nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		var current v1alpha1.PreviewEnvironment
		if err := cl.Get(waitCtx, types.NamespacedName{Name: name}, &current); err != nil {
			return err
		}
		switch current.Status.Phase {
		case v1alpha1.PhaseReady:
			fmt.Fprintf(out, "Environment %q is Ready: %s\n", name, current.Status.PreviewURL)
			return nil
		case v1alpha1.PhaseFailed:
			return fmt.Errorf("environment %q failed validation/provisioning: %s", name, current.Status.Message)
		}

		select {
		case <-waitCtx.Done():
			return fmt.Errorf("timed out waiting for %q to become Ready (last phase: %s)", name, current.Status.Phase)
		case <-ticker.C:
		}
	}
}

func runGet(ctx context.Context, cl client.Client, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	var name, outputFormat string
	fs.StringVar(&name, "name", "", "Environment name")
	fs.StringVar(&outputFormat, "o", "text", "Output format: text|json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if name == "" && fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	if name == "" {
		return fmt.Errorf("environment name is required (usage: previewctl get <name>)")
	}

	var pe v1alpha1.PreviewEnvironment
	if err := cl.Get(ctx, types.NamespacedName{Name: name}, &pe); err != nil {
		return err
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(pe)
	}

	fmt.Fprintf(out, "Name:               %s\n", pe.Name)
	fmt.Fprintf(out, "Phase:              %s\n", defaultPhase(pe.Status.Phase))
	fmt.Fprintf(out, "Namespace:          %s\n", pe.Status.Namespace)
	fmt.Fprintf(out, "Preview URL:        %s\n", pe.Status.PreviewURL)
	fmt.Fprintf(out, "Image:              %s\n", pe.Spec.Image)
	fmt.Fprintf(out, "Developer / PR:     %s (PR #%d)\n", pe.Spec.Developer, pe.Spec.PRNumber)
	fmt.Fprintf(out, "Ready Replicas:     %d/%d\n", pe.Status.ReadyReplicas, pe.Spec.Replicas)
	fmt.Fprintf(out, "Remaining TTL:      %s\n", formatRemainingTTL(pe.Status.ExpiresAt))
	if pe.Status.CreatedAt != nil {
		fmt.Fprintf(out, "Created At:         %s\n", pe.Status.CreatedAt.Format(time.RFC3339))
	}
	if pe.Status.ReadyAt != nil {
		fmt.Fprintf(out, "Ready At:           %s\n", pe.Status.ReadyAt.Format(time.RFC3339))
	}
	if pe.Status.ExpiresAt != nil {
		fmt.Fprintf(out, "Expires At:         %s\n", pe.Status.ExpiresAt.Format(time.RFC3339))
	}
	fmt.Fprintf(out, "Message:            %s\n", pe.Status.Message)
	if len(pe.Status.Conditions) > 0 {
		fmt.Fprintf(out, "Conditions:\n")
		for _, c := range pe.Status.Conditions {
			fmt.Fprintf(out, "  - %-16s Status=%-5s Reason=%-22s Message=%s\n", c.Type, c.Status, c.Reason, c.Message)
		}
	}
	return nil
}

func runList(ctx context.Context, cl client.Client, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	var outputFormat string
	fs.StringVar(&outputFormat, "o", "table", "Output format: table|json")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var list v1alpha1.PreviewEnvironmentList
	if err := cl.List(ctx, &list); err != nil {
		return err
	}

	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].Name < list.Items[j].Name
	})

	if outputFormat == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(list.Items)
	}

	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tPHASE\tNAMESPACE\tREADY\tREMAINING_TTL\tPREVIEW_URL\tMESSAGE")
	for _, pe := range list.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d/%d\t%s\t%s\t%s\n",
			pe.Name,
			defaultPhase(pe.Status.Phase),
			pe.Status.Namespace,
			pe.Status.ReadyReplicas,
			pe.Spec.Replicas,
			formatRemainingTTL(pe.Status.ExpiresAt),
			pe.Status.PreviewURL,
			pe.Status.Message,
		)
	}
	return tw.Flush()
}

func runDelete(ctx context.Context, cl client.Client, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	var (
		name     string
		waitGone bool
		timeout  time.Duration
	)
	fs.StringVar(&name, "name", "", "Environment name")
	fs.BoolVar(&waitGone, "wait", false, "Wait for complete deletion of the resource and namespace")
	fs.DurationVar(&timeout, "timeout", 3*time.Minute, "Timeout when --wait is enabled")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if name == "" && fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	if name == "" {
		return fmt.Errorf("environment name is required (usage: previewctl delete <name>)")
	}

	pe := &v1alpha1.PreviewEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	if err := cl.Delete(ctx, pe); err != nil {
		if apierrors.IsNotFound(err) {
			fmt.Fprintf(out, "PreviewEnvironment %q already absent\n", name)
			return nil
		}
		return err
	}
	fmt.Fprintf(out, "Deleted PreviewEnvironment %q\n", name)

	if !waitGone {
		return nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		var check v1alpha1.PreviewEnvironment
		err := cl.Get(waitCtx, types.NamespacedName{Name: name}, &check)
		if apierrors.IsNotFound(err) {
			fmt.Fprintf(out, "Confirmed PreviewEnvironment %q and its owned namespace are completely removed\n", name)
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("timed out waiting for deletion of %q", name)
		case <-ticker.C:
		}
	}
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>GKE Ephemeral Environment Platform — Read-Only Dashboard</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 2rem; background: #0f172a; color: #f8fafc; }
    h1 { font-size: 1.5rem; margin-bottom: 0.5rem; }
    table { width: 100%; border-collapse: collapse; margin-top: 1rem; background: #1e293b; border-radius: 8px; overflow: hidden; }
    th, td { padding: 0.75rem 1rem; text-align: left; border-bottom: 1px solid #334155; font-size: 0.9rem; }
    th { background: #334155; color: #cbd5e1; }
    a { color: #38bdf8; }
    .Ready { color: #4ade80; font-weight: 600; }
    .Failed { color: #f87171; font-weight: 600; }
    .Provisioning, .Pending { color: #facc15; font-weight: 600; }
  </style>
</head>
<body>
  <h1>GKE Ephemeral Environment Platform (Read-Only Local View)</h1>
  <table>
    <thead>
      <tr>
        <th>Name</th><th>Phase</th><th>Namespace</th><th>URL</th><th>Remaining TTL</th><th>Created At</th><th>Expires At</th><th>Message / Conditions</th>
      </tr>
    </thead>
    <tbody>
      {{range .}}
      <tr>
        <td>{{.Name}}</td>
        <td class="{{.Phase}}">{{.Phase}}</td>
        <td>{{.Namespace}}</td>
        <td>{{if .URL}}<a href="{{.URL}}">{{.URL}}</a>{{else}}-{{end}}</td>
        <td>{{.RemainingTTL}}</td>
        <td>{{.CreatedAt}}</td>
        <td>{{.ExpiresAt}}</td>
        <td>{{.Message}}</td>
      </tr>
      {{else}}
      <tr><td colspan="8">No active preview environments.</td></tr>
      {{end}}
    </tbody>
  </table>
</body>
</html>`

func runDashboard(ctx context.Context, cl client.Client, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("dashboard", flag.ContinueOnError)
	var addr string
	fs.StringVar(&addr, "addr", "127.0.0.1:9090", "Local loopback address to bind read-only dashboard")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !strings.HasPrefix(addr, "127.0.0.1:") && !strings.HasPrefix(addr, "localhost:") {
		return fmt.Errorf("dashboard must bind to loopback (127.0.0.1 or localhost) to prevent unauthenticated public exposure")
	}

	tmpl := template.Must(template.New("dash").Parse(dashboardHTML))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var list v1alpha1.PreviewEnvironmentList
		if err := cl.List(r.Context(), &list); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type row struct {
			Name, Phase, Namespace, URL, RemainingTTL, CreatedAt, ExpiresAt, Message string
		}
		var rows []row
		for _, pe := range list.Items {
			ca, ea := "-", "-"
			if pe.Status.CreatedAt != nil {
				ca = pe.Status.CreatedAt.Format(time.RFC3339)
			}
			if pe.Status.ExpiresAt != nil {
				ea = pe.Status.ExpiresAt.Format(time.RFC3339)
			}
			rows = append(rows, row{
				Name:         pe.Name,
				Phase:        string(defaultPhase(pe.Status.Phase)),
				Namespace:    pe.Status.Namespace,
				URL:          pe.Status.PreviewURL,
				RemainingTTL: formatRemainingTTL(pe.Status.ExpiresAt),
				CreatedAt:    ca,
				ExpiresAt:    ea,
				Message:      pe.Status.Message,
			})
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.Execute(w, rows)
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Fprintf(out, "Starting read-only dashboard on http://%s\n", addr)
	return srv.ListenAndServe()
}

func reorderPositionalArgsLast(args []string) []string {
	boolFlags := map[string]bool{
		"-wait":  true,
		"--wait": true,
		"-h":     true,
		"--help": true,
	}
	var flagsPart, posPart []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flagsPart = append(flagsPart, a)
			if !strings.Contains(a, "=") && !boolFlags[a] && i+1 < len(args) {
				i++
				flagsPart = append(flagsPart, args[i])
			}
		} else {
			posPart = append(posPart, a)
		}
	}
	return append(flagsPart, posPart...)
}

func defaultPhase(p v1alpha1.PreviewPhase) v1alpha1.PreviewPhase {
	if p == "" {
		return v1alpha1.PhasePending
	}
	return p
}

func formatRemainingTTL(expiresAt *metav1.Time) string {
	if expiresAt == nil || expiresAt.IsZero() {
		return "-"
	}
	rem := time.Until(expiresAt.Time).Round(time.Second)
	if rem <= 0 {
		return "expired"
	}
	return rem.String()
}
