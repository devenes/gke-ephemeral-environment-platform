package gateway

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuildAndInspectHTTPRoute(t *testing.T) {
	hr := BuildHTTPRoute(RouteParams{
		EnvironmentName:  "pr-10",
		Namespace:        "preview-pr-10",
		RouteName:        "preview-route",
		GatewayName:      "preview-shared-gateway",
		GatewayNamespace: "preview-platform-system",
		ServiceName:      "preview-svc",
		ServicePort:      80,
		PathPrefix:       "/preview/pr-10",
		Labels:           map[string]string{"platform.gke.io/environment": "pr-10"},
	})

	if hr.GetName() != "preview-route" || hr.GetNamespace() != "preview-pr-10" {
		t.Fatalf("unexpected route metadata: %s/%s", hr.GetNamespace(), hr.GetName())
	}

	accepted, reason, _ := IsHTTPRouteAccepted(hr)
	if accepted || reason != "PendingParentAcceptance" {
		t.Fatalf("expected unaccepted route initially, got accepted=%v reason=%s", accepted, reason)
	}

	hr.Object["status"] = map[string]any{
		"parents": []any{
			map[string]any{
				"conditions": []any{
					map[string]any{
						"type":    "Accepted",
						"status":  "True",
						"reason":  "Accepted",
						"message": "Route accepted",
					},
				},
			},
		},
	}

	accepted, reason, _ = IsHTTPRouteAccepted(hr)
	if !accepted || reason != "Accepted" {
		t.Fatalf("expected accepted route, got accepted=%v reason=%s", accepted, reason)
	}

	gw := &unstructured.Unstructured{
		Object: map[string]any{
			"status": map[string]any{
				"addresses": []any{
					map[string]any{
						"type":  "IPAddress",
						"value": "34.125.10.20",
					},
				},
			},
		},
	}

	ip, ok := ExtractGatewayAddress(gw)
	if !ok || ip != "34.125.10.20" {
		t.Fatalf("expected 34.125.10.20, got %q (ok=%v)", ip, ok)
	}
}
