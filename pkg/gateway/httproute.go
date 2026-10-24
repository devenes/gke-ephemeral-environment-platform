package gateway

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GatewayGVK = schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "Gateway",
	}
	HTTPRouteGVK = schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "HTTPRoute",
	}
)

// RouteParams holds the parameters for constructing a per-environment HTTPRoute.
type RouteParams struct {
	EnvironmentName  string
	Namespace        string
	RouteName        string
	GatewayName      string
	GatewayNamespace string
	ServiceName      string
	ServicePort      int32
	PathPrefix       string
	Labels           map[string]string
}

// BuildHTTPRoute constructs an unstructured gateway.networking.k8s.io/v1 HTTPRoute resource.
func BuildHTTPRoute(p RouteParams) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(HTTPRouteGVK)
	u.SetName(p.RouteName)
	u.SetNamespace(p.Namespace)
	u.SetLabels(p.Labels)

	u.Object["spec"] = BuildHTTPRouteSpec(p)
	return u
}

// BuildHTTPRouteSpec constructs the spec map for an HTTPRoute with PathPrefix matching and URLRewrite.
func BuildHTTPRouteSpec(p RouteParams) map[string]any {
	return map[string]any{
		"parentRefs": []any{
			map[string]any{
				"group":     "gateway.networking.k8s.io",
				"kind":      "Gateway",
				"name":      p.GatewayName,
				"namespace": p.GatewayNamespace,
			},
		},
		"rules": []any{
			map[string]any{
				"matches": []any{
					map[string]any{
						"path": map[string]any{
							"type":  "PathPrefix",
							"value": p.PathPrefix,
						},
					},
				},
				"filters": []any{
					map[string]any{
						"type": "URLRewrite",
						"urlRewrite": map[string]any{
							"path": map[string]any{
								"type":               "ReplacePrefixMatch",
								"replacePrefixMatch": "/",
							},
						},
					},
				},
				"backendRefs": []any{
					map[string]any{
						"group":  "",
						"kind":   "Service",
						"name":   p.ServiceName,
						"port":   int64(p.ServicePort),
						"weight": int64(1),
					},
				},
			},
		},
	}
}

// ExtractGatewayAddress reads the first assigned IP/hostname from Gateway status.addresses.
func ExtractGatewayAddress(gw *unstructured.Unstructured) (string, bool) {
	if gw == nil {
		return "", false
	}
	addresses, found, err := unstructured.NestedSlice(gw.Object, "status", "addresses")
	if err != nil || !found {
		return "", false
	}
	for _, item := range addresses {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if val, ok := m["value"].(string); ok && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val), true
		}
	}
	return "", false
}

// IsHTTPRouteAccepted inspects status.parents[].conditions on an HTTPRoute to determine if Accepted=True.
func IsHTTPRouteAccepted(hr *unstructured.Unstructured) (bool, string, string) {
	if hr == nil {
		return false, "RouteNotFound", "HTTPRoute object is nil"
	}
	parents, found, err := unstructured.NestedSlice(hr.Object, "status", "parents")
	if err != nil || !found || len(parents) == 0 {
		return false, "PendingParentAcceptance", "Waiting for GKE Gateway controller to report parent status on HTTPRoute"
	}

	for _, p := range parents {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		conds, ok := pm["conditions"].([]any)
		if !ok {
			continue
		}
		for _, c := range conds {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			cType, _ := cm["type"].(string)
			cStatus, _ := cm["status"].(string)
			cReason, _ := cm["reason"].(string)
			cMsg, _ := cm["message"].(string)
			if cType == "Accepted" {
				if cStatus == "True" {
					if cReason == "" {
						cReason = "Accepted"
					}
					if cMsg == "" {
						cMsg = "HTTPRoute accepted by shared Gateway"
					}
					return true, cReason, cMsg
				}
				return false, defaultReason(cReason, "NotAccepted"), fmt.Sprintf("HTTPRoute Accepted=%s: %s", cStatus, cMsg)
			}
		}
	}

	return false, "PendingParentAcceptance", "Waiting for Accepted condition on HTTPRoute parent status"
}

func defaultReason(r, fallback string) string {
	if strings.TrimSpace(r) != "" {
		return r
	}
	return fallback
}
