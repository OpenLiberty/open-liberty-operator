package utils

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/appsody/appsody-operator/pkg/common"
	prometheusv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"

	appsodyv1beta1 "github.com/appsody/appsody-operator/pkg/apis/appsody/v1beta1"
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// CustomizeDeployment ...
func CustomizeDeployment(deploy *appsv1.Deployment, ba common.BaseApplication) {
	obj := ba.(metav1.Object)
	deploy.Labels = ba.GetLabels()
	deploy.Annotations = MergeMaps(deploy.Annotations, ba.GetAnnotations())

	deploy.Spec.Replicas = ba.GetReplicas()
	deploy.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app.kubernetes.io/name": obj.GetName(),
		},
	}

	UpdateAppDefinition(deploy.Labels, deploy.Annotations, ba)
}

// CustomizeStatefulSet ...
func CustomizeStatefulSet(statefulSet *appsv1.StatefulSet, ba common.BaseApplication) {
	obj := ba.(metav1.Object)
	statefulSet.Labels = ba.GetLabels()
	statefulSet.Annotations = MergeMaps(statefulSet.Annotations, ba.GetAnnotations())

	statefulSet.Spec.Replicas = ba.GetReplicas()
	statefulSet.Spec.ServiceName = obj.GetName() + "-headless"
	statefulSet.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app.kubernetes.io/name": obj.GetName(),
		},
	}

	UpdateAppDefinition(statefulSet.Labels, statefulSet.Annotations, ba)
}

// UpdateAppDefinition ...
func UpdateAppDefinition(labels map[string]string, annotations map[string]string, ba common.BaseApplication) {
	obj := ba.(metav1.Object)
	if ba.GetCreateAppDefinition() != nil && !*ba.GetCreateAppDefinition() {
		delete(labels, "kappnav.app.auto-create")
		delete(annotations, "kappnav.app.auto-create.name")
		delete(annotations, "kappnav.app.auto-create.kinds")
		delete(annotations, "kappnav.app.auto-create.label")
		delete(annotations, "kappnav.app.auto-create.labels-values")
		delete(annotations, "kappnav.app.auto-create.version")
	} else {
		labels["kappnav.app.auto-create"] = "true"
		annotations["kappnav.app.auto-create.name"] = obj.GetName()
		annotations["kappnav.app.auto-create.kinds"] = "Deployment, StatefulSet, Service, Route, Ingress, ConfigMap"
		annotations["kappnav.app.auto-create.label"] = "app.kubernetes.io/name"
		annotations["kappnav.app.auto-create.labels-values"] = obj.GetName()
		if ba.GetVersion() == "" {
			delete(annotations, "kappnav.app.auto-create.version")
		} else {
			annotations["kappnav.app.auto-create.version"] = ba.GetVersion()
		}
	}
}

// CustomizeRoute ...
func CustomizeRoute(route *routev1.Route, ba common.BaseApplication) {
	obj := ba.(metav1.Object)
	route.Labels = ba.GetLabels()
	route.Annotations = MergeMaps(route.Annotations, ba.GetAnnotations())
	route.Spec.To.Kind = "Service"
	route.Spec.To.Name = obj.GetName()
	weight := int32(100)
	route.Spec.To.Weight = &weight
	if route.Spec.Port == nil {
		route.Spec.Port = &routev1.RoutePort{}
	}
	route.Spec.Port.TargetPort = intstr.FromString(strconv.Itoa(int(ba.GetService().GetPort())) + "-tcp")
}

// ErrorIsNoMatchesForKind ...
func ErrorIsNoMatchesForKind(err error, kind string, version string) bool {
	return strings.HasPrefix(err.Error(), fmt.Sprintf("no matches for kind \"%s\" in version \"%s\"", kind, version))
}

// CustomizeService ...
func CustomizeService(svc *corev1.Service, ba common.BaseApplication) {
	obj := ba.(metav1.Object)
	svc.Labels = ba.GetLabels()
	svc.Annotations = MergeMaps(svc.Annotations, ba.GetAnnotations())

	if len(svc.Spec.Ports) == 0 {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{})
	}

	svc.Spec.Ports[0].Port = ba.GetService().GetPort()
	svc.Spec.Ports[0].TargetPort = intstr.FromInt(int(ba.GetService().GetPort()))
	svc.Spec.Ports[0].Name = strconv.Itoa(int(ba.GetService().GetPort())) + "-tcp"
	svc.Spec.Type = *ba.GetService().GetType()
	svc.Spec.Selector = map[string]string{
		"app.kubernetes.io/name": obj.GetName(),
	}
}

// CustomizePodSpec ...
func CustomizePodSpec(pts *corev1.PodTemplateSpec, ba common.BaseApplication) {
	obj := ba.(metav1.Object)
	pts.Labels = ba.GetLabels()
	pts.Annotations = MergeMaps(pts.Annotations, ba.GetAnnotations())

	if len(pts.Spec.Containers) == 0 {
		pts.Spec.Containers = append(pts.Spec.Containers, corev1.Container{})
	}
	pts.Spec.Containers[0].Name = "app"
	if len(pts.Spec.Containers[0].Ports) == 0 {
		pts.Spec.Containers[0].Ports = append(pts.Spec.Containers[0].Ports, corev1.ContainerPort{})
	}

	pts.Spec.Containers[0].Ports[0].ContainerPort = ba.GetService().GetPort()
	pts.Spec.Containers[0].Image = ba.GetApplicationImage()
	pts.Spec.Containers[0].Ports[0].Name = strconv.Itoa(int(ba.GetService().GetPort())) + "-tcp"
	if ba.GetResourceConstraints() != nil {
		pts.Spec.Containers[0].Resources = *ba.GetResourceConstraints()
	}
	pts.Spec.Containers[0].ReadinessProbe = ba.GetReadinessProbe()
	pts.Spec.Containers[0].LivenessProbe = ba.GetLivenessProbe()
	pts.Spec.Containers[0].VolumeMounts = ba.GetVolumeMounts()
	if ba.GetPullPolicy() != nil {
		pts.Spec.Containers[0].ImagePullPolicy = *ba.GetPullPolicy()
	}
	pts.Spec.Containers[0].Env = ba.GetEnv()
	pts.Spec.Containers[0].EnvFrom = ba.GetEnvFrom()
	pts.Spec.Volumes = ba.GetVolumes()
	if ba.GetServiceAccountName() != nil && *ba.GetServiceAccountName() != "" {
		pts.Spec.ServiceAccountName = *ba.GetServiceAccountName()
	} else {
		pts.Spec.ServiceAccountName = obj.GetName()
	}
	pts.Spec.RestartPolicy = corev1.RestartPolicyAlways
	pts.Spec.DNSPolicy = corev1.DNSClusterFirst

	if len(ba.GetArchitecture()) > 0 {
		pts.Spec.Affinity = &corev1.Affinity{}
		CustomizeAffinity(pts.Spec.Affinity, ba)
	}

}

// CustomizePersistence ...
func CustomizePersistence(statefulSet *appsv1.StatefulSet, ba common.BaseApplication) {
	obj := ba.(metav1.Object)
	if len(statefulSet.Spec.VolumeClaimTemplates) == 0 {
		var pvc *corev1.PersistentVolumeClaim
		if ba.GetStorage().GetVolumeClaimTemplate() != nil {
			pvc = ba.GetStorage().GetVolumeClaimTemplate()
		} else {
			pvc = &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc",
					Namespace: obj.GetNamespace(),
					Labels:    ba.GetLabels(),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(ba.GetStorage().GetSize()),
						},
					},
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
				},
			}
			pvc.Annotations = MergeMaps(pvc.Annotations, ba.GetAnnotations())
		}
		statefulSet.Spec.VolumeClaimTemplates = append(statefulSet.Spec.VolumeClaimTemplates, *pvc)
	}

	if ba.GetStorage().GetMountPath() != "" {
		found := false
		for _, v := range statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts {
			if v.Name == statefulSet.Spec.VolumeClaimTemplates[0].Name {
				found = true
			}
		}

		if !found {
			vm := corev1.VolumeMount{
				Name:      statefulSet.Spec.VolumeClaimTemplates[0].Name,
				MountPath: ba.GetStorage().GetMountPath(),
			}
			statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts = append(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts, vm)
		}
	}

}

// CustomizeServiceAccount ...
func CustomizeServiceAccount(sa *corev1.ServiceAccount, ba common.BaseApplication) {
	sa.Labels = ba.GetLabels()
	sa.Annotations = MergeMaps(sa.Annotations, ba.GetAnnotations())

	if ba.GetPullSecret() != nil {
		if len(sa.ImagePullSecrets) == 0 {
			sa.ImagePullSecrets = append(sa.ImagePullSecrets, corev1.LocalObjectReference{
				Name: *ba.GetPullSecret(),
			})
		} else {
			sa.ImagePullSecrets[0].Name = *ba.GetPullSecret()
		}
	}
}

// CustomizeAffinity ...
func CustomizeAffinity(a *corev1.Affinity, ba common.BaseApplication) {
	a.NodeAffinity = &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Operator: corev1.NodeSelectorOpIn,
							Values:   ba.GetArchitecture(),
							Key:      "beta.kubernetes.io/arch",
						},
					},
				},
			},
		},
	}

	archs := len(ba.GetArchitecture())
	for i := range ba.GetArchitecture() {
		arch := ba.GetArchitecture()[i]
		term := corev1.PreferredSchedulingTerm{
			Weight: int32(archs - i),
			Preference: corev1.NodeSelectorTerm{
				MatchExpressions: []corev1.NodeSelectorRequirement{
					{
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{arch},
						Key:      "beta.kubernetes.io/arch",
					},
				},
			},
		}
		a.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = append(a.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution, term)
	}
}

// CustomizeKnativeService ...
func CustomizeKnativeService(ksvc *servingv1alpha1.Service, ba common.BaseApplication) {
	obj := ba.(metav1.Object)
	ksvc.Labels = ba.GetLabels()
	MergeMaps(ksvc.Annotations, ba.GetAnnotations())

	// If `expose` is not set to `true`, make Knative route a private route by adding `serving.knative.dev/visibility: cluster-local`
	// to the Knative service. If `serving.knative.dev/visibility: XYZ` is defined in cr.Labels, `expose` always wins.
	if ba.GetExpose() != nil && *ba.GetExpose() {
		delete(ksvc.Labels, "serving.knative.dev/visibility")
	} else {
		ksvc.Labels["serving.knative.dev/visibility"] = "cluster-local"
	}

	if ksvc.Spec.Template == nil {
		ksvc.Spec.Template = &servingv1alpha1.RevisionTemplateSpec{}
	}
	if len(ksvc.Spec.Template.Spec.Containers) == 0 {
		ksvc.Spec.Template.Spec.Containers = append(ksvc.Spec.Template.Spec.Containers, corev1.Container{})
	}

	if len(ksvc.Spec.Template.Spec.Containers[0].Ports) == 0 {
		ksvc.Spec.Template.Spec.Containers[0].Ports = append(ksvc.Spec.Template.Spec.Containers[0].Ports, corev1.ContainerPort{})
	}
	ksvc.Spec.Template.ObjectMeta.Labels = ba.GetLabels()
	ksvc.Spec.Template.ObjectMeta.Annotations = MergeMaps(ksvc.Spec.Template.ObjectMeta.Annotations, ba.GetAnnotations())

	ksvc.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort = ba.GetService().GetPort()
	ksvc.Spec.Template.Spec.Containers[0].Image = ba.GetApplicationImage()
	// Knative sets its own resource constraints
	//ksvc.Spec.Template.Spec.Containers[0].Resources = *cr.Spec.ResourceConstraints
	ksvc.Spec.Template.Spec.Containers[0].ReadinessProbe = ba.GetReadinessProbe()
	ksvc.Spec.Template.Spec.Containers[0].LivenessProbe = ba.GetLivenessProbe()
	ksvc.Spec.Template.Spec.Containers[0].VolumeMounts = ba.GetVolumeMounts()
	ksvc.Spec.Template.Spec.Containers[0].ImagePullPolicy = *ba.GetPullPolicy()
	ksvc.Spec.Template.Spec.Containers[0].Env = ba.GetEnv()
	ksvc.Spec.Template.Spec.Containers[0].EnvFrom = ba.GetEnvFrom()

	ksvc.Spec.Template.Spec.Volumes = ba.GetVolumes()

	if ba.GetServiceAccountName() != nil && *ba.GetServiceAccountName() != "" {
		ksvc.Spec.Template.Spec.ServiceAccountName = *ba.GetServiceAccountName()
	} else {
		ksvc.Spec.Template.Spec.ServiceAccountName = obj.GetName()
	}

	if ksvc.Spec.Template.Spec.Containers[0].LivenessProbe != nil {
		if ksvc.Spec.Template.Spec.Containers[0].LivenessProbe.HTTPGet != nil {
			ksvc.Spec.Template.Spec.Containers[0].LivenessProbe.HTTPGet.Port = intstr.IntOrString{}
		}
		if ksvc.Spec.Template.Spec.Containers[0].LivenessProbe.TCPSocket != nil {
			ksvc.Spec.Template.Spec.Containers[0].LivenessProbe.TCPSocket.Port = intstr.IntOrString{}
		}
	}

	if ksvc.Spec.Template.Spec.Containers[0].ReadinessProbe != nil {
		if ksvc.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet != nil {
			ksvc.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet.Port = intstr.IntOrString{}
		}
		if ksvc.Spec.Template.Spec.Containers[0].ReadinessProbe.TCPSocket != nil {
			ksvc.Spec.Template.Spec.Containers[0].ReadinessProbe.TCPSocket.Port = intstr.IntOrString{}
		}
	}
}

// CustomizeHPA ...
func CustomizeHPA(hpa *autoscalingv1.HorizontalPodAutoscaler, ba common.BaseApplication) {
	obj := ba.(metav1.Object)
	hpa.Labels = ba.GetLabels()
	hpa.Annotations = MergeMaps(hpa.Annotations, ba.GetAnnotations())

	hpa.Spec.MaxReplicas = ba.GetAutoscaling().GetMaxReplicas()
	hpa.Spec.MinReplicas = ba.GetAutoscaling().GetMinReplicas()
	hpa.Spec.TargetCPUUtilizationPercentage = ba.GetAutoscaling().GetTargetCPUUtilizationPercentage()

	hpa.Spec.ScaleTargetRef.Name = obj.GetName()
	hpa.Spec.ScaleTargetRef.APIVersion = "apps/v1"

	if ba.GetStorage() != nil {
		hpa.Spec.ScaleTargetRef.Kind = "StatefulSet"
	} else {
		hpa.Spec.ScaleTargetRef.Kind = "Deployment"
	}
}

// Validate if the BaseApplication is valid
func Validate(ba common.BaseApplication) (bool, error) {
	// Storage validation
	if ba.GetStorage() != nil {
		if ba.GetStorage().GetVolumeClaimTemplate() == nil {
			if ba.GetStorage().GetSize() == "" {
				return false, fmt.Errorf("validation failed: " + requiredFieldMessage("spec.storage.size"))
			}
			if _, err := resource.ParseQuantity(ba.GetStorage().GetSize()); err != nil {
				return false, fmt.Errorf("validation failed: cannot parse '%v': %v", ba.GetStorage().GetSize(), err)
			}
		}
	}

	return true, nil
}

func createValidationError(msg string) error {
	return fmt.Errorf("validation failed: " + msg)
}

func requiredFieldMessage(fieldPaths ...string) string {
	return "must set the field(s): " + strings.Join(fieldPaths, ",")
}

// CustomizeServiceMonitor ...
func CustomizeServiceMonitor(sm *prometheusv1.ServiceMonitor, ba common.BaseApplication) {
	obj := ba.(metav1.Object)
	sm.Labels = ba.GetLabels()
	sm.Annotations = MergeMaps(sm.Annotations, ba.GetAnnotations())

	sm.Spec.Selector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app.kubernetes.io/name":  obj.GetName(),
			"app.appsody.dev/monitor": "true",
		},
	}
	if len(sm.Spec.Endpoints) == 0 {
		sm.Spec.Endpoints = append(sm.Spec.Endpoints, prometheusv1.Endpoint{})
	}
	sm.Spec.Endpoints[0].Port = strconv.Itoa(int(ba.GetService().GetPort())) + "-tcp"
	if len(ba.GetMonitoring().GetLabels()) > 0 {
		for k, v := range ba.GetMonitoring().GetLabels() {
			sm.Labels[k] = v
		}
	}

	if len(ba.GetMonitoring().GetEndpoints()) > 0 {
		endpoints := ba.GetMonitoring().GetEndpoints()
		if endpoints[0].Scheme != "" {
			sm.Spec.Endpoints[0].Scheme = endpoints[0].Scheme
		}
		if endpoints[0].Interval != "" {
			sm.Spec.Endpoints[0].Interval = endpoints[0].Interval
		}
		if endpoints[0].Path != "" {
			sm.Spec.Endpoints[0].Path = endpoints[0].Path
		}

		if endpoints[0].TLSConfig != nil {
			sm.Spec.Endpoints[0].TLSConfig = endpoints[0].TLSConfig
		}

		if endpoints[0].BasicAuth != nil {
			sm.Spec.Endpoints[0].BasicAuth = endpoints[0].BasicAuth
		}

		if endpoints[0].Params != nil {
			sm.Spec.Endpoints[0].Params = endpoints[0].Params
		}
		if endpoints[0].ScrapeTimeout != "" {
			sm.Spec.Endpoints[0].ScrapeTimeout = endpoints[0].ScrapeTimeout
		}
		if endpoints[0].BearerTokenFile != "" {
			sm.Spec.Endpoints[0].BearerTokenFile = endpoints[0].BearerTokenFile
		}
	}

}

// GetCondition ...
func GetCondition(conditionType appsodyv1beta1.StatusConditionType, status *appsodyv1beta1.AppsodyApplicationStatus) *appsodyv1beta1.StatusCondition {
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return &status.Conditions[i]
		}
	}

	return nil
}

// SetCondition ...
func SetCondition(condition appsodyv1beta1.StatusCondition, status *appsodyv1beta1.AppsodyApplicationStatus) {
	for i := range status.Conditions {
		if status.Conditions[i].Type == condition.Type {
			status.Conditions[i] = condition
			return
		}
	}

	status.Conditions = append(status.Conditions, condition)
}

// GetWatchNamespaces returns a slice of namespaces the operator should watch based on WATCH_NAMESPSCE value
// WATCH_NAMESPSCE value could be empty for watching the whole cluster or a comma-separated list of namespaces
func GetWatchNamespaces() ([]string, error) {
	watchNamespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		return nil, err
	}

	var watchNamespaces []string
	for _, ns := range strings.Split(watchNamespace, ",") {
		watchNamespaces = append(watchNamespaces, strings.TrimSpace(ns))
	}

	return watchNamespaces, nil
}

// MergeMaps returns a map containing the union of al the key-value pairs from the input maps. The order of the maps passed into the
// func, defines the importance. e.g. if (keyA, value1) is in map1, and (keyA, value2) is in map2, mergeMaps(map1, map2) would contain (keyA, value2).
func MergeMaps(maps ...map[string]string) map[string]string {
	dest := make(map[string]string)

	for i := range maps {
		for key, value := range maps[i] {
			dest[key] = value
		}
	}

	return dest
}
