package controller

import (
	"context"
	"testing"
	"time"

	"github.com/devenes/gke-ephemeral-environment-platform/pkg/apis/platform/v1alpha1"
	"github.com/devenes/gke-ephemeral-environment-platform/pkg/validation"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func setupTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add clientgo scheme: %v", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add v1alpha1 scheme: %v", err)
	}
	return scheme
}

func TestReconcilerProvisionAndReady(t *testing.T) {
	scheme := setupTestScheme(t)
	now := metav1.Now()
	pe := &v1alpha1.PreviewEnvironment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pr-200",
			CreationTimestamp: now,
			Generation:        1,
		},
		Spec: v1alpha1.PreviewEnvironmentSpec{
			Developer:  "devenes",
			PRNumber:   200,
			Repository: "devenes/gke-ephemeral-environment-platform",
			CommitSHA:  "1234567",
			Image:      "us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/sample-app:v0.1.0",
			TTLSeconds: 600,
			Replicas:   1,
			Port:       8080,
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.PreviewEnvironment{}).
		WithObjects(pe).
		Build()

	rec := &PreviewEnvironmentReconciler{
		Client: cl,
		Scheme: scheme,
		Config: ReconcilerConfig{
			Policy:           validation.DefaultPolicy("us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/"),
			GatewayName:      "preview-shared-gateway",
			GatewayNamespace: "preview-platform-system",
			ProxySubnetCIDR:  "10.42.16.0/23",
			SkipHTTPRoute:    true,
		},
	}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "pr-200"}}

	// First reconcile provisions resources and transitions to Provisioning
	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	var updated v1alpha1.PreviewEnvironment
	if err := cl.Get(ctx, req.NamespacedName, &updated); err != nil {
		t.Fatalf("get updated PE: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseProvisioning {
		t.Fatalf("expected phase Provisioning, got %s", updated.Status.Phase)
	}
	if updated.Status.Namespace != "preview-pr-200" {
		t.Fatalf("expected namespace preview-pr-200, got %s", updated.Status.Namespace)
	}

	// Verify child resources exist
	var ns corev1.Namespace
	if err := cl.Get(ctx, types.NamespacedName{Name: "preview-pr-200"}, &ns); err != nil {
		t.Fatalf("expected namespace to exist: %v", err)
	}
	var rq corev1.ResourceQuota
	if err := cl.Get(ctx, types.NamespacedName{Name: ResourceQuotaName, Namespace: "preview-pr-200"}, &rq); err != nil {
		t.Fatalf("expected ResourceQuota to exist: %v", err)
	}
	var lr corev1.LimitRange
	if err := cl.Get(ctx, types.NamespacedName{Name: LimitRangeName, Namespace: "preview-pr-200"}, &lr); err != nil {
		t.Fatalf("expected LimitRange to exist: %v", err)
	}
	var np networkingv1.NetworkPolicy
	if err := cl.Get(ctx, types.NamespacedName{Name: NetworkPolicyName, Namespace: "preview-pr-200"}, &np); err != nil {
		t.Fatalf("expected NetworkPolicy to exist: %v", err)
	}
	var svc corev1.Service
	if err := cl.Get(ctx, types.NamespacedName{Name: ServiceName, Namespace: "preview-pr-200"}, &svc); err != nil {
		t.Fatalf("expected Service to exist: %v", err)
	}

	// Mark deployment ready and reconcile again -> should transition to Ready
	var dep appsv1.Deployment
	if err := cl.Get(ctx, types.NamespacedName{Name: DeploymentName, Namespace: "preview-pr-200"}, &dep); err != nil {
		t.Fatalf("expected Deployment to exist: %v", err)
	}
	dep.Status.ReadyReplicas = 1
	dep.Status.UpdatedReplicas = 1
	if err := cl.Status().Update(ctx, &dep); err != nil {
		t.Fatalf("update deployment status: %v", err)
	}

	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	if err := cl.Get(ctx, req.NamespacedName, &updated); err != nil {
		t.Fatalf("get updated PE: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseReady {
		t.Fatalf("expected phase Ready, got %s", updated.Status.Phase)
	}
	if updated.Status.PreviewURL != "http://127.0.0.1/preview/pr-200" {
		t.Fatalf("unexpected previewURL: %s", updated.Status.PreviewURL)
	}
}

func TestReconcilerValidationFailure(t *testing.T) {
	scheme := setupTestScheme(t)
	pe := &v1alpha1.PreviewEnvironment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "bad-env",
			Generation: 1,
		},
		Spec: v1alpha1.PreviewEnvironmentSpec{
			Image: "docker.io/library/alpine:3.19",
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.PreviewEnvironment{}).
		WithObjects(pe).
		Build()

	rec := &PreviewEnvironmentReconciler{
		Client: cl,
		Scheme: scheme,
		Config: ReconcilerConfig{
			Policy:        validation.DefaultPolicy("us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/"),
			SkipHTTPRoute: true,
		},
	}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "bad-env"}}
	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var updated v1alpha1.PreviewEnvironment
	if err := cl.Get(ctx, req.NamespacedName, &updated); err != nil {
		t.Fatalf("get PE: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseFailed {
		t.Fatalf("expected phase Failed, got %s", updated.Status.Phase)
	}

	var ns corev1.Namespace
	err := cl.Get(ctx, types.NamespacedName{Name: "preview-bad-env"}, &ns)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected no namespace to be created for invalid PE, got err=%v", err)
	}
}

func TestReconcilerTTLExpiration(t *testing.T) {
	scheme := setupTestScheme(t)
	past := metav1.NewTime(time.Now().Add(-5 * time.Minute))
	pe := &v1alpha1.PreviewEnvironment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "expired-env",
			CreationTimestamp: past,
			Generation:        1,
		},
		Spec: v1alpha1.PreviewEnvironmentSpec{
			Image:      "us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/sample-app:v0.1.0",
			TTLSeconds: 60,
		},
		Status: v1alpha1.PreviewEnvironmentStatus{
			CreatedAt: &past,
			ReadyAt:   &past,
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.PreviewEnvironment{}).
		WithObjects(pe).
		Build()

	rec := &PreviewEnvironmentReconciler{
		Client: cl,
		Scheme: scheme,
		Config: ReconcilerConfig{
			Policy:        validation.DefaultPolicy("us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/"),
			SkipHTTPRoute: true,
		},
	}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "expired-env"}}

	// First reconcile adds finalizer, detects TTL expired, and triggers Delete
	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	// Second reconcile processes deletionTimestamp and removes finalizer
	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	var check v1alpha1.PreviewEnvironment
	err := cl.Get(ctx, req.NamespacedName, &check)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected expired PE to be deleted, got err=%v (phase=%s)", err, check.Status.Phase)
	}
}
