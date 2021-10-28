package v1beta1

import (
	"time"

	"github.com/application-stacks/runtime-component-operator/common"
	prometheusv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Defines the desired state of OpenLibertyApplication.
// +k8s:openapi-gen=true
type OpenLibertyApplicationSpec struct {
	// The name of the application this resource is part of. If not specified, it defaults to the name of the CR.
	// +operator-sdk:csv:customresourcedefinitions:order=1,type=spec,displayName="Application Name",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	ApplicationName string `json:"applicationName,omitempty"`

	// Application image to be installed.
	// +operator-sdk:csv:customresourcedefinitions:order=2,type=spec,displayName="Application Image",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	ApplicationImage string `json:"applicationImage"`

	// +operator-sdk:csv:customresourcedefinitions:order=3,type=spec,displayName="Application Version",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	Version string `json:"version,omitempty"`

	// Policy for pulling container images. Defaults to IfNotPresent. Parameters spec.autoscaling.maxReplicas and spec.resourceConstraints.requests.cpu must be specified.
	// +operator-sdk:csv:customresourcedefinitions:order=4,type=spec,displayName="Pull Policy",xDescriptors="urn:alm:descriptor:com.tectonic.ui:imagePullPolicy"
	PullPolicy *corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// Number of pods to create.
	// +operator-sdk:csv:customresourcedefinitions:order=5,type=spec,displayName="Replicas",xDescriptors="urn:alm:descriptor:com.tectonic.ui:podCount"
	Replicas *int32 `json:"replicas,omitempty"`

	// A boolean that toggles the external exposure of this deployment via a Route or a Knative Route resource.
	// +operator-sdk:csv:customresourcedefinitions:order=6,type=spec,displayName="Expose",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	Expose *bool `json:"expose,omitempty"`

	// Limits the amount of required resources.
	// +operator-sdk:csv:customresourcedefinitions:order=7,type=spec,displayName="Resource Requirements",xDescriptors="urn:alm:descriptor:com.tectonic.ui:resourceRequirements"
	ResourceConstraints *corev1.ResourceRequirements `json:"resourceConstraints,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=8,type=spec,displayName="Service"
	Service *OpenLibertyApplicationService `json:"service,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=16,type=spec,displayName="Auto Scaling"
	Autoscaling *OpenLibertyApplicationAutoScaling `json:"autoscaling,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=20,type=spec,displayName="Deployment"
	Deployment *OpenLibertyApplicationDeployment `json:"deployment,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=22,type=spec,displayName="StatefulSet"
	StatefulSet *OpenLibertyApplicationStatefulSet `json:"statefulSet,omitempty"`

	// The name of the OpenShift service account to be used during deployment.
	// +operator-sdk:csv:customresourcedefinitions:order=28,type=spec,displayName="Service Account Name",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	ServiceAccountName *string `json:"serviceAccountName,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=31,type=spec,displayName="Create App Definition",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	CreateAppDefinition *bool `json:"createAppDefinition,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=33,type=spec,displayName="Monitoring"
	Monitoring *OpenLibertyApplicationMonitoring `json:"monitoring,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=36,type=spec,displayName="Affinity"
	Affinity *OpenLibertyApplicationAffinity `json:"affinity,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=41,type=spec,displayName="Route"
	Route *OpenLibertyApplicationRoute `json:"route,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=48,type=spec,displayName="Bindings"
	Bindings *OpenLibertyApplicationBindings `json:"bindings,omitempty"`

	// A boolean to toggle the creation of Knative resources and usage of Knative serving.
	// +operator-sdk:csv:customresourcedefinitions:order=52,type=spec,displayName="Create Knative Service",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	CreateKnativeService *bool `json:"createKnativeService,omitempty"`

	// Detects if the services need to be restarted.
	// +operator-sdk:csv:customresourcedefinitions:order=53,type=spec,displayName="Liveness Probe"
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`

	// Detects if the services are ready to serve.
	// +operator-sdk:csv:customresourcedefinitions:order=54,type=spec,displayName="Readiness Probe"
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`

	// Protects slow starting containers from livenessProbe.
	// +operator-sdk:csv:customresourcedefinitions:order=55,type=spec,displayName="StartupProbe Probe"
	StartupProbe *corev1.Probe `json:"startupProbe,omitempty"`

	// Name of the Secret to use to pull images from the specified repository. It is not required if the cluster is configured with a global image pull secret.
	// +operator-sdk:csv:customresourcedefinitions:order=56,type=spec,displayName="Pull Secret",xDescriptors="urn:alm:descriptor:io.kubernetes:Secret"
	PullSecret *string `json:"pullSecret,omitempty"`

	// Represents a pod volume with data that is accessible to the containers.
	// +listType=map
	// +listMapKey=name
	// +operator-sdk:csv:customresourcedefinitions:order=57,type=spec,displayName="Volume"
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Represents where to mount the volumes into containers.
	// +listType=atomic
	// +operator-sdk:csv:customresourcedefinitions:order=58,type=spec,displayName="Volume Mounts"
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// An array of environment variables following the format of {name, value}, where value is a simple string.
	// +listType=map
	// +listMapKey=name
	// +operator-sdk:csv:customresourcedefinitions:order=59,type=spec,displayName="Env Var"
	Env []corev1.EnvVar `json:"env,omitempty"`

	// An array of references to ConfigMap or Secret resources containing environment variables.
	// +listType=atomic
	// +operator-sdk:csv:customresourcedefinitions:order=60,type=spec,displayName="Env From"
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// An array of architectures to be considered for deployment. Their position in the array indicates preference.
	// +listType=set
	// +operator-sdk:csv:customresourcedefinitions:order=61,type=spec,displayName="Architecture"
	Architecture []string `json:"architecture,omitempty"`

	// List of containers that run before other containers in a pod.
	// +listType=map
	// +listMapKey=name
	// +operator-sdk:csv:customresourcedefinitions:order=62,type=spec,displayName="Init Containers"
	InitContainers []corev1.Container `json:"initContainers,omitempty"`

	// The list of sidecar containers. These are additional containers to be added to the pods.
	// +listType=map
	// +listMapKey=name
	// +operator-sdk:csv:customresourcedefinitions:order=63,type=spec,displayName="Sidecar Containers"
	SidecarContainers []corev1.Container `json:"sidecarContainers,omitempty"`

	// Open Liberty specific capabilities

	// +operator-sdk:csv:customresourcedefinitions:order=29,type=spec,displayName="Serviceability"
	Serviceability *OpenLibertyApplicationServiceability `json:"serviceability,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=30,type=spec,displayName="Single sign-on"
	SSO *OpenLibertyApplicationSSO `json:"sso,omitempty"`
}

// Configures a Pod to run on particular Nodes.
// +k8s:openapi-gen=true
type OpenLibertyApplicationAffinity struct {
	// Controls which nodes the pod are scheduled to run on, based on labels on the node.
	// +operator-sdk:csv:customresourcedefinitions:order=37,type=spec,displayName="Node Affinity",xDescriptors="urn:alm:descriptor:com.tectonic.ui:nodeAffinity"
	NodeAffinity *corev1.NodeAffinity `json:"nodeAffinity,omitempty"`

	// Controls the nodes the pod are scheduled to run on, based on labels on the pods that are already running on the node.
	// +operator-sdk:csv:customresourcedefinitions:order=38,type=spec,displayName="Pod Affinity",xDescriptors="urn:alm:descriptor:com.tectonic.ui:podAffinity"
	PodAffinity *corev1.PodAffinity `json:"podAffinity,omitempty"`

	// Enables the ability to prevent running a pod on the same node as another pod.
	// +operator-sdk:csv:customresourcedefinitions:order=39,type=spec,displayName="Pod Anti Affinity",xDescriptors="urn:alm:descriptor:com.tectonic.ui:podAntiAffinity"
	PodAntiAffinity *corev1.PodAntiAffinity `json:"podAntiAffinity,omitempty"`

	// A YAML object that contains a set of required labels and their values.
	// +operator-sdk:csv:customresourcedefinitions:order=40,type=spec,displayName="Node Affinity Labels",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	NodeAffinityLabels map[string]string `json:"nodeAffinityLabels,omitempty"`

	// An array of architectures to be considered for deployment. Their position in the array indicates preference.
	// +listType=set
	Architecture []string `json:"architecture,omitempty"`
}

// Configures the desired resource consumption of pods.
// +k8s:openapi-gen=true
type OpenLibertyApplicationAutoScaling struct {
	// Required field for autoscaling. Upper limit for the number of pods that can be set by the autoscaler.
	// +kubebuilder:validation:Minimum=1
	// +operator-sdk:csv:customresourcedefinitions:order=17,type=spec,displayName="Max Replicas",xDescriptors="urn:alm:descriptor:com.tectonic.ui:number"
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// Lower limit for the number of pods that can be set by the autoscaler.
	// +operator-sdk:csv:customresourcedefinitions:order=18,type=spec,displayName="Min Replicas",xDescriptors="urn:alm:descriptor:com.tectonic.ui:number"
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// Target average CPU utilization (represented as a percentage of requested CPU) over all the pods.
	// +operator-sdk:csv:customresourcedefinitions:order=19,type=spec,displayName="Target CPU Utilization Percentage",xDescriptors="urn:alm:descriptor:com.tectonic.ui:number"
	TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`
}

// Configures parameters for the network service of pods.
// +k8s:openapi-gen=true
type OpenLibertyApplicationService struct {
	// The port exposed by the container.
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:validation:Minimum=1
	// +operator-sdk:csv:customresourcedefinitions:order=9,type=spec,displayName="Service Port",xDescriptors="urn:alm:descriptor:com.tectonic.ui:number"
	Port int32 `json:"port,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=10,type=spec,displayName="Service Type",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	Type *corev1.ServiceType `json:"type,omitempty"`

	// Node proxies this port into your service.
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:validation:Minimum=0
	// +operator-sdk:csv:customresourcedefinitions:order=11,type=spec,displayName="Node Port",xDescriptors="urn:alm:descriptor:com.tectonic.ui:number"
	NodePort *int32 `json:"nodePort,omitempty"`

	// The name for the port exposed by the container.
	// +operator-sdk:csv:customresourcedefinitions:order=12,type=spec,displayName="Port Name",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	PortName string `json:"portName,omitempty"`

	// Annotations to be added to the service.
	// +operator-sdk:csv:customresourcedefinitions:order=13,type=spec,displayName="Service Annotations",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	Annotations map[string]string `json:"annotations,omitempty"`

	// The port that the operator assigns to containers inside pods. Defaults to the value of spec.service.port.
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:validation:Minimum=1
	// +operator-sdk:csv:customresourcedefinitions:order=14,type=spec,displayName="Target Port",xDescriptors="urn:alm:descriptor:com.tectonic.ui:number"
	TargetPort *int32 `json:"targetPort,omitempty"`

	// 	A name of a secret that already contains TLS key, certificate and CA to be mounted in the pod.
	// +k8s:openapi-gen=true
	// +operator-sdk:csv:customresourcedefinitions:order=15,type=spec,displayName="Certificate Secret Reference",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	CertificateSecretRef *string `json:"certificateSecretRef,omitempty"`

	// An array consisting of service ports.
	Ports []corev1.ServicePort `json:"ports,omitempty"`

	// +listType=atomic
	Consumes []ServiceBindingConsumes `json:"consumes,omitempty"`
	Provides *ServiceBindingProvides  `json:"provides,omitempty"`
}

// Defines the desired state and cycle of applications.
type OpenLibertyApplicationDeployment struct {
	// Specifies the strategy to replace old deployment pods with new pods.
	// +operator-sdk:csv:customresourcedefinitions:order=21,type=spec,displayName="Deployment Update Strategy",xDescriptors="urn:alm:descriptor:com.tectonic.ui:updateStrategy"
	UpdateStrategy *appsv1.DeploymentStrategy `json:"updateStrategy,omitempty"`

	// Annotations to be added only to the Deployment and resources owned by the Deployment
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Defines the desired state and cycle of stateful applications.
type OpenLibertyApplicationStatefulSet struct {
	// Specifies the strategy to replace old StatefulSet pods with new pods.
	// +operator-sdk:csv:customresourcedefinitions:order=23,type=spec,displayName="StatefulSet Update Strategy",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	UpdateStrategy *appsv1.StatefulSetUpdateStrategy `json:"updateStrategy,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=24,type=spec,displayName="Storage"
	Storage *OpenLibertyApplicationStorage `json:"storage,omitempty"`

	// Annotations to be added only to the StatefulSet and resources owned by the StatefulSet
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Configures the OpenAPI information to expose.
type ServiceBindingProvides struct {
	// Service binding type to be provided by this CR. At this time, the only allowed value is openapi.
	Category common.ServiceBindingCategory `json:"category"`

	// Specifies context root of the service.
	Context string `json:"context,omitempty"`

	// Protocol of the provided service. Defauts to http.
	Protocol string `json:"protocol,omitempty"`

	Auth *ServiceBindingAuth `json:"auth,omitempty"`
}

// Represents a service to be consumed.
// +k8s:openapi-gen=true
type ServiceBindingConsumes struct {
	// The name of the service to be consumed. If binding to an OpenLibertyApplication, then this would be the provider’s CR name.
	Name string `json:"name"`

	// The namespace of the service to be consumed. If binding to an OpenLibertyApplication, then this would be the provider’s CR namespace.
	Namespace string `json:"namespace,omitempty"`

	// The type of service binding to be consumed. At this time, the only allowed value is openapi.
	Category common.ServiceBindingCategory `json:"category"`

	// Optional field to specify which location in the pod, service binding secret should be mounted.
	MountPath string `json:"mountPath,omitempty"`
}

// Defines settings of persisted storage for StatefulSets.
// +k8s:openapi-gen=true
type OpenLibertyApplicationStorage struct {
	// A convenient field to set the size of the persisted storage.
	// +kubebuilder:validation:Pattern=^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$
	// +operator-sdk:csv:customresourcedefinitions:order=25,type=spec,displayName="Storage Size",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	Size string `json:"size,omitempty"`

	// The directory inside the container where this persisted storage will be bound to.
	// +operator-sdk:csv:customresourcedefinitions:order=26,type=spec,displayName="Storage Mount Path",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	MountPath string `json:"mountPath,omitempty"`

	// A YAML object that represents a volumeClaimTemplate component of a StatefulSet.
	// +operator-sdk:csv:customresourcedefinitions:order=27,type=spec,displayName="Storage Volume Claim Template",xDescriptors="urn:alm:descriptor:com.tectonic.ui:PersistentVolumeClaim"
	VolumeClaimTemplate *corev1.PersistentVolumeClaim `json:"volumeClaimTemplate,omitempty"`
}

// Specifies parameters for Service Monitor.
// +k8s:openapi-gen=true
type OpenLibertyApplicationMonitoring struct {
	// Labels to set on ServiceMonitor.
	// +operator-sdk:csv:customresourcedefinitions:order=34,type=spec,displayName="Monitoring Labels",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	Labels map[string]string `json:"labels,omitempty"`

	// A YAML snippet representing an array of Endpoint component from ServiceMonitor.
	// +listType=atomic
	// +operator-sdk:csv:customresourcedefinitions:order=35,type=spec,displayName="Monitoring Endpoints",xDescriptors="urn:alm:descriptor:com.tectonic.ui:endpointList"
	Endpoints []prometheusv1.Endpoint `json:"endpoints,omitempty"`
}

// Specifies serviceability-related operations, such as gathering server memory dumps and server traces.
// +k8s:openapi-gen=true
type OpenLibertyApplicationServiceability struct {
	// A convenient field to request the size of the persisted storage to use for serviceability.
	// +kubebuilder:validation:Pattern=^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$
	Size string `json:"size,omitempty"`

	// The name of the PersistentVolumeClaim resource you created to be used for serviceability.
	// +kubebuilder:validation:Pattern=.+
	VolumeClaimName string `json:"volumeClaimName,omitempty"`

	// A convenient field to request the StorageClassName of the persisted storage to use for serviceability.
	// +kubebuilder:validation:Pattern=.+
	StorageClassName string `json:"storageClassName,omitempty"`
}

// Configures the ingress resource.
// +k8s:openapi-gen=true
type OpenLibertyApplicationRoute struct {

	// Annotations to be added to the Route.
	// +operator-sdk:csv:customresourcedefinitions:order=42,type=spec,displayName="Route Annotations",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	Annotations map[string]string `json:"annotations,omitempty"`

	// Hostname to be used for the Route.
	// +operator-sdk:csv:customresourcedefinitions:order=43,type=spec,displayName="Route Host",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	Host string `json:"host,omitempty"`

	// Path to be used for Route.
	// +operator-sdk:csv:customresourcedefinitions:order=44,type=spec,displayName="Route Path",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	Path string `json:"path,omitempty"`

	// A name of a secret that already contains TLS key, certificate and CA to be used in the route. Also can contain destination CA certificate.
	// +operator-sdk:csv:customresourcedefinitions:order=45,type=spec,displayName="Certificate Secret Reference",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	CertificateSecretRef *string `json:"certificateSecretRef,omitempty"`

	// TLS termination policy. Can be one of edge, reencrypt and passthrough.
	// +operator-sdk:csv:customresourcedefinitions:order=46,type=spec,displayName="Termination",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	Termination *routev1.TLSTerminationType `json:"termination,omitempty"`

	// HTTP traffic policy with TLS enabled. Can be one of Allow, Redirect and None.
	// +operator-sdk:csv:customresourcedefinitions:order=47,type=spec,displayName="Insecure Edge Termination Policy",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	InsecureEdgeTerminationPolicy *routev1.InsecureEdgeTerminationPolicyType `json:"insecureEdgeTerminationPolicy,omitempty"`
}

// Allows a service to provide authentication information.
type ServiceBindingAuth struct {
	// The secret that contains the username for authenticating.
	Username corev1.SecretKeySelector `json:"username,omitempty"`
	// The secret that contains the password for authenticating.
	Password corev1.SecretKeySelector `json:"password,omitempty"`
}

// Represents service binding related parameters.
type OpenLibertyApplicationBindings struct {

	// A boolean to toggle whether the operator should automatically detect and use a ServiceBindingRequest resource with <CR_NAME>-binding naming format.
	// +operator-sdk:csv:customresourcedefinitions:order=49,type=spec,displayName="Bindings Autodetect",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	AutoDetect *bool `json:"autoDetect,omitempty"`

	// The name of a ServiceBindingRequest custom resource created manually in the same namespace as the application.
	// +operator-sdk:csv:customresourcedefinitions:order=50,type=spec,displayName="Bindings Resource Ref",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	ResourceRef string `json:"resourceRef,omitempty"`

	// A boolean to toggle whether the operator expose the application as a bindable service.
	// +operator-sdk:csv:customresourcedefinitions:order=51,type=spec,displayName="Bindings Expose Enabled",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	Expose *OpenLibertyApplicationBindingExpose `json:"expose,omitempty"`

	// A YAML object that represents a ServiceBindingRequest custom resource.
	Embedded *runtime.RawExtension `json:"embedded,omitempty"`
}

// Encapsulates information exposed by the application.
type OpenLibertyApplicationBindingExpose struct {
	// A boolean to toggle whether the operator expose the application as a bindable service. The default value for this parameter is false.
	Enabled *bool `json:"enabled,omitempty"`
}

// Defines the observed state of OpenLibertyApplication.
// +k8s:openapi-gen=true
type OpenLibertyApplicationStatus struct {
	// +listType=atomic
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Status Conditions",xDescriptors="urn:alm:descriptor:io.kubernetes.conditions"
	Conditions       []StatusCondition       `json:"conditions,omitempty"`
	ConsumedServices common.ConsumedServices `json:"consumedServices,omitempty"`
	RouteAvailable   *bool                   `json:"routeAvailable,omitempty"`
	// +listType=set
	ResolvedBindings []string `json:"resolvedBindings,omitempty"`
	ImageReference   string   `json:"imageReference,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Service Binding Secret",xDescriptors="urn:alm:descriptor:io.kubernetes:Secret"
	Binding *corev1.LocalObjectReference `json:"binding,omitempty"`
}

// Defines possible status conditions.
// +k8s:openapi-gen=true
type StatusCondition struct {
	LastTransitionTime *metav1.Time           `json:"lastTransitionTime,omitempty"`
	LastUpdateTime     metav1.Time            `json:"lastUpdateTime,omitempty"`
	Reason             string                 `json:"reason,omitempty"`
	Message            string                 `json:"message,omitempty"`
	Status             corev1.ConditionStatus `json:"status,omitempty"`
	Type               StatusConditionType    `json:"type,omitempty"`
}

// Defines the type of status condition.
type StatusConditionType string

const (
	// StatusConditionTypeReconciled ...
	StatusConditionTypeReconciled StatusConditionType = "Reconciled"

	// StatusConditionTypeDependenciesSatisfied ...
	StatusConditionTypeDependenciesSatisfied StatusConditionType = "DependenciesSatisfied"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OpenLibertyApplication is the Schema for the OpenLibertyApplications API
// +k8s:openapi-gen=true
// +kubebuilder:resource:path=openlibertyapplications,scope=Namespaced,shortName=olapp;olapps
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Image",type="string",JSONPath=".spec.applicationImage",priority=0,description="Absolute name of the deployed image containing registry and tag"
// +kubebuilder:printcolumn:name="Exposed",type="boolean",JSONPath=".spec.expose",priority=0,description="Specifies whether deployment is exposed externally via default Route"
// +kubebuilder:printcolumn:name="Reconciled",type="string",JSONPath=".status.conditions[?(@.type=='Reconciled')].status",priority=0,description="Status of the reconcile condition"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type=='Reconciled')].reason",priority=1,description="Reason for the failure of reconcile condition"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type=='Reconciled')].message",priority=1,description="Failure message from reconcile condition"
// +kubebuilder:printcolumn:name="DependenciesSatisfied",type="string",JSONPath=".status.conditions[?(@.type=='DependenciesSatisfied')].status",priority=1,description="Status of the application dependencies"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",priority=0,description="Age of the resource"
type OpenLibertyApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenLibertyApplicationSpec   `json:"spec,omitempty"`
	Status OpenLibertyApplicationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OpenLibertyApplicationList contains a list of OpenLibertyApplication
type OpenLibertyApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenLibertyApplication `json:"items"`
}

// Specifies the configuration for Single sign-on (SSO) providers to authenticate with.
// +k8s:openapi-gen=true
type OpenLibertyApplicationSSO struct {
	// +listType=atomic
	OIDC []OidcClient `json:"oidc,omitempty"`

	// +listType=atomic
	Oauth2 []OAuth2Client `json:"oauth2,omitempty"`

	Github *GithubLogin `json:"github,omitempty"`

	// Common parameters for all SSO providers

	// Specifies a callback protocol, host and port number.
	RedirectToRPHostAndPort string `json:"redirectToRPHostAndPort,omitempty"`

	// Specifies whether to map a user identifier to a registry user. This parameter applies to all providers.
	MapToUserRegistry *bool `json:"mapToUserRegistry,omitempty"`
}

// Represents configuration for an OpenID Connect (OIDC) client.
// +k8s:openapi-gen=true
type OidcClient struct {
	// The unique ID for the provider. Default value is oidc.
	ID string `json:"id,omitempty"`

	// Specifies a discovery endpoint URL for the OpenID Connect provider. Required field.
	DiscoveryEndpoint string `json:"discoveryEndpoint"`

	// Specifies the name of the claim. Use its value as the user group membership.
	GroupNameAttribute string `json:"groupNameAttribute,omitempty"`

	// Specifies the name of the claim. Use its value as the authenticated user principal.
	UserNameAttribute string `json:"userNameAttribute,omitempty"`

	// The name of the social login configuration for display.
	DisplayName string `json:"displayName,omitempty"`

	// Specifies whether the UserInfo endpoint is contacted.
	UserInfoEndpointEnabled *bool `json:"userInfoEndpointEnabled,omitempty"`

	// Specifies the name of the claim. Use its value as the subject realm.
	RealmNameAttribute string `json:"realmNameAttribute,omitempty"`

	// Specifies one or more scopes to request.
	Scope string `json:"scope,omitempty"`

	// Specifies the required authentication method.
	TokenEndpointAuthMethod string `json:"tokenEndpointAuthMethod,omitempty"`

	// Specifies whether to enable host name verification when the client contacts the provider.
	HostNameVerificationEnabled *bool `json:"hostNameVerificationEnabled,omitempty"`
}

// Represents configuration for an OAuth2 client.
// +k8s:openapi-gen=true
type OAuth2Client struct {
	// Specifies the unique ID for the provider. The default value is oauth2.
	ID string `json:"id,omitempty"`

	// Specifies a token endpoint URL for the OAuth 2.0 provider. Required field.
	TokenEndpoint string `json:"tokenEndpoint"`

	// Specifies an authorization endpoint URL for the OAuth 2.0 provider. Required field.
	AuthorizationEndpoint string `json:"authorizationEndpoint"`

	// Specifies the name of the claim. Use its value as the user group membership
	GroupNameAttribute string `json:"groupNameAttribute,omitempty"`

	// Specifies the name of the claim. Use its value as the authenticated user principal.
	UserNameAttribute string `json:"userNameAttribute,omitempty"`

	// The name of the social login configuration for display.
	DisplayName string `json:"displayName,omitempty"`

	// Specifies the name of the claim. Use its value as the subject realm.
	RealmNameAttribute string `json:"realmNameAttribute,omitempty"`

	// Specifies the realm name for this social media.
	RealmName string `json:"realmName,omitempty"`

	// Specifies one or more scopes to request.
	Scope string `json:"scope,omitempty"`

	// Specifies the required authentication method.
	TokenEndpointAuthMethod string `json:"tokenEndpointAuthMethod,omitempty"`

	// Name of the header to use when an OAuth access token is forwarded.
	AccessTokenHeaderName string `json:"accessTokenHeaderName,omitempty"`

	// Determines whether the access token that is provided in the request is used for authentication.
	AccessTokenRequired *bool `json:"accessTokenRequired,omitempty"`

	// Determines whether to support access token authentication if an access token is provided in the request.
	AccessTokenSupported *bool `json:"accessTokenSupported,omitempty"`

	// Indicates which specification to use for the user API.
	UserApiType string `json:"userApiType,omitempty"`

	// The URL for retrieving the user information.
	UserApi string `json:"userApi,omitempty"`
}

// Represents configuration for social login using GitHub.
// +k8s:openapi-gen=true
type GithubLogin struct {
	// Specifies the host name of your enterprise GitHub.
	Hostname string `json:"hostname,omitempty"`
}

func init() {
	SchemeBuilder.Register(&OpenLibertyApplication{}, &OpenLibertyApplicationList{})
}

// GetApplicationImage returns application image
func (cr *OpenLibertyApplication) GetApplicationImage() string {
	return cr.Spec.ApplicationImage
}

// GetPullPolicy returns image pull policy
func (cr *OpenLibertyApplication) GetPullPolicy() *corev1.PullPolicy {
	return cr.Spec.PullPolicy
}

// GetPullSecret returns secret name for docker registry credentials
func (cr *OpenLibertyApplication) GetPullSecret() *string {
	return cr.Spec.PullSecret
}

// GetServiceAccountName returns service account name
func (cr *OpenLibertyApplication) GetServiceAccountName() *string {
	return cr.Spec.ServiceAccountName
}

// GetReplicas returns number of replicas
func (cr *OpenLibertyApplication) GetReplicas() *int32 {
	return cr.Spec.Replicas
}

// GetLivenessProbe returns liveness probe
func (cr *OpenLibertyApplication) GetLivenessProbe() *corev1.Probe {
	return cr.Spec.LivenessProbe
}

// GetReadinessProbe returns readiness probe
func (cr *OpenLibertyApplication) GetReadinessProbe() *corev1.Probe {
	return cr.Spec.ReadinessProbe
}

// GetStartupProbe returns startup probe
func (cr *OpenLibertyApplication) GetStartupProbe() *corev1.Probe {
	return cr.Spec.StartupProbe
}

// GetVolumes returns volumes slice
func (cr *OpenLibertyApplication) GetVolumes() []corev1.Volume {
	return cr.Spec.Volumes
}

// GetVolumeMounts returns volume mounts slice
func (cr *OpenLibertyApplication) GetVolumeMounts() []corev1.VolumeMount {
	return cr.Spec.VolumeMounts
}

// GetResourceConstraints returns resource constraints
func (cr *OpenLibertyApplication) GetResourceConstraints() *corev1.ResourceRequirements {
	return cr.Spec.ResourceConstraints
}

// GetExpose returns expose flag
func (cr *OpenLibertyApplication) GetExpose() *bool {
	return cr.Spec.Expose
}

// GetEnv returns slice of environment variables
func (cr *OpenLibertyApplication) GetEnv() []corev1.EnvVar {
	return cr.Spec.Env
}

// GetEnvFrom returns slice of environment variables from source
func (cr *OpenLibertyApplication) GetEnvFrom() []corev1.EnvFromSource {
	return cr.Spec.EnvFrom
}

// GetCreateKnativeService returns flag that toggles Knative service
func (cr *OpenLibertyApplication) GetCreateKnativeService() *bool {
	return cr.Spec.CreateKnativeService
}

// GetArchitecture returns slice of architectures
func (cr *OpenLibertyApplication) GetArchitecture() []string {
	return cr.Spec.Architecture
}

// GetAutoscaling returns autoscaling settings
func (cr *OpenLibertyApplication) GetAutoscaling() common.BaseComponentAutoscaling {
	if cr.Spec.Autoscaling == nil {
		return nil
	}
	return cr.Spec.Autoscaling
}

// GetStorage returns storage settings
func (cr *OpenLibertyApplication) GetStorage() common.BaseComponentStorage {
	if cr.Spec.Storage == nil {
		return nil
	}
	return cr.Spec.Storage
}

// GetService returns service settings
func (cr *OpenLibertyApplication) GetService() common.BaseComponentService {
	if cr.Spec.Service == nil {
		return nil
	}
	return cr.Spec.Service
}

// GetVersion returns application version
func (cr *OpenLibertyApplication) GetVersion() string {
	return cr.Spec.Version
}

// GetCreateAppDefinition returns a toggle for integration with kAppNav
func (cr *OpenLibertyApplication) GetCreateAppDefinition() *bool {
	return cr.Spec.CreateAppDefinition
}

// GetApplicationName returns Application name to be used for integration with kAppNav
func (cr *OpenLibertyApplication) GetApplicationName() string {
	return cr.Spec.ApplicationName
}

// GetMonitoring returns monitoring settings
func (cr *OpenLibertyApplication) GetMonitoring() common.BaseComponentMonitoring {
	if cr.Spec.Monitoring == nil {
		return nil
	}
	return cr.Spec.Monitoring
}

// GetStatus returns OpenLibertyApplication status
func (cr *OpenLibertyApplication) GetStatus() common.BaseComponentStatus {
	return &cr.Status
}

// GetInitContainers returns list of init containers
func (cr *OpenLibertyApplication) GetInitContainers() []corev1.Container {
	return cr.Spec.InitContainers
}

// GetSidecarContainers returns list of sidecar containers
func (cr *OpenLibertyApplication) GetSidecarContainers() []corev1.Container {
	return cr.Spec.SidecarContainers
}

// GetGroupName returns group name to be used in labels and annotation
func (cr *OpenLibertyApplication) GetGroupName() string {
	return "openliberty.io"
}

// GetRoute returns route
func (cr *OpenLibertyApplication) GetRoute() common.BaseComponentRoute {
	if cr.Spec.Route == nil {
		return nil
	}
	return cr.Spec.Route
}

// GetBindings returns binding configuration for OpenLibertyApplication
func (cr *OpenLibertyApplication) GetBindings() common.BaseComponentBindings {
	if cr.Spec.Bindings == nil {
		return nil
	}
	return cr.Spec.Bindings
}

// GetAffinity returns deployment's node and pod affinity settings
func (cr *OpenLibertyApplication) GetAffinity() common.BaseComponentAffinity {
	if cr.Spec.Affinity == nil {
		return nil
	}
	return cr.Spec.Affinity
}

// GetDeployment returns deployment settings
func (cr *OpenLibertyApplication) GetDeployment() common.BaseComponentDeployment {
	if cr.Spec.Deployment == nil {
		return nil
	}
	return cr.Spec.Deployment
}

// GetDeploymentStrategy returns deployment strategy struct
func (cr *OpenLibertyApplicationDeployment) GetDeploymentUpdateStrategy() *appsv1.DeploymentStrategy {
	return cr.UpdateStrategy
}

// GetAnnotations returns annotations to be added only to the Deployment and its child resources
func (rcd *OpenLibertyApplicationDeployment) GetAnnotations() map[string]string {
	return rcd.Annotations
}

// GetStatefulSet returns statefulSet settings
func (cr *OpenLibertyApplication) GetStatefulSet() common.BaseComponentStatefulSet {
	if cr.Spec.StatefulSet == nil {
		return nil
	}
	return cr.Spec.StatefulSet
}

// GetStatefulSetUpdateStrategy returns statefulSet strategy struct
func (cr *OpenLibertyApplicationStatefulSet) GetStatefulSetUpdateStrategy() *appsv1.StatefulSetUpdateStrategy {
	return cr.UpdateStrategy
}

// GetStorage returns storage settings
func (ss *OpenLibertyApplicationStatefulSet) GetStorage() common.BaseComponentStorage {
	if ss.Storage == nil {
		return nil
	}
	return ss.Storage
}

// GetAnnotations returns annotations to be added only to the StatefulSet and its child resources
func (rcss *OpenLibertyApplicationStatefulSet) GetAnnotations() map[string]string {
	return rcss.Annotations
}

// GetResolvedBindings returns a map of all the service names to be consumed by the application
func (s *OpenLibertyApplicationStatus) GetResolvedBindings() []string {
	return s.ResolvedBindings
}

// SetResolvedBindings sets ConsumedServices
func (s *OpenLibertyApplicationStatus) SetResolvedBindings(rb []string) {
	s.ResolvedBindings = rb
}

// GetConsumedServices returns a map of all the service names to be consumed by the application
func (s *OpenLibertyApplicationStatus) GetConsumedServices() common.ConsumedServices {
	if s.ConsumedServices == nil {
		return nil
	}
	return s.ConsumedServices
}

// SetConsumedServices sets ConsumedServices
func (s *OpenLibertyApplicationStatus) SetConsumedServices(c common.ConsumedServices) {
	s.ConsumedServices = c
}

// GetImageReference returns Docker image reference to be deployed by the CR
func (s *OpenLibertyApplicationStatus) GetImageReference() string {
	return s.ImageReference
}

// SetImageReference sets Docker image reference on the status portion of the CR
func (s *OpenLibertyApplicationStatus) SetImageReference(imageReference string) {
	s.ImageReference = imageReference
}

// GetBinding returns BindingStatus representing binding status
func (s *OpenLibertyApplicationStatus) GetBinding() *corev1.LocalObjectReference {
	return s.Binding
}

// SetBinding sets BindingStatus representing binding status
func (s *OpenLibertyApplicationStatus) SetBinding(r *corev1.LocalObjectReference) {
	s.Binding = r
}

// GetMinReplicas returns minimum replicas
func (a *OpenLibertyApplicationAutoScaling) GetMinReplicas() *int32 {
	return a.MinReplicas
}

// GetMaxReplicas returns maximum replicas
func (a *OpenLibertyApplicationAutoScaling) GetMaxReplicas() int32 {
	return a.MaxReplicas
}

// GetTargetCPUUtilizationPercentage returns target cpu usage
func (a *OpenLibertyApplicationAutoScaling) GetTargetCPUUtilizationPercentage() *int32 {
	return a.TargetCPUUtilizationPercentage
}

// GetSize returns pesistent volume size
func (s *OpenLibertyApplicationStorage) GetSize() string {
	return s.Size
}

// GetMountPath returns mount path for persistent volume
func (s *OpenLibertyApplicationStorage) GetMountPath() string {
	return s.MountPath
}

// GetVolumeClaimTemplate returns a template representing requested persistent volume
func (s *OpenLibertyApplicationStorage) GetVolumeClaimTemplate() *corev1.PersistentVolumeClaim {
	return s.VolumeClaimTemplate
}

// GetAnnotations returns a set of annotations to be added to the service
func (s *OpenLibertyApplicationService) GetAnnotations() map[string]string {
	return s.Annotations
}

// GetServiceability returns serviceability
func (cr *OpenLibertyApplication) GetServiceability() *OpenLibertyApplicationServiceability {
	return cr.Spec.Serviceability
}

// GetSize returns pesistent volume size for Serviceability
func (s *OpenLibertyApplicationServiceability) GetSize() string {
	return s.Size
}

// GetVolumeClaimName returns the name of custom PersistentVolumeClaim (PVC) for Serviceability. Must be in the same namespace as the OpenLibertyApplication.
func (s *OpenLibertyApplicationServiceability) GetVolumeClaimName() string {
	return s.VolumeClaimName
}

// GetPort returns service port
func (s *OpenLibertyApplicationService) GetPort() int32 {
	if s != nil && s.Port != 0 {
		return s.Port
	}
	return 9080
}

// GetNodePort returns service nodePort
func (s *OpenLibertyApplicationService) GetNodePort() *int32 {
	if s.NodePort == nil {
		return nil
	}
	return s.NodePort
}

// GetTargetPort returns the internal target port for containers
func (s *OpenLibertyApplicationService) GetTargetPort() *int32 {
	return s.TargetPort
}

// GetPortName returns name of service port
func (s *OpenLibertyApplicationService) GetPortName() string {
	return s.PortName
}

// GetType returns service type
func (s *OpenLibertyApplicationService) GetType() *corev1.ServiceType {
	return s.Type
}

// GetPorts returns a list of service ports
func (s *OpenLibertyApplicationService) GetPorts() []corev1.ServicePort {
	return s.Ports
}

// GetProvides returns service provider configuration
func (s *OpenLibertyApplicationService) GetProvides() common.ServiceBindingProvides {
	if s.Provides == nil {
		return nil
	}
	return s.Provides
}

// GetCertificateSecretRef returns a secret reference with a certificate
func (s *OpenLibertyApplicationService) GetCertificateSecretRef() *string {
	return s.CertificateSecretRef
}

// GetCategory returns category of a service provider configuration
func (p *ServiceBindingProvides) GetCategory() common.ServiceBindingCategory {
	return p.Category
}

// GetContext returns context of a service provider configuration
func (p *ServiceBindingProvides) GetContext() string {
	return p.Context
}

// GetAuth returns secret of a service provider configuration
func (p *ServiceBindingProvides) GetAuth() common.ServiceBindingAuth {
	if p.Auth == nil {
		return nil
	}
	return p.Auth
}

// GetProtocol returns protocol of a service provider configuration
func (p *ServiceBindingProvides) GetProtocol() string {
	return p.Protocol
}

// GetConsumes returns a list of service consumers' configuration
func (s *OpenLibertyApplicationService) GetConsumes() []common.ServiceBindingConsumes {
	consumes := make([]common.ServiceBindingConsumes, len(s.Consumes))
	for i := range s.Consumes {
		consumes[i] = &s.Consumes[i]
	}
	return consumes
}

// GetName returns service name of a service consumer configuration
func (c *ServiceBindingConsumes) GetName() string {
	return c.Name
}

// GetNamespace returns namespace of a service consumer configuration
func (c *ServiceBindingConsumes) GetNamespace() string {
	return c.Namespace
}

// GetCategory returns category of a service consumer configuration
func (c *ServiceBindingConsumes) GetCategory() common.ServiceBindingCategory {
	return common.ServiceBindingCategoryOpenAPI
}

// GetMountPath returns mount path of a service consumer configuration
func (c *ServiceBindingConsumes) GetMountPath() string {
	return c.MountPath
}

// GetUsername returns username of a service binding auth object
func (a *ServiceBindingAuth) GetUsername() corev1.SecretKeySelector {
	return a.Username
}

// GetPassword returns password of a service binding auth object
func (a *ServiceBindingAuth) GetPassword() corev1.SecretKeySelector {
	return a.Password
}

// GetLabels returns labels to be added on ServiceMonitor
func (m *OpenLibertyApplicationMonitoring) GetLabels() map[string]string {
	return m.Labels
}

// GetEndpoints returns endpoints to be added to ServiceMonitor
func (m *OpenLibertyApplicationMonitoring) GetEndpoints() []prometheusv1.Endpoint {
	return m.Endpoints
}

// GetAnnotations returns route annotations
func (r *OpenLibertyApplicationRoute) GetAnnotations() map[string]string {
	return r.Annotations
}

// GetCertificateSecretRef returns a secret reference with a certificate
func (r *OpenLibertyApplicationRoute) GetCertificateSecretRef() *string {
	return r.CertificateSecretRef
}

// GetTermination returns terminatation of the route's TLS
func (r *OpenLibertyApplicationRoute) GetTermination() *routev1.TLSTerminationType {
	return r.Termination
}

// GetInsecureEdgeTerminationPolicy returns terminatation of the route's TLS
func (r *OpenLibertyApplicationRoute) GetInsecureEdgeTerminationPolicy() *routev1.InsecureEdgeTerminationPolicyType {
	return r.InsecureEdgeTerminationPolicy
}

// GetHost returns hostname to be used by the route
func (r *OpenLibertyApplicationRoute) GetHost() string {
	return r.Host
}

// GetPath returns path to use for the route
func (r *OpenLibertyApplicationRoute) GetPath() string {
	return r.Path
}

// GetAutoDetect returns a boolean to specify if the operator should auto-detect ServiceBinding CRs with the same name as the OpenLibertyApplication CR
func (r *OpenLibertyApplicationBindings) GetAutoDetect() *bool {
	return r.AutoDetect
}

// GetResourceRef returns name of ServiceBinding CRs created manually in the same namespace as the OpenLibertyApplication CR
func (r *OpenLibertyApplicationBindings) GetResourceRef() string {
	return r.ResourceRef
}

// GetEmbedded returns the embedded underlying Service Binding resource
func (r *OpenLibertyApplicationBindings) GetEmbedded() *runtime.RawExtension {
	return r.Embedded
}

// GetExpose returns the map used making this application a bindable service
func (r *OpenLibertyApplicationBindings) GetExpose() common.BaseComponentExpose {
	if r.Expose == nil {
		return nil
	}
	return r.Expose
}

// GetEnabled returns whether the application should be exposable as a service
func (e *OpenLibertyApplicationBindingExpose) GetEnabled() *bool {
	return e.Enabled
}

// GetNodeAffinity returns node affinity
func (a *OpenLibertyApplicationAffinity) GetNodeAffinity() *corev1.NodeAffinity {
	return a.NodeAffinity
}

// GetPodAffinity returns pod affinity
func (a *OpenLibertyApplicationAffinity) GetPodAffinity() *corev1.PodAffinity {
	return a.PodAffinity
}

// GetPodAntiAffinity returns pod anti-affinity
func (a *OpenLibertyApplicationAffinity) GetPodAntiAffinity() *corev1.PodAntiAffinity {
	return a.PodAntiAffinity
}

// GetArchitecture returns list of architecture names
func (a *OpenLibertyApplicationAffinity) GetArchitecture() []string {
	return a.Architecture
}

// GetNodeAffinityLabels returns list of architecture names
func (a *OpenLibertyApplicationAffinity) GetNodeAffinityLabels() map[string]string {
	return a.NodeAffinityLabels
}

// Initialize sets default values
func (cr *OpenLibertyApplication) Initialize() {
	if cr.Spec.PullPolicy == nil {
		pp := corev1.PullIfNotPresent
		cr.Spec.PullPolicy = &pp
	}

	if cr.Spec.ResourceConstraints == nil {
		cr.Spec.ResourceConstraints = &corev1.ResourceRequirements{}
	}

	// Default applicationName to cr.Name, if a user sets createAppDefinition to true but doesn't set applicationName
	if cr.Spec.ApplicationName == "" {
		if cr.Labels != nil && cr.Labels["app.kubernetes.io/part-of"] != "" {
			cr.Spec.ApplicationName = cr.Labels["app.kubernetes.io/part-of"]
		} else {
			cr.Spec.ApplicationName = cr.Name
		}
	}

	if cr.Labels != nil {
		cr.Labels["app.kubernetes.io/part-of"] = cr.Spec.ApplicationName
	}

	// This is to handle when there is no service in the CR
	if cr.Spec.Service == nil {
		cr.Spec.Service = &OpenLibertyApplicationService{}
	}

	if cr.Spec.Service.Type == nil {
		st := corev1.ServiceTypeClusterIP
		cr.Spec.Service.Type = &st
	}

	if cr.Spec.Service.Port == 0 {
		cr.Spec.Service.Port = 9080
	}

	if cr.Spec.Service.Provides != nil && cr.Spec.Service.Provides.Protocol == "" {
		cr.Spec.Service.Provides.Protocol = "http"
	}

}

// GetLabels returns set of labels to be added to all resources
func (cr *OpenLibertyApplication) GetLabels() map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/name":       cr.Name,
		"app.kubernetes.io/managed-by": "open-liberty-operator",
		"app.kubernetes.io/component":  "backend",
		"app.kubernetes.io/part-of":    cr.Spec.ApplicationName,
	}

	if cr.Spec.Version != "" {
		labels["app.kubernetes.io/version"] = cr.Spec.Version
	}

	for key, value := range cr.Labels {
		if key != "app.kubernetes.io/instance" {
			labels[key] = value
		}
	}

	if cr.Spec.Service != nil && cr.Spec.Service.Provides != nil {
		labels["service.app.stacks/bindable"] = "true"
	}

	return labels
}

// GetAnnotations returns set of annotations to be added to all resources
func (cr *OpenLibertyApplication) GetAnnotations() map[string]string {
	annotations := map[string]string{}
	for k, v := range cr.Annotations {
		annotations[k] = v
	}
	delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
	return annotations
}

// GetType returns status condition type
func (c *StatusCondition) GetType() common.StatusConditionType {
	return convertToCommonStatusConditionType(c.Type)
}

// SetType returns status condition type
func (c *StatusCondition) SetType(ct common.StatusConditionType) {
	c.Type = convertFromCommonStatusConditionType(ct)
}

// GetLastTransitionTime return time of last status change
func (c *StatusCondition) GetLastTransitionTime() *metav1.Time {
	return c.LastTransitionTime
}

// SetLastTransitionTime sets time of last status change
func (c *StatusCondition) SetLastTransitionTime(t *metav1.Time) {
	c.LastTransitionTime = t
}

// GetLastUpdateTime return time of last status update
func (c *StatusCondition) GetLastUpdateTime() metav1.Time {
	return c.LastUpdateTime
}

// SetLastUpdateTime sets time of last status update
func (c *StatusCondition) SetLastUpdateTime(t metav1.Time) {
	c.LastUpdateTime = t
}

// GetMessage return condition's message
func (c *StatusCondition) GetMessage() string {
	return c.Message
}

// SetMessage sets condition's message
func (c *StatusCondition) SetMessage(m string) {
	c.Message = m
}

// GetReason return condition's message
func (c *StatusCondition) GetReason() string {
	return c.Reason
}

// SetReason sets condition's reason
func (c *StatusCondition) SetReason(r string) {
	c.Reason = r
}

// GetStatus return condition's status
func (c *StatusCondition) GetStatus() corev1.ConditionStatus {
	return c.Status
}

// SetStatus sets condition's status
func (c *StatusCondition) SetStatus(s corev1.ConditionStatus) {
	c.Status = s
}

// NewCondition returns new condition
func (s *OpenLibertyApplicationStatus) NewCondition() common.StatusCondition {
	return &StatusCondition{}
}

// GetConditions returns slice of conditions
func (s *OpenLibertyApplicationStatus) GetConditions() []common.StatusCondition {
	var conditions = make([]common.StatusCondition, len(s.Conditions))
	for i := range s.Conditions {
		conditions[i] = &s.Conditions[i]
	}
	return conditions
}

// GetCondition ...
func (s *OpenLibertyApplicationStatus) GetCondition(t common.StatusConditionType) common.StatusCondition {
	for i := range s.Conditions {
		if s.Conditions[i].GetType() == t {
			return &s.Conditions[i]
		}
	}
	return nil
}

// SetCondition ...
func (s *OpenLibertyApplicationStatus) SetCondition(c common.StatusCondition) {
	condition := &StatusCondition{}
	found := false
	for i := range s.Conditions {
		if s.Conditions[i].GetType() == c.GetType() {
			condition = &s.Conditions[i]
			found = true
		}
	}

	if condition.GetStatus() != c.GetStatus() {
		condition.SetLastTransitionTime(&metav1.Time{Time: time.Now()})
	}

	condition.SetLastUpdateTime(metav1.Time{Time: time.Now()})
	condition.SetReason(c.GetReason())
	condition.SetMessage(c.GetMessage())
	condition.SetStatus(c.GetStatus())
	condition.SetType(c.GetType())
	if !found {
		s.Conditions = append(s.Conditions, *condition)
	}
}

func convertToCommonStatusConditionType(c StatusConditionType) common.StatusConditionType {
	switch c {
	case StatusConditionTypeReconciled:
		return common.StatusConditionTypeReconciled
	case StatusConditionTypeDependenciesSatisfied:
		return common.StatusConditionTypeDependenciesSatisfied
	default:
		panic(c)
	}
}

func convertFromCommonStatusConditionType(c common.StatusConditionType) StatusConditionType {
	switch c {
	case common.StatusConditionTypeReconciled:
		return StatusConditionTypeReconciled
	case common.StatusConditionTypeDependenciesSatisfied:
		return StatusConditionTypeDependenciesSatisfied
	default:
		panic(c)
	}
}
