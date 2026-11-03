package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/devenes/gke-ephemeral-environment-platform/pkg/apis/platform/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPreviewctlLifecycleCommands(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	var stdout, stderr bytes.Buffer

	// 1. Create
	err := run(ctx, []string{
		"create",
		"--name", "cli-test",
		"--image", "us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/sample-app:v0.1.0",
		"--pr", "77",
		"--ttl", "1200",
	}, &stdout, &stderr, cl)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !strings.Contains(stdout.String(), `Created PreviewEnvironment "cli-test"`) {
		t.Fatalf("unexpected create output: %s", stdout.String())
	}

	// 2. Get
	stdout.Reset()
	err = run(ctx, []string{"get", "cli-test"}, &stdout, &stderr, cl)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "cli-test") || !strings.Contains(stdout.String(), "PR #77") {
		t.Fatalf("unexpected get output: %s", stdout.String())
	}

	// 3. List
	stdout.Reset()
	err = run(ctx, []string{"list"}, &stdout, &stderr, cl)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "cli-test") {
		t.Fatalf("unexpected list output: %s", stdout.String())
	}

	// 4. Delete
	stdout.Reset()
	err = run(ctx, []string{"delete", "cli-test", "--wait"}, &stdout, &stderr, cl)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if !strings.Contains(stdout.String(), `Deleted PreviewEnvironment "cli-test"`) {
		t.Fatalf("unexpected delete output: %s", stdout.String())
	}
}
