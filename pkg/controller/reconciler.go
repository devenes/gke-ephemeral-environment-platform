package controller

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/devenes/gke-ephemeral-environment-platform/pkg/apis/platform/v1alpha1"
	"github.com/devenes/gke-ephemeral-environment-platform/pkg/gateway"
	"github.com/devenes/gke-ephemeral-environment-platform/pkg/metrics"
	"github.com/devenes/gke-ephemeral-environment-platform/pkg/validation"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	LabelManagedBy       = "platform.gke.io/managed-by"
	LabelManagedByValue  = "preview-controller"
	LabelEnvironmentName = "platform.gke.io/environment"
	LabelPRNumber        = "platform.gke.io/pr-number"

	ResourceQuotaName = "preview-quota"
	LimitRangeName    = "preview-limits"
	NetworkPolicyName = "preview-isolation"
	DeploymentName    = "preview-app"
	ServiceName       = "preview-svc"
	HTTPRouteName     = "preview-route"

	minProvisioningGraceSeconds = 180
)

// ReconcilerConfig configures the PreviewEnvironmentReconciler.
type ReconcilerConfig struct {
	Policy           validation.Policy
	GatewayName      string
	GatewayNamespace string
	ProxySubnetCIDR  string
	SkipHTTPRoute    bool // Used only in unit tests without Gateway API CRDs registered
}

// PreviewEnvironmentReconciler reconciles PreviewEnvironment custom resources.
type PreviewEnvironmentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config ReconcilerConfig
}

func (r *PreviewEnvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("previewEnvironment", req.Name)

	var pe v1alpha1.PreviewEnvironment
	if err := r.Get(ctx, req.NamespacedName, &pe); err != nil {
		if apierrors.IsNotFound(err) {
			r.refreshPhaseMetrics(ctx)
			return ctrl.Result{}, nil
		}
		metrics.ReconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	defer r.refreshPhaseMetrics(ctx)

	// 1. Handle deletion if DeletionTimestamp is non-zero
	if !pe.DeletionTimestamp.IsZero() {
		res, err := r.reconcileDelete(ctx, &pe)
		if err != nil {
			metrics.ReconcileTotal.WithLabelValues("error").Inc()
			metrics.CleanupFailuresTotal.Inc()
			return res, err
		}
		metrics.ReconcileTotal.WithLabelValues("success").Inc()
		return res, nil
	}

	// 2. Validate spec before creating any resources or adding finalizers
	norm, verr := validation.Validate(&pe, r.Config.Policy)
	if verr != nil {
		logger.Info("validation failed for PreviewEnvironment", "reason", verr.Reason, "message", verr.Message)
		if err := r.updateStatusFailed(ctx, &pe, verr.Reason, verr.Message); err != nil {
			metrics.ReconcileTotal.WithLabelValues("error").Inc()
			return ctrl.Result{}, err
		}
		metrics.ReconcileTotal.WithLabelValues("success").Inc()
		return ctrl.Result{}, nil
	}

	// 3. Ensure Finalizer is present
	if !controllerutil.ContainsFinalizer(&pe, v1alpha1.Finalizer) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			var latest v1alpha1.PreviewEnvironment
			if err := r.Get(ctx, req.NamespacedName, &latest); err != nil {
				return err
			}
			if !controllerutil.ContainsFinalizer(&latest, v1alpha1.Finalizer) {
				controllerutil.AddFinalizer(&latest, v1alpha1.Finalizer)
				return r.Update(ctx, &latest)
			}
			return nil
		})
		if err != nil {
			metrics.ReconcileTotal.WithLabelValues("error").Inc()
			return ctrl.Result{}, err
		}
		if err := r.Get(ctx, req.NamespacedName, &pe); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 4. Compute CreatedAt and ExpiresAt timestamps.
	// Once ReadyAt is recorded, TTL is anchored from ReadyAt so slow external load-balancer
	// route programming never consumes the environment's active preview TTL.
	now := metav1.Now()
	createdTime := pe.CreationTimestamp
	if createdTime.IsZero() {
		createdTime = now
	}
	if pe.Status.CreatedAt != nil && !pe.Status.CreatedAt.IsZero() {
		createdTime = *pe.Status.CreatedAt
	}

	var expiresTime metav1.Time
	var effectiveDeadline metav1.Time
	if pe.Status.ReadyAt != nil && !pe.Status.ReadyAt.IsZero() {
		expiresTime = metav1.NewTime(pe.Status.ReadyAt.Add(time.Duration(norm.TTLSeconds) * time.Second))
		effectiveDeadline = expiresTime
	} else {
		expiresTime = metav1.NewTime(createdTime.Add(time.Duration(norm.TTLSeconds) * time.Second))
		graceSecs := norm.TTLSeconds
		if graceSecs < minProvisioningGraceSeconds {
			graceSecs = minProvisioningGraceSeconds
		}
		effectiveDeadline = metav1.NewTime(createdTime.Add(time.Duration(graceSecs) * time.Second))
	}

	// 5. Evaluate TTL Expiration
	if !now.Before(&effectiveDeadline) {
		delaySeconds := now.Sub(effectiveDeadline.Time).Seconds()
		if delaySeconds >= 0 {
			metrics.TTLCleanupDelaySeconds.Observe(delaySeconds)
		}
		logger.Info("PreviewEnvironment TTL expired; initiating cleanup and deletion",
			"expiresAt", effectiveDeadline.Format(time.RFC3339),
			"delaySeconds", fmt.Sprintf("%.2f", delaySeconds),
		)
		_ = r.updateStatusExpired(ctx, &pe, &createdTime, &effectiveDeadline)
		if err := r.Delete(ctx, &pe); err != nil && !apierrors.IsNotFound(err) {
			metrics.CleanupFailuresTotal.Inc()
			metrics.ReconcileTotal.WithLabelValues("error").Inc()
			return ctrl.Result{RequeueAfter: 2 * time.Second}, err
		}
		metrics.ReconcileTotal.WithLabelValues("success").Inc()
		return ctrl.Result{}, nil
	}

	// 6. Reconcile isolated tenant Namespace and child resources
	if err := r.reconcileNamespace(ctx, &pe, norm); err != nil {
		metrics.ReconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	if err := r.reconcileResourceQuota(ctx, norm); err != nil {
		metrics.ReconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	if err := r.reconcileLimitRange(ctx, norm); err != nil {
		metrics.ReconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	if err := r.reconcileNetworkPolicy(ctx, norm); err != nil {
		metrics.ReconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	dep, err := r.reconcileDeployment(ctx, norm)
	if err != nil {
		metrics.ReconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	if err := r.reconcileService(ctx, norm); err != nil {
		metrics.ReconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	routeAccepted := true
	routeReason := "Accepted"
	routeMsg := "HTTPRoute accepted by shared Gateway"
	gatewayIP := ""

	if !r.Config.SkipHTTPRoute {
		hr, err := r.reconcileHTTPRoute(ctx, norm)
		if err != nil {
			metrics.ReconcileTotal.WithLabelValues("error").Inc()
			return ctrl.Result{}, err
		}
		routeAccepted, routeReason, routeMsg = gateway.IsHTTPRouteAccepted(hr)
		gatewayIP = r.lookupGatewayIP(ctx)
	} else {
		gatewayIP = "127.0.0.1"
	}

	// 7. Compute readiness and update status
	workloadReady := dep != nil && dep.Status.ReadyReplicas >= norm.Replicas && dep.Status.UpdatedReplicas >= norm.Replicas
	allReady := workloadReady && routeAccepted && gatewayIP != ""

	previewURL := ""
	if gatewayIP != "" {
		previewURL = fmt.Sprintf("http://%s%s", gatewayIP, norm.PathPrefix)
	}

	wasReady := pe.Status.ReadyAt != nil && !pe.Status.ReadyAt.IsZero()
	if allReady && !wasReady {
		expiresTime = metav1.NewTime(now.Add(time.Duration(norm.TTLSeconds) * time.Second))
	}

	if err := r.updateStatusProgress(ctx, &pe, norm, &createdTime, &expiresTime, dep, workloadReady, routeAccepted, routeReason, routeMsg, previewURL, allReady); err != nil {
		metrics.ReconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	if allReady && !wasReady {
		dur := time.Since(createdTime.Time).Seconds()
		if dur >= 0 {
			metrics.ProvisioningDurationSeconds.Observe(dur)
		}
		logger.Info("PreviewEnvironment is Ready", "previewURL", previewURL, "provisioningSeconds", fmt.Sprintf("%.2f", dur))
	}

	remainingTTL := time.Until(expiresTime.Time)
	if remainingTTL <= 0 {
		return ctrl.Result{RequeueAfter: 500 * time.Millisecond}, nil
	}

	if !allReady {
		metrics.ReconcileTotal.WithLabelValues("requeue").Inc()
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	metrics.ReconcileTotal.WithLabelValues("success").Inc()
	requeueAfter := 15 * time.Second
	if remainingTTL < requeueAfter {
		requeueAfter = remainingTTL
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *PreviewEnvironmentReconciler) reconcileDelete(ctx context.Context, pe *v1alpha1.PreviewEnvironment) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("previewEnvironment", pe.Name)
	nsName := pe.Status.Namespace
	if nsName == "" {
		nsName = "preview-" + pe.Name
	}

	_ = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest v1alpha1.PreviewEnvironment
		if err := r.Get(ctx, types.NamespacedName{Name: pe.Name}, &latest); err != nil {
			return err
		}
		if latest.Status.Phase != v1alpha1.PhaseDeleting {
			latest.Status.Phase = v1alpha1.PhaseDeleting
			latest.Status.CleanupStatus = fmt.Sprintf("Deleting tenant namespace %s and child resources", nsName)
			latest.Status.Message = "Environment teardown in progress"
			return r.Status().Update(ctx, &latest)
		}
		return nil
	})

	// Stage 1: Delete HTTPRoute and Deployment first while keeping Service/Namespace intact
	// so GKE's Gateway controller detaches the route and deletes the BackendService cleanly
	// before gke-neg-controller deletes the NetworkEndpointGroup.
	if !r.Config.SkipHTTPRoute {
		hr := &unstructured.Unstructured{}
		hr.SetGroupVersionKind(gateway.HTTPRouteGVK)
		hr.SetNamespace(nsName)
		hr.SetName(HTTPRouteName)
		_ = client.IgnoreNotFound(r.Delete(ctx, hr))
	}
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: DeploymentName, Namespace: nsName}}
	_ = client.IgnoreNotFound(r.Delete(ctx, dep))

	var ns corev1.Namespace
	nsErr := r.Get(ctx, types.NamespacedName{Name: nsName}, &ns)
	if nsErr == nil {
		if ns.DeletionTimestamp.IsZero() && !r.Config.SkipHTTPRoute {
			deletingFor := time.Since(pe.DeletionTimestamp.Time)
			if r.gatewayStillReferencesNamespace(ctx, nsName) && deletingFor < 35*time.Second {
				logger.Info("waiting for shared Gateway to detach tenant backend service before deleting Service/Namespace", "namespace", nsName)
				return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
			}
		}

		// Stage 2: Delete Service and Namespace
		svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: ServiceName, Namespace: nsName}}
		_ = client.IgnoreNotFound(r.Delete(ctx, svc))

		if ns.DeletionTimestamp.IsZero() {
			if err := r.Delete(ctx, &ns); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{RequeueAfter: 2 * time.Second}, err
			}
		} else if time.Since(ns.DeletionTimestamp.Time) > 15*time.Second && !r.gatewayStillReferencesNamespace(ctx, nsName) {
			// Stage 3: Self-heal any orphaned ServiceNetworkEndpointGroup finalizer if namespace termination stalls
			r.clearStuckSVCNEGFinalizers(ctx, nsName)
		}

		logger.Info("waiting for tenant namespace termination before removing finalizer", "namespace", nsName)
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}
	if !apierrors.IsNotFound(nsErr) {
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nsErr
	}

	// Stage 4: Namespace is confirmed NotFound; remove the finalizer
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest v1alpha1.PreviewEnvironment
		if err := r.Get(ctx, types.NamespacedName{Name: pe.Name}, &latest); err != nil {
			return client.IgnoreNotFound(err)
		}
		if controllerutil.ContainsFinalizer(&latest, v1alpha1.Finalizer) {
			controllerutil.RemoveFinalizer(&latest, v1alpha1.Finalizer)
			return r.Update(ctx, &latest)
		}
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("completed PreviewEnvironment teardown and removed finalizer", "namespace", nsName)
	return ctrl.Result{}, nil
}

func (r *PreviewEnvironmentReconciler) gatewayStillReferencesNamespace(ctx context.Context, nsName string) bool {
	if r.Config.SkipHTTPRoute {
		return false
	}
	gw := &unstructured.Unstructured{}
	gw.SetGroupVersionKind(gateway.GatewayGVK)
	if err := r.Get(ctx, types.NamespacedName{Name: r.Config.GatewayName, Namespace: r.Config.GatewayNamespace}, gw); err != nil {
		return false
	}
	ann := gw.GetAnnotations()
	if ann == nil {
		return false
	}
	bs := ann["networking.gke.io/backend-services"]
	return bs != "" && stringsContains(bs, "-"+nsName+"-")
}

func stringsContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func (r *PreviewEnvironmentReconciler) clearStuckSVCNEGFinalizers(ctx context.Context, nsName string) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gateway.HTTPRouteGVK.GroupVersion().WithKind("ServiceNetworkEndpointGroupList"))
	list.SetAPIVersion("networking.gke.io/v1beta1")
	list.SetKind("ServiceNetworkEndpointGroupList")
	if err := r.List(ctx, list, client.InNamespace(nsName)); err != nil {
		return
	}
	for i := range list.Items {
		item := &list.Items[i]
		if len(item.GetFinalizers()) > 0 {
			item.SetFinalizers(nil)
			_ = r.Update(ctx, item)
		}
	}
}

func (r *PreviewEnvironmentReconciler) commonLabels(norm *validation.NormalizedSpec) map[string]string {
	lbls := map[string]string{
		LabelManagedBy:       LabelManagedByValue,
		LabelEnvironmentName: norm.EnvironmentName,
	}
	if norm.PRNumber > 0 {
		lbls[LabelPRNumber] = strconv.Itoa(int(norm.PRNumber))
	}
	return lbls
}

func mergeLabels(existing, desired map[string]string) map[string]string {
	if existing == nil {
		existing = make(map[string]string, len(desired))
	}
	for k, v := range desired {
		existing[k] = v
	}
	return existing
}

func (r *PreviewEnvironmentReconciler) reconcileNamespace(ctx context.Context, pe *v1alpha1.PreviewEnvironment, norm *validation.NormalizedSpec) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: norm.Namespace}}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ns, func() error {
			ns.Labels = mergeLabels(ns.Labels, r.commonLabels(norm))
			ns.Labels["pod-security.kubernetes.io/enforce"] = "restricted"
			ns.Labels["pod-security.kubernetes.io/warn"] = "restricted"
			return controllerutil.SetControllerReference(pe, ns, r.Scheme)
		})
		return err
	})
}

func (r *PreviewEnvironmentReconciler) reconcileResourceQuota(ctx context.Context, norm *validation.NormalizedSpec) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		rq := &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ResourceQuotaName,
				Namespace: norm.Namespace,
			},
		}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, rq, func() error {
			rq.Labels = mergeLabels(rq.Labels, r.commonLabels(norm))
			rq.Spec.Hard = corev1.ResourceList{
				corev1.ResourceRequestsCPU:    resource.MustParse("600m"),
				corev1.ResourceLimitsCPU:      r.Config.Policy.MaxNamespaceCPU,
				corev1.ResourceRequestsMemory: resource.MustParse("512Mi"),
				corev1.ResourceLimitsMemory:   r.Config.Policy.MaxNamespaceMemory,
				corev1.ResourcePods:           resource.MustParse("6"),
				corev1.ResourceServices:       resource.MustParse("4"),
			}
			return nil
		})
		return err
	})
}

func (r *PreviewEnvironmentReconciler) reconcileLimitRange(ctx context.Context, norm *validation.NormalizedSpec) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		lr := &corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{
				Name:      LimitRangeName,
				Namespace: norm.Namespace,
			},
		}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, lr, func() error {
			lr.Labels = mergeLabels(lr.Labels, r.commonLabels(norm))
			lr.Spec.Limits = []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					Default: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					DefaultRequest: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
					Max: corev1.ResourceList{
						corev1.ResourceCPU:    r.Config.Policy.MaxPodCPU,
						corev1.ResourceMemory: r.Config.Policy.MaxPodMemory,
					},
					Min: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10m"),
						corev1.ResourceMemory: resource.MustParse("16Mi"),
					},
				},
			}
			return nil
		})
		return err
	})
}

func (r *PreviewEnvironmentReconciler) reconcileNetworkPolicy(ctx context.Context, norm *validation.NormalizedSpec) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		np := &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      NetworkPolicyName,
				Namespace: norm.Namespace,
			},
		}
		proxyCIDR := r.Config.ProxySubnetCIDR
		if proxyCIDR == "" {
			proxyCIDR = "10.42.16.0/23"
		}
		protoTCP := corev1.ProtocolTCP
		appPort := intstr.FromInt32(norm.Port)

		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, np, func() error {
			np.Labels = mergeLabels(np.Labels, r.commonLabels(norm))
			np.Spec = networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{},
				PolicyTypes: []networkingv1.PolicyType{
					networkingv1.PolicyTypeIngress,
				},
				Ingress: []networkingv1.NetworkPolicyIngressRule{
					{
						From: []networkingv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{},
							},
						},
						Ports: []networkingv1.NetworkPolicyPort{
							{
								Protocol: &protoTCP,
								Port:     &appPort,
							},
						},
					},
					{
						From: []networkingv1.NetworkPolicyPeer{
							{
								IPBlock: &networkingv1.IPBlock{CIDR: "35.191.0.0/16"},
							},
							{
								IPBlock: &networkingv1.IPBlock{CIDR: "130.211.0.0/22"},
							},
							{
								IPBlock: &networkingv1.IPBlock{CIDR: proxyCIDR},
							},
						},
						Ports: []networkingv1.NetworkPolicyPort{
							{
								Protocol: &protoTCP,
								Port:     &appPort,
							},
						},
					},
				},
			}
			return nil
		})
		return err
	})
}

func (r *PreviewEnvironmentReconciler) reconcileDeployment(ctx context.Context, norm *validation.NormalizedSpec) (*appsv1.Deployment, error) {
	var result *appsv1.Deployment
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      DeploymentName,
				Namespace: norm.Namespace,
			},
		}

		labels := r.commonLabels(norm)
		podLabels := map[string]string{
			"app.kubernetes.io/name":     "preview-app",
			"app.kubernetes.io/instance": norm.EnvironmentName,
			LabelManagedBy:               LabelManagedByValue,
			LabelEnvironmentName:         norm.EnvironmentName,
		}

		runAsNonRoot := true
		runAsUser := int64(65532)
		allowPrivEsc := false
		readOnlyRootFS := true
		automountToken := false

		envVars := []corev1.EnvVar{
			{Name: "PORT", Value: strconv.Itoa(int(norm.Port))},
			{Name: "PREVIEW_ENV_NAME", Value: norm.EnvironmentName},
			{Name: "PREVIEW_NAMESPACE", Value: norm.Namespace},
			{Name: "PREVIEW_DEVELOPER", Value: norm.Developer},
			{Name: "PREVIEW_PR_NUMBER", Value: strconv.Itoa(int(norm.PRNumber))},
			{Name: "PREVIEW_REPOSITORY", Value: norm.Repository},
			{Name: "PREVIEW_COMMIT_SHA", Value: norm.CommitSHA},
		}
		for _, custom := range norm.Env {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "APP_CUSTOM_" + custom.Name,
				Value: custom.Value,
			})
		}

		replicas := norm.Replicas
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
			dep.Labels = mergeLabels(dep.Labels, labels)
			dep.Spec.Replicas = &replicas
			dep.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":     "preview-app",
					"app.kubernetes.io/instance": norm.EnvironmentName,
				},
			}
			dep.Spec.Template.ObjectMeta.Labels = mergeLabels(dep.Spec.Template.ObjectMeta.Labels, podLabels)
			dep.Spec.Template.Spec.AutomountServiceAccountToken = &automountToken
			dep.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
				RunAsNonRoot: &runAsNonRoot,
				RunAsUser:    &runAsUser,
				RunAsGroup:   &runAsUser,
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			}
			dep.Spec.Template.Spec.Containers = []corev1.Container{
				{
					Name:            "app",
					Image:           norm.Image,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							ContainerPort: norm.Port,
							Protocol:      corev1.ProtocolTCP,
						},
					},
					Env: envVars,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    norm.CPURequest,
							corev1.ResourceMemory: norm.MemoryRequest,
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    norm.CPULimit,
							corev1.ResourceMemory: norm.MemoryLimit,
						},
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/readyz",
								Port: intstr.FromInt32(norm.Port),
							},
						},
						InitialDelaySeconds: 2,
						PeriodSeconds:       3,
						TimeoutSeconds:      2,
						FailureThreshold:    3,
					},
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/healthz",
								Port: intstr.FromInt32(norm.Port),
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       10,
						TimeoutSeconds:      2,
						FailureThreshold:    3,
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: &allowPrivEsc,
						ReadOnlyRootFilesystem:   &readOnlyRootFS,
						RunAsNonRoot:             &runAsNonRoot,
						RunAsUser:                &runAsUser,
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
					},
				},
			}
			return nil
		})
		result = dep
		return err
	})
	return result, err
}

func (r *PreviewEnvironmentReconciler) reconcileService(ctx context.Context, norm *validation.NormalizedSpec) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ServiceName,
				Namespace: norm.Namespace,
			},
		}

		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
			svc.Labels = mergeLabels(svc.Labels, r.commonLabels(norm))
			if svc.Annotations == nil {
				svc.Annotations = make(map[string]string)
			}
			if _, exists := svc.Annotations["cloud.google.com/neg"]; !exists {
				svc.Annotations["cloud.google.com/neg"] = `{"ingress":true}`
			}
			svc.Spec.Type = corev1.ServiceTypeClusterIP
			svc.Spec.Selector = map[string]string{
				"app.kubernetes.io/name":     "preview-app",
				"app.kubernetes.io/instance": norm.EnvironmentName,
			}
			svc.Spec.Ports = []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt32(norm.Port),
					Protocol:   corev1.ProtocolTCP,
				},
			}
			return nil
		})
		return err
	})
}

func (r *PreviewEnvironmentReconciler) reconcileHTTPRoute(ctx context.Context, norm *validation.NormalizedSpec) (*unstructured.Unstructured, error) {
	params := gateway.RouteParams{
		EnvironmentName:  norm.EnvironmentName,
		Namespace:        norm.Namespace,
		RouteName:        HTTPRouteName,
		GatewayName:      r.Config.GatewayName,
		GatewayNamespace: r.Config.GatewayNamespace,
		ServiceName:      ServiceName,
		ServicePort:      80,
		PathPrefix:       norm.PathPrefix,
		Labels:           r.commonLabels(norm),
	}

	desiredSpec := gateway.BuildHTTPRouteSpec(params)
	var result *unstructured.Unstructured

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing := &unstructured.Unstructured{}
		existing.SetGroupVersionKind(gateway.HTTPRouteGVK)

		err := r.Get(ctx, types.NamespacedName{Name: HTTPRouteName, Namespace: norm.Namespace}, existing)
		if apierrors.IsNotFound(err) {
			hr := gateway.BuildHTTPRoute(params)
			if err := r.Create(ctx, hr); err != nil {
				return err
			}
			result = hr
			return nil
		}
		if err != nil {
			return err
		}

		currentSpec := existing.Object["spec"]
		if !reflect.DeepEqual(currentSpec, desiredSpec) || !reflect.DeepEqual(existing.GetLabels(), params.Labels) {
			existing.SetLabels(mergeLabels(existing.GetLabels(), params.Labels))
			existing.Object["spec"] = desiredSpec
			if err := r.Update(ctx, existing); err != nil {
				return err
			}
		}
		result = existing
		return nil
	})

	return result, err
}

func (r *PreviewEnvironmentReconciler) lookupGatewayIP(ctx context.Context) string {
	gw := &unstructured.Unstructured{}
	gw.SetGroupVersionKind(gateway.GatewayGVK)
	if err := r.Get(ctx, types.NamespacedName{Name: r.Config.GatewayName, Namespace: r.Config.GatewayNamespace}, gw); err != nil {
		return ""
	}
	ip, _ := gateway.ExtractGatewayAddress(gw)
	return ip
}

func (r *PreviewEnvironmentReconciler) updateStatusFailed(ctx context.Context, pe *v1alpha1.PreviewEnvironment, reason, message string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest v1alpha1.PreviewEnvironment
		if err := r.Get(ctx, types.NamespacedName{Name: pe.Name}, &latest); err != nil {
			return err
		}
		before := latest.Status.DeepCopy()

		latest.Status.Phase = v1alpha1.PhaseFailed
		latest.Status.ObservedGeneration = latest.Generation
		latest.Status.Message = message
		meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
			Type:               v1alpha1.ConditionValidated,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: latest.Generation,
			Reason:             reason,
			Message:            message,
		})
		meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
			Type:               v1alpha1.ConditionReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: latest.Generation,
			Reason:             reason,
			Message:            message,
		})

		if apiequality.Semantic.DeepEqual(before, &latest.Status) {
			return nil
		}
		return r.Status().Update(ctx, &latest)
	})
}

func (r *PreviewEnvironmentReconciler) updateStatusExpired(ctx context.Context, pe *v1alpha1.PreviewEnvironment, createdAt, expiresAt *metav1.Time) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest v1alpha1.PreviewEnvironment
		if err := r.Get(ctx, types.NamespacedName{Name: pe.Name}, &latest); err != nil {
			return err
		}
		latest.Status.Phase = v1alpha1.PhaseExpired
		latest.Status.CreatedAt = createdAt
		latest.Status.ExpiresAt = expiresAt
		latest.Status.CleanupStatus = "TTL expired; deleting namespace and PreviewEnvironment"
		latest.Status.Message = "Environment TTL expired; automatic cleanup triggered"
		meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
			Type:               v1alpha1.ConditionReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: latest.Generation,
			Reason:             "TTLExpired",
			Message:            "Environment TTL expired",
		})
		return r.Status().Update(ctx, &latest)
	})
}

func (r *PreviewEnvironmentReconciler) updateStatusProgress(
	ctx context.Context,
	pe *v1alpha1.PreviewEnvironment,
	norm *validation.NormalizedSpec,
	createdAt, expiresAt *metav1.Time,
	dep *appsv1.Deployment,
	workloadReady, routeAccepted bool,
	routeReason, routeMsg, previewURL string,
	allReady bool,
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest v1alpha1.PreviewEnvironment
		if err := r.Get(ctx, types.NamespacedName{Name: pe.Name}, &latest); err != nil {
			return err
		}
		before := latest.Status.DeepCopy()

		latest.Status.ObservedGeneration = latest.Generation
		latest.Status.Namespace = norm.Namespace
		latest.Status.DeploymentName = DeploymentName
		latest.Status.ServiceName = ServiceName
		latest.Status.HTTPRouteName = HTTPRouteName
		latest.Status.CreatedAt = createdAt
		latest.Status.ExpiresAt = expiresAt
		latest.Status.CleanupStatus = "Active"
		if dep != nil {
			latest.Status.ReadyReplicas = dep.Status.ReadyReplicas
		}

		meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
			Type:               v1alpha1.ConditionValidated,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: latest.Generation,
			Reason:             "SpecValid",
			Message:            "Spec passed platform security and resource validation",
		})
		meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
			Type:               v1alpha1.ConditionNamespaceReady,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: latest.Generation,
			Reason:             "NamespaceProvisioned",
			Message:            fmt.Sprintf("Namespace %s, ResourceQuota, LimitRange, and NetworkPolicy reconciled", norm.Namespace),
		})

		if workloadReady {
			meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
				Type:               v1alpha1.ConditionWorkloadReady,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: latest.Generation,
				Reason:             "DeploymentAvailable",
				Message:            fmt.Sprintf("Deployment has %d/%d ready replicas", latest.Status.ReadyReplicas, norm.Replicas),
			})
		} else {
			meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
				Type:               v1alpha1.ConditionWorkloadReady,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: latest.Generation,
				Reason:             "WaitingForReplicas",
				Message:            fmt.Sprintf("Deployment has %d/%d ready replicas", latest.Status.ReadyReplicas, norm.Replicas),
			})
		}

		if routeAccepted && previewURL != "" {
			meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
				Type:               v1alpha1.ConditionRouteAccepted,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: latest.Generation,
				Reason:             routeReason,
				Message:            routeMsg,
			})
		} else {
			meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
				Type:               v1alpha1.ConditionRouteAccepted,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: latest.Generation,
				Reason:             routeReason,
				Message:            routeMsg,
			})
		}

		if allReady {
			latest.Status.Phase = v1alpha1.PhaseReady
			latest.Status.PreviewURL = previewURL
			latest.Status.Message = fmt.Sprintf("Environment ready at %s", previewURL)
			if latest.Status.ReadyAt == nil || latest.Status.ReadyAt.IsZero() {
				now := metav1.Now()
				latest.Status.ReadyAt = &now
				exp := metav1.NewTime(now.Add(time.Duration(norm.TTLSeconds) * time.Second))
				latest.Status.ExpiresAt = &exp
			}
			meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
				Type:               v1alpha1.ConditionReady,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: latest.Generation,
				Reason:             "EnvironmentReady",
				Message:            fmt.Sprintf("Preview environment healthy and routed at %s", previewURL),
			})
		} else {
			latest.Status.Phase = v1alpha1.PhaseProvisioning
			if previewURL != "" {
				latest.Status.PreviewURL = previewURL
			}
			latest.Status.Message = "Waiting for workload readiness and Gateway route acceptance"
			meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
				Type:               v1alpha1.ConditionReady,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: latest.Generation,
				Reason:             "Provisioning",
				Message:            latest.Status.Message,
			})
		}

		if apiequality.Semantic.DeepEqual(before, &latest.Status) {
			return nil
		}
		return r.Status().Update(ctx, &latest)
	})
}

func (r *PreviewEnvironmentReconciler) refreshPhaseMetrics(ctx context.Context) {
	var list v1alpha1.PreviewEnvironmentList
	if err := r.List(ctx, &list); err != nil {
		return
	}
	counts := map[v1alpha1.PreviewPhase]float64{
		v1alpha1.PhasePending:      0,
		v1alpha1.PhaseProvisioning: 0,
		v1alpha1.PhaseReady:        0,
		v1alpha1.PhaseFailed:       0,
		v1alpha1.PhaseDeleting:     0,
		v1alpha1.PhaseExpired:      0,
	}
	for _, item := range list.Items {
		phase := item.Status.Phase
		if phase == "" {
			phase = v1alpha1.PhasePending
		}
		counts[phase]++
	}
	for phase, val := range counts {
		metrics.EnvironmentsByPhase.WithLabelValues(string(phase)).Set(val)
	}
}

func mapChildToPreviewEnvironment(_ context.Context, obj client.Object) []reconcile.Request {
	labels := obj.GetLabels()
	if labels == nil {
		return nil
	}
	if labels[LabelManagedBy] != LabelManagedByValue {
		return nil
	}
	envName := labels[LabelEnvironmentName]
	if envName == "" {
		return nil
	}
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: envName}},
	}
}

func (r *PreviewEnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PreviewEnvironment{}).
		Owns(&corev1.Namespace{}).
		Watches(&appsv1.Deployment{}, handler.EnqueueRequestsFromMapFunc(mapChildToPreviewEnvironment)).
		Watches(&corev1.Service{}, handler.EnqueueRequestsFromMapFunc(mapChildToPreviewEnvironment)).
		Watches(&networkingv1.NetworkPolicy{}, handler.EnqueueRequestsFromMapFunc(mapChildToPreviewEnvironment)).
		Complete(r)
}
