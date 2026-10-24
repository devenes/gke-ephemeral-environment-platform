package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	GroupName    = "platform.gke.io"
	GroupVersion = "v1alpha1"
	Kind         = "PreviewEnvironment"
	Finalizer    = "preview.platform.gke.io/finalizer"
)

// PreviewPhase represents the lifecycle state of a PreviewEnvironment.
type PreviewPhase string

const (
	PhasePending      PreviewPhase = "Pending"
	PhaseProvisioning PreviewPhase = "Provisioning"
	PhaseReady        PreviewPhase = "Ready"
	PhaseFailed       PreviewPhase = "Failed"
	PhaseDeleting     PreviewPhase = "Deleting"
	PhaseExpired      PreviewPhase = "Expired"
)

// ConditionType constants for PreviewEnvironment status conditions.
const (
	ConditionValidated      = "Validated"
	ConditionNamespaceReady = "NamespaceReady"
	ConditionWorkloadReady  = "WorkloadReady"
	ConditionRouteAccepted  = "RouteAccepted"
	ConditionReady          = "Ready"
)

// EnvVar defines a controlled environment variable for the preview workload.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ResourceSpec defines CPU and memory requests/limits for each preview pod replica.
type ResourceSpec struct {
	CPURequest    string `json:"cpuRequest,omitempty"`
	CPULimit      string `json:"cpuLimit,omitempty"`
	MemoryRequest string `json:"memoryRequest,omitempty"`
	MemoryLimit   string `json:"memoryLimit,omitempty"`
}

// PreviewEnvironmentSpec defines the desired state of a PreviewEnvironment.
type PreviewEnvironmentSpec struct {
	// EnvironmentName is an optional explicit identifier; defaults to metadata.name.
	EnvironmentName string `json:"environmentName,omitempty"`

	// Developer is the username or identifier of the requester.
	Developer string `json:"developer,omitempty"`

	// PRNumber is the associated GitHub Pull Request number.
	PRNumber int32 `json:"prNumber,omitempty"`

	// Repository is the GitHub owner/repo string (e.g. devenes/gke-ephemeral-environment-platform).
	Repository string `json:"repository,omitempty"`

	// CommitSHA is the Git commit SHA being previewed.
	CommitSHA string `json:"commitSHA,omitempty"`

	// Image is the container image reference from the allowed Artifact Registry repository.
	Image string `json:"image"`

	// TTLSeconds is the time-to-live in seconds from creation before automatic deletion.
	TTLSeconds int32 `json:"ttlSeconds,omitempty"`

	// Replicas is the desired pod count (1..3).
	Replicas int32 `json:"replicas,omitempty"`

	// Port is the container HTTP listening port (1024..65535, default 8080).
	Port int32 `json:"port,omitempty"`

	// PathPrefix is the optional custom HTTP route path prefix; defaults to /preview/<name>.
	PathPrefix string `json:"pathPrefix,omitempty"`

	// Resources specifies per-pod CPU and memory requests and limits.
	Resources ResourceSpec `json:"resources,omitempty"`

	// Env specifies controlled environment variables exposed to the application container with APP_CUSTOM_ prefix.
	Env []EnvVar `json:"env,omitempty"`
}

// PreviewEnvironmentStatus defines the observed state of a PreviewEnvironment.
type PreviewEnvironmentStatus struct {
	// Phase is the high-level lifecycle summary.
	Phase PreviewPhase `json:"phase,omitempty"`

	// Message provides a human-readable summary of the current phase or error.
	Message string `json:"message,omitempty"`

	// ObservedGeneration is the most recent metadata.generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Namespace is the isolated tenant namespace created for this environment.
	Namespace string `json:"namespace,omitempty"`

	// DeploymentName is the managed Deployment name in the tenant namespace.
	DeploymentName string `json:"deploymentName,omitempty"`

	// ServiceName is the managed Service name in the tenant namespace.
	ServiceName string `json:"serviceName,omitempty"`

	// HTTPRouteName is the managed HTTPRoute name in the tenant namespace.
	HTTPRouteName string `json:"httpRouteName,omitempty"`

	// PreviewURL is the external HTTP URL routed through the shared GKE Gateway.
	PreviewURL string `json:"previewURL,omitempty"`

	// ReadyReplicas is the number of ready pods in the tenant Deployment.
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// CreatedAt records when the controller first initialized the environment.
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// ReadyAt records when the environment first reached the Ready phase.
	ReadyAt *metav1.Time `json:"readyAt,omitempty"`

	// ExpiresAt records when the environment's TTL expires.
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// CleanupStatus describes the progress of TTL or finalizer cleanup.
	CleanupStatus string `json:"cleanupStatus,omitempty"`

	// Conditions represent the latest available observations of the environment's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// PreviewEnvironment is the Schema for the previewenvironments API.
type PreviewEnvironment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PreviewEnvironmentSpec   `json:"spec,omitempty"`
	Status PreviewEnvironmentStatus `json:"status,omitempty"`
}

// PreviewEnvironmentList contains a list of PreviewEnvironment.
type PreviewEnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PreviewEnvironment `json:"items"`
}
