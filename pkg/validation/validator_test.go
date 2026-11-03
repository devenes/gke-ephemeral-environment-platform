package validation

import (
	"testing"

	"github.com/devenes/gke-ephemeral-environment-platform/pkg/apis/platform/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidatePreviewEnvironment(t *testing.T) {
	policy := DefaultPolicy("us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/")
	validImage := "us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/sample-app:v0.1.0"

	tests := []struct {
		name       string
		pe         *v1alpha1.PreviewEnvironment
		wantReason string
	}{
		{
			name: "valid minimal request with defaults",
			pe: &v1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "pr-101"},
				Spec: v1alpha1.PreviewEnvironmentSpec{
					Image: validImage,
				},
			},
			wantReason: "",
		},
		{
			name: "valid digest image and custom env",
			pe: &v1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "pr-102"},
				Spec: v1alpha1.PreviewEnvironmentSpec{
					Image:      "us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/sample-app@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					TTLSeconds: 600,
					Replicas:   2,
					Port:       8080,
					Resources: v1alpha1.ResourceSpec{
						CPURequest:    "100m",
						CPULimit:      "300m",
						MemoryRequest: "128Mi",
						MemoryLimit:   "256Mi",
					},
					Env: []v1alpha1.EnvVar{
						{Name: "FEATURE_BANNER", Value: "enabled"},
					},
				},
			},
			wantReason: "",
		},
		{
			name: "reject disallowed image registry",
			pe: &v1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "bad-reg"},
				Spec: v1alpha1.PreviewEnvironmentSpec{
					Image: "docker.io/library/nginx:1.27",
				},
			},
			wantReason: "DisallowedImage",
		},
		{
			name: "reject latest tag",
			pe: &v1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "bad-tag"},
				Spec: v1alpha1.PreviewEnvironmentSpec{
					Image: "us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/sample-app:latest",
				},
			},
			wantReason: "DisallowedImage",
		},
		{
			name: "reject untagged image",
			pe: &v1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "no-tag"},
				Spec: v1alpha1.PreviewEnvironmentSpec{
					Image: "us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/sample-app",
				},
			},
			wantReason: "DisallowedImage",
		},
		{
			name: "reject invalid environment name",
			pe: &v1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "Invalid_Name!"},
				Spec: v1alpha1.PreviewEnvironmentSpec{
					Image: validImage,
				},
			},
			wantReason: "InvalidEnvironmentName",
		},
		{
			name: "reject TTL below minimum",
			pe: &v1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "short-ttl"},
				Spec: v1alpha1.PreviewEnvironmentSpec{
					Image:      validImage,
					TTLSeconds: 5,
				},
			},
			wantReason: "InvalidTTL",
		},
		{
			name: "reject TTL above maximum",
			pe: &v1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "long-ttl"},
				Spec: v1alpha1.PreviewEnvironmentSpec{
					Image:      validImage,
					TTLSeconds: 100000,
				},
			},
			wantReason: "InvalidTTL",
		},
		{
			name: "reject excessive replicas",
			pe: &v1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "many-replicas"},
				Spec: v1alpha1.PreviewEnvironmentSpec{
					Image:    validImage,
					Replicas: 5,
				},
			},
			wantReason: "InvalidReplicas",
		},
		{
			name: "reject privileged port",
			pe: &v1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "priv-port"},
				Spec: v1alpha1.PreviewEnvironmentSpec{
					Image: validImage,
					Port:  80,
				},
			},
			wantReason: "InvalidPort",
		},
		{
			name: "reject excessive CPU limit",
			pe: &v1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "high-cpu"},
				Spec: v1alpha1.PreviewEnvironmentSpec{
					Image: validImage,
					Resources: v1alpha1.ResourceSpec{
						CPURequest: "100m",
						CPULimit:   "2000m",
					},
				},
			},
			wantReason: "ExcessiveResources",
		},
		{
			name: "reject excessive memory limit",
			pe: &v1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "high-mem"},
				Spec: v1alpha1.PreviewEnvironmentSpec{
					Image: validImage,
					Resources: v1alpha1.ResourceSpec{
						MemoryRequest: "128Mi",
						MemoryLimit:   "4Gi",
					},
				},
			},
			wantReason: "ExcessiveResources",
		},
		{
			name: "reject request greater than limit",
			pe: &v1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "req-gt-lim"},
				Spec: v1alpha1.PreviewEnvironmentSpec{
					Image: validImage,
					Resources: v1alpha1.ResourceSpec{
						CPURequest: "300m",
						CPULimit:   "100m",
					},
				},
			},
			wantReason: "InvalidResources",
		},
		{
			name: "reject reserved env variable",
			pe: &v1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "bad-env"},
				Spec: v1alpha1.PreviewEnvironmentSpec{
					Image: validImage,
					Env: []v1alpha1.EnvVar{
						{Name: "LD_PRELOAD", Value: "/tmp/evil.so"},
					},
				},
			},
			wantReason: "InvalidEnv",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			norm, verr := Validate(tc.pe, policy)
			if tc.wantReason == "" {
				if verr != nil {
					t.Fatalf("expected valid spec, got error: %v", verr)
				}
				if norm == nil || norm.Namespace != "preview-"+tc.pe.Name {
					t.Fatalf("unexpected normalized spec: %+v", norm)
				}
			} else {
				if verr == nil {
					t.Fatalf("expected validation error with reason %q, got nil", tc.wantReason)
				}
				if verr.Reason != tc.wantReason {
					t.Fatalf("got reason %q, want %q (msg: %s)", verr.Reason, tc.wantReason, verr.Message)
				}
			}
		})
	}
}
