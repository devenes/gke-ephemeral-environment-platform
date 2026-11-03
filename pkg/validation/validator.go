package validation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/devenes/gke-ephemeral-environment-platform/pkg/apis/platform/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var (
	dnsLabelRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	envNameRegex  = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)
	sha256Regex   = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

	reservedEnvKeys = map[string]bool{
		"PATH":                    true,
		"HOME":                    true,
		"USER":                    true,
		"LD_PRELOAD":              true,
		"LD_LIBRARY_PATH":         true,
		"KUBERNETES_SERVICE_HOST": true,
		"KUBERNETES_SERVICE_PORT": true,
		"PORT":                    true,
	}
)

// Policy defines the platform security and resource boundaries enforced by the controller.
type Policy struct {
	AllowedImagePrefixes []string
	MinTTLSeconds        int32
	MaxTTLSeconds        int32
	MaxReplicas          int32
	MaxPodCPU            resource.Quantity
	MaxPodMemory         resource.Quantity
	MaxNamespaceCPU      resource.Quantity
	MaxNamespaceMemory   resource.Quantity
}

// DefaultPolicy returns the standard platform policy for your-gcp-project-id.
func DefaultPolicy(allowedPrefixes ...string) Policy {
	prefixes := allowedPrefixes
	if len(prefixes) == 0 {
		prefixes = []string{
			"us-west4-docker.pkg.dev/your-gcp-project-id/gke-ephemeral-env/",
		}
	}
	return Policy{
		AllowedImagePrefixes: prefixes,
		MinTTLSeconds:        30,
		MaxTTLSeconds:        86400,
		MaxReplicas:          3,
		MaxPodCPU:            resource.MustParse("500m"),
		MaxPodMemory:         resource.MustParse("512Mi"),
		MaxNamespaceCPU:      resource.MustParse("1000m"),
		MaxNamespaceMemory:   resource.MustParse("1Gi"),
	}
}

// ValidationError carries a structured Reason and human-readable Message for status conditions.
type ValidationError struct {
	Reason  string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Reason, e.Message)
}

// NormalizedSpec holds the defaulted and parsed values ready for reconciliation.
type NormalizedSpec struct {
	EnvironmentName string
	Namespace       string
	Developer       string
	PRNumber        int32
	Repository      string
	CommitSHA       string
	Image           string
	TTLSeconds      int32
	Replicas        int32
	Port            int32
	PathPrefix      string
	CPURequest      resource.Quantity
	CPULimit        resource.Quantity
	MemoryRequest   resource.Quantity
	MemoryLimit     resource.Quantity
	Env             []v1alpha1.EnvVar
}

// Validate checks a PreviewEnvironment against the platform policy and returns a NormalizedSpec.
func Validate(pe *v1alpha1.PreviewEnvironment, policy Policy) (*NormalizedSpec, *ValidationError) {
	if pe == nil {
		return nil, &ValidationError{Reason: "NilResource", Message: "PreviewEnvironment cannot be nil"}
	}

	name := strings.TrimSpace(pe.Name)
	if name == "" || len(name) > 40 || !dnsLabelRegex.MatchString(name) {
		return nil, &ValidationError{
			Reason:  "InvalidEnvironmentName",
			Message: fmt.Sprintf("metadata.name %q must be a lowercase DNS-1035 label between 1 and 40 characters", pe.Name),
		}
	}

	envName := strings.TrimSpace(pe.Spec.EnvironmentName)
	if envName == "" {
		envName = name
	} else if envName != name {
		return nil, &ValidationError{
			Reason:  "InvalidEnvironmentName",
			Message: fmt.Sprintf("spec.environmentName %q must match metadata.name %q when specified", envName, name),
		}
	}

	if err := validateImage(pe.Spec.Image, policy.AllowedImagePrefixes); err != nil {
		return nil, err
	}

	ttl := pe.Spec.TTLSeconds
	if ttl == 0 {
		ttl = 3600
	}
	if ttl < policy.MinTTLSeconds || ttl > policy.MaxTTLSeconds {
		return nil, &ValidationError{
			Reason:  "InvalidTTL",
			Message: fmt.Sprintf("spec.ttlSeconds (%d) must be between %d and %d seconds", ttl, policy.MinTTLSeconds, policy.MaxTTLSeconds),
		}
	}

	replicas := pe.Spec.Replicas
	if replicas == 0 {
		replicas = 1
	}
	if replicas < 1 || replicas > policy.MaxReplicas {
		return nil, &ValidationError{
			Reason:  "InvalidReplicas",
			Message: fmt.Sprintf("spec.replicas (%d) must be between 1 and %d", replicas, policy.MaxReplicas),
		}
	}

	port := pe.Spec.Port
	if port == 0 {
		port = 8080
	}
	if port < 1024 || port > 65535 {
		return nil, &ValidationError{
			Reason:  "InvalidPort",
			Message: fmt.Sprintf("spec.port (%d) must be an unprivileged TCP port between 1024 and 65535", port),
		}
	}

	if pe.Spec.PRNumber < 0 {
		return nil, &ValidationError{
			Reason:  "InvalidPRNumber",
			Message: "spec.prNumber cannot be negative",
		}
	}

	cpuReqStr := defaultStr(pe.Spec.Resources.CPURequest, "50m")
	cpuLimStr := defaultStr(pe.Spec.Resources.CPULimit, "200m")
	memReqStr := defaultStr(pe.Spec.Resources.MemoryRequest, "64Mi")
	memLimStr := defaultStr(pe.Spec.Resources.MemoryLimit, "128Mi")

	cpuReq, err := resource.ParseQuantity(cpuReqStr)
	if err != nil || cpuReq.Sign() <= 0 {
		return nil, &ValidationError{
			Reason:  "InvalidResources",
			Message: fmt.Sprintf("invalid spec.resources.cpuRequest %q", cpuReqStr),
		}
	}
	cpuLim, err := resource.ParseQuantity(cpuLimStr)
	if err != nil || cpuLim.Sign() <= 0 {
		return nil, &ValidationError{
			Reason:  "InvalidResources",
			Message: fmt.Sprintf("invalid spec.resources.cpuLimit %q", cpuLimStr),
		}
	}
	memReq, err := resource.ParseQuantity(memReqStr)
	if err != nil || memReq.Sign() <= 0 {
		return nil, &ValidationError{
			Reason:  "InvalidResources",
			Message: fmt.Sprintf("invalid spec.resources.memoryRequest %q", memReqStr),
		}
	}
	memLim, err := resource.ParseQuantity(memLimStr)
	if err != nil || memLim.Sign() <= 0 {
		return nil, &ValidationError{
			Reason:  "InvalidResources",
			Message: fmt.Sprintf("invalid spec.resources.memoryLimit %q", memLimStr),
		}
	}

	if cpuReq.Cmp(cpuLim) > 0 {
		return nil, &ValidationError{
			Reason:  "InvalidResources",
			Message: fmt.Sprintf("cpuRequest (%s) cannot exceed cpuLimit (%s)", cpuReq.String(), cpuLim.String()),
		}
	}
	if memReq.Cmp(memLim) > 0 {
		return nil, &ValidationError{
			Reason:  "InvalidResources",
			Message: fmt.Sprintf("memoryRequest (%s) cannot exceed memoryLimit (%s)", memReq.String(), memLim.String()),
		}
	}

	if cpuLim.Cmp(policy.MaxPodCPU) > 0 {
		return nil, &ValidationError{
			Reason:  "ExcessiveResources",
			Message: fmt.Sprintf("per-pod cpuLimit (%s) exceeds platform maximum (%s)", cpuLim.String(), policy.MaxPodCPU.String()),
		}
	}
	if memLim.Cmp(policy.MaxPodMemory) > 0 {
		return nil, &ValidationError{
			Reason:  "ExcessiveResources",
			Message: fmt.Sprintf("per-pod memoryLimit (%s) exceeds platform maximum (%s)", memLim.String(), policy.MaxPodMemory.String()),
		}
	}

	totalCPULimMillis := cpuLim.MilliValue() * int64(replicas)
	if totalCPULimMillis > policy.MaxNamespaceCPU.MilliValue() {
		return nil, &ValidationError{
			Reason:  "ExcessiveResources",
			Message: fmt.Sprintf("total namespace CPU limit (%dm across %d replicas) exceeds namespace quota (%s)", totalCPULimMillis, replicas, policy.MaxNamespaceCPU.String()),
		}
	}

	totalMemLimBytes := memLim.Value() * int64(replicas)
	if totalMemLimBytes > policy.MaxNamespaceMemory.Value() {
		return nil, &ValidationError{
			Reason:  "ExcessiveResources",
			Message: fmt.Sprintf("total namespace memory limit (%d bytes across %d replicas) exceeds namespace quota (%s)", totalMemLimBytes, replicas, policy.MaxNamespaceMemory.String()),
		}
	}

	if len(pe.Spec.Env) > 20 {
		return nil, &ValidationError{
			Reason:  "InvalidEnv",
			Message: "spec.env cannot contain more than 20 variables",
		}
	}

	seenEnv := make(map[string]bool)
	for _, ev := range pe.Spec.Env {
		if !envNameRegex.MatchString(ev.Name) {
			return nil, &ValidationError{
				Reason:  "InvalidEnv",
				Message: fmt.Sprintf("environment variable name %q must match ^[A-Z_][A-Z0-9_]{0,63}$", ev.Name),
			}
		}
		if reservedEnvKeys[ev.Name] || strings.HasPrefix(ev.Name, "PREVIEW_") {
			return nil, &ValidationError{
				Reason:  "InvalidEnv",
				Message: fmt.Sprintf("environment variable name %q is reserved by the platform", ev.Name),
			}
		}
		if len(ev.Value) > 1024 {
			return nil, &ValidationError{
				Reason:  "InvalidEnv",
				Message: fmt.Sprintf("environment variable %q value exceeds 1024 bytes", ev.Name),
			}
		}
		if seenEnv[ev.Name] {
			return nil, &ValidationError{
				Reason:  "InvalidEnv",
				Message: fmt.Sprintf("duplicate environment variable name %q", ev.Name),
			}
		}
		seenEnv[ev.Name] = true
	}

	pathPrefix := strings.TrimSpace(pe.Spec.PathPrefix)
	if pathPrefix == "" {
		pathPrefix = "/preview/" + name
	}
	if !strings.HasPrefix(pathPrefix, "/preview/") || strings.Contains(pathPrefix, "..") {
		return nil, &ValidationError{
			Reason:  "InvalidPathPrefix",
			Message: fmt.Sprintf("spec.pathPrefix %q must start with /preview/ and not contain '..'", pathPrefix),
		}
	}

	return &NormalizedSpec{
		EnvironmentName: envName,
		Namespace:       "preview-" + name,
		Developer:       defaultStr(pe.Spec.Developer, "unknown"),
		PRNumber:        pe.Spec.PRNumber,
		Repository:      strings.TrimSpace(pe.Spec.Repository),
		CommitSHA:       strings.TrimSpace(pe.Spec.CommitSHA),
		Image:           strings.TrimSpace(pe.Spec.Image),
		TTLSeconds:      ttl,
		Replicas:        replicas,
		Port:            port,
		PathPrefix:      pathPrefix,
		CPURequest:      cpuReq,
		CPULimit:        cpuLim,
		MemoryRequest:   memReq,
		MemoryLimit:     memLim,
		Env:             pe.Spec.Env,
	}, nil
}

func validateImage(rawImage string, allowedPrefixes []string) *ValidationError {
	img := strings.TrimSpace(rawImage)
	if img == "" {
		return &ValidationError{
			Reason:  "DisallowedImage",
			Message: "spec.image is required",
		}
	}
	if strings.ContainsAny(img, " \t\n\r") {
		return &ValidationError{
			Reason:  "DisallowedImage",
			Message: "spec.image must not contain whitespace",
		}
	}

	matchedPrefix := ""
	for _, p := range allowedPrefixes {
		if strings.HasPrefix(img, p) {
			matchedPrefix = p
			break
		}
	}
	if matchedPrefix == "" {
		return &ValidationError{
			Reason:  "DisallowedImage",
			Message: fmt.Sprintf("image %q is not from an allowed Artifact Registry prefix (%v)", img, allowedPrefixes),
		}
	}

	remainder := strings.TrimPrefix(img, matchedPrefix)
	if remainder == "" {
		return &ValidationError{
			Reason:  "DisallowedImage",
			Message: "image must include a repository name and immutable tag or @sha256 digest",
		}
	}

	// Check for @sha256:... digest or :tag
	if atIdx := strings.IndexByte(remainder, '@'); atIdx != -1 {
		repoPart := remainder[:atIdx]
		digestPart := remainder[atIdx+1:]
		if repoPart == "" || !sha256Regex.MatchString(digestPart) {
			return &ValidationError{
				Reason:  "DisallowedImage",
				Message: fmt.Sprintf("image %q has an invalid sha256 digest format", img),
			}
		}
		return nil
	}

	colonIdx := strings.LastIndexByte(remainder, ':')
	if colonIdx <= 0 || colonIdx == len(remainder)-1 {
		return &ValidationError{
			Reason:  "DisallowedImage",
			Message: fmt.Sprintf("image %q must specify an explicit version tag or @sha256 digest", img),
		}
	}
	tag := remainder[colonIdx+1:]
	if strings.EqualFold(tag, "latest") {
		return &ValidationError{
			Reason:  "DisallowedImage",
			Message: "mutable image tag ':latest' is not allowed; use a pinned version tag or @sha256 digest",
		}
	}

	return nil
}

func defaultStr(val, fallback string) string {
	if v := strings.TrimSpace(val); v != "" {
		return v
	}
	return fallback
}
