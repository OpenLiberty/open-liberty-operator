package v1

import (
	"time"

	"github.com/application-stacks/runtime-component-operator/common"
	routev1 "github.com/openshift/api/route/v1"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Defines the desired state of OpenLibertyApplication.
type OpenLibertyApplicationSpec struct {

	// Application image to deploy.
	// +operator-sdk:csv:customresourcedefinitions:order=1,type=spec,displayName="Application Image",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	ApplicationImage string `json:"applicationImage"`

	// Name of the application. Defaults to the name of this custom resource.
	// +operator-sdk:csv:customresourcedefinitions:order=2,type=spec,displayName="Application Name",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	ApplicationName string `json:"applicationName,omitempty"`

	// Version of the application.
	// +operator-sdk:csv:customresourcedefinitions:order=3,type=spec,displayName="Application Version",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	ApplicationVersion string `json:"applicationVersion,omitempty"`

	// Policy for pulling container images. Defaults to IfNotPresent.
	// +operator-sdk:csv:customresourcedefinitions:order=4,type=spec,displayName="Pull Policy",xDescriptors="urn:alm:descriptor:com.tectonic.ui:imagePullPolicy"
	PullPolicy *corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// Name of the Secret to use to pull images from the specified repository. It is not required if the cluster is configured with a global image pull secret.
	// +operator-sdk:csv:customresourcedefinitions:order=5,type=spec,displayName="Pull Secret",xDescriptors="urn:alm:descriptor:io.kubernetes:Secret"
	PullSecret *string `json:"pullSecret,omitempty"`

	// Deprecated. .spec.serviceAccount.name should be used instead. If both are specified, .spec.serviceAccount.name will override this.
	// +operator-sdk:csv:customresourcedefinitions:order=6,type=spec,displayName="Service Account Name",xDescriptors="urn:alm:descriptor:com.tectonic.ui:hidden"
	ServiceAccountName *string `json:"serviceAccountName,omitempty"`

	// The service account to use for deploying the application. A service account is automatically created if this is not specifed.
	// +operator-sdk:csv:customresourcedefinitions:order=6,type=spec,displayName="Service Account"
	ServiceAccount *OpenLibertyApplicationServiceAccount `json:"serviceAccount,omitempty"`

	// Create Knative resources and use Knative serving.
	// +operator-sdk:csv:customresourcedefinitions:order=7,type=spec,displayName="Create Knative Service",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	CreateKnativeService *bool `json:"createKnativeService,omitempty"`

	// Expose the application externally via a Route, a Knative Route or an Ingress resource.
	// +operator-sdk:csv:customresourcedefinitions:order=8,type=spec,displayName="Expose",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	Expose *bool `json:"expose,omitempty"`

	// Enable management of LTPA key sharing amongst Liberty containers. Defaults to false.
	// +operator-sdk:csv:customresourcedefinitions:order=9,type=spec,displayName="Manage LTPA",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	ManageLTPA *bool `json:"manageLTPA,omitempty"`

	// Enable management of TLS certificates. Defaults to true.
	// +operator-sdk:csv:customresourcedefinitions:order=10,type=spec,displayName="Manage TLS",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	ManageTLS *bool `json:"manageTLS,omitempty"`

	// Number of pods to create. Defaults to 1. Not applicable when .spec.autoscaling or .spec.createKnativeService is specified.
	// +operator-sdk:csv:customresourcedefinitions:order=11,type=spec,displayName="Replicas",xDescriptors="urn:alm:descriptor:com.tectonic.ui:podCount"
	Replicas *int32 `json:"replicas,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=12,type=spec,displayName="Auto Scaling"
	Autoscaling *OpenLibertyApplicationAutoScaling `json:"autoscaling,omitempty"`

	// Resource requests and limits for the application container.
	// +operator-sdk:csv:customresourcedefinitions:order=13,type=spec,displayName="Resource Requirements",xDescriptors="urn:alm:descriptor:com.tectonic.ui:resourceRequirements"
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=14,type=spec,displayName="Probes"
	Probes *OpenLibertyApplicationProbes `json:"probes,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=15,type=spec,displayName="Deployment"
	Deployment *OpenLibertyApplicationDeployment `json:"deployment,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=16,type=spec,displayName="StatefulSet"
	StatefulSet *OpenLibertyApplicationStatefulSet `json:"statefulSet,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=17,type=spec,displayName="Service"
	Service *OpenLibertyApplicationService `json:"service,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=18,type=spec,displayName="Route"
	Route *OpenLibertyApplicationRoute `json:"route,omitempty"`

	// Configures the Semeru Cloud Compiler to handle Just-In-Time (JIT) compilation requests from the application.
	// +operator-sdk:csv:customresourcedefinitions:order=19,type=spec,displayName="Semeru Cloud Compiler"
	SemeruCloudCompiler *OpenLibertyApplicationSemeruCloudCompiler `json:"semeruCloudCompiler,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=20,type=spec,displayName="Network Policy"
	NetworkPolicy *OpenLibertyApplicationNetworkPolicy `json:"networkPolicy,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=21,type=spec,displayName="Serviceability"
	Serviceability *OpenLibertyApplicationServiceability `json:"serviceability,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=22,type=spec,displayName="Single Sign-On"
	SSO *OpenLibertyApplicationSSO `json:"sso,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=23,type=spec,displayName="Monitoring"
	Monitoring *OpenLibertyApplicationMonitoring `json:"monitoring,omitempty"`

	// An array of environment variables for the application container.
	// +listType=map
	// +listMapKey=name
	// +operator-sdk:csv:customresourcedefinitions:order=24,type=spec,displayName="Environment Variables"
	Env []corev1.EnvVar `json:"env,omitempty"`

	// List of sources to populate environment variables in the application container.
	// +listType=atomic
	// +operator-sdk:csv:customresourcedefinitions:order=25,type=spec,displayName="Environment Variables from Sources"
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// Represents a volume with data that is accessible to the application container.
	// +listType=map
	// +listMapKey=name
	// +operator-sdk:csv:customresourcedefinitions:order=26,type=spec,displayName="Volumes"
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Represents where to mount the volumes into the application container.
	// +listType=atomic
	// +operator-sdk:csv:customresourcedefinitions:order=27,type=spec,displayName="Volume Mounts"
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// List of containers to run before other containers in a pod.
	// +listType=map
	// +listMapKey=name
	// +operator-sdk:csv:customresourcedefinitions:order=28,type=spec,displayName="Init Containers"
	InitContainers []corev1.Container `json:"initContainers,omitempty"`

	// List of sidecar containers. These are additional containers to be added to the pods.
	// +listType=map
	// +listMapKey=name
	// +operator-sdk:csv:customresourcedefinitions:order=29,type=spec,displayName="Sidecar Containers"
	SidecarContainers []corev1.Container `json:"sidecarContainers,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=30,type=spec,displayName="Affinity"
	Affinity *OpenLibertyApplicationAffinity `json:"affinity,omitempty"`

	// Security context for the application container.
	// +operator-sdk:csv:customresourcedefinitions:order=31,type=spec,displayName="Security Context"
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=32,type=spec,displayName="Topology Spread Constraints"
	TopologySpreadConstraints *OpenLibertyApplicationTopologySpreadConstraints `json:"topologySpreadConstraints,omitempty"`
}

// Defines the topology spread constraints
type OpenLibertyApplicationTopologySpreadConstraints struct {
	// The list of TopologySpreadConstraints for the application instance and if applicable, the Semeru Cloud Compiler instance.
	// +operator-sdk:csv:customresourcedefinitions:order=1,type=spec,displayName="Constraints"
	Constraints *[]corev1.TopologySpreadConstraint `json:"constraints,omitempty"`

	// Whether the operator should disable its default set of TopologySpreadConstraints. Defaults to false.
	// +operator-sdk:csv:customresourcedefinitions:order=1,type=spec,displayName="Disable Operator Defaults",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	DisableOperatorDefaults *bool `json:"disableOperatorDefaults,omitempty"`
}

// Defines the service account
type OpenLibertyApplicationServiceAccount struct {
	// Whether the Service Account token should be mounted into the application pods. Defaults to true.
	// +operator-sdk:csv:customresourcedefinitions:order=1,type=spec,displayName="Mount Service Account Token",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	MountToken *bool `json:"mountToken,omitempty"`

	// Name of the service account to use for deploying the application. A service account is automatically created if it's not specified.
	// +operator-sdk:csv:customresourcedefinitions:order=2,type=spec,displayName="Service Account Name",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	Name *string `json:"name,omitempty"`
}

// Define health checks on application container to determine whether it is alive or ready to receive traffic
type OpenLibertyApplicationProbes struct {
	// Periodic probe of container liveness. Container will be restarted if the probe fails.
	// +operator-sdk:csv:customresourcedefinitions:order=49,type=spec,displayName="Liveness Probe"
	Liveness *corev1.Probe `json:"liveness,omitempty"`

	// Periodic probe of container service readiness. Container will be removed from service endpoints if the probe fails.
	// +operator-sdk:csv:customresourcedefinitions:order=50,type=spec,displayName="Readiness Probe"
	Readiness *corev1.Probe `json:"readiness,omitempty"`

	// Probe to determine successful initialization. If specified, other probes are not executed until this completes successfully.
	// +operator-sdk:csv:customresourcedefinitions:order=51,type=spec,displayName="Startup Probe"
	Startup *corev1.Probe `json:"startup,omitempty"`
}

// Configure pods to run on particular Nodes.
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
type OpenLibertyApplicationAutoScaling struct {
	// Required field for autoscaling. Upper limit for the number of pods that can be set by the autoscaler. Parameter .spec.resources.requests.cpu must also be specified.
	// +kubebuilder:validation:Minimum=1
	// +operator-sdk:csv:customresourcedefinitions:order=1,type=spec,displayName="Max Replicas",xDescriptors="urn:alm:descriptor:com.tectonic.ui:number"
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// Lower limit for the number of pods that can be set by the autoscaler.
	// +operator-sdk:csv:customresourcedefinitions:order=2,type=spec,displayName="Min Replicas",xDescriptors="urn:alm:descriptor:com.tectonic.ui:number"
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// Target average CPU utilization, represented as a percentage of requested CPU, over all the pods.
	// +operator-sdk:csv:customresourcedefinitions:order=3,type=spec,displayName="Target CPU Utilization Percentage",xDescriptors="urn:alm:descriptor:com.tectonic.ui:number"
	TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`
}

// Configures parameters for the network service of pods.
type OpenLibertyApplicationService struct {
	// The port exposed by the container.
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:validation:Minimum=1
	// +operator-sdk:csv:customresourcedefinitions:order=9,type=spec,displayName="Service Port",xDescriptors="urn:alm:descriptor:com.tectonic.ui:number"
	Port int32 `json:"port,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=10,type=spec,displayName="Service Type",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	Type *corev1.ServiceType `json:"type,omitempty"`

	// Node proxies this port into your service.
	// +kubebuilder:validation:Maximum=32767
	// +kubebuilder:validation:Minimum=30000
	// +operator-sdk:csv:customresourcedefinitions:order=11,type=spec,displayName="Node Port",xDescriptors="urn:alm:descriptor:com.tectonic.ui:number"
	NodePort *int32 `json:"nodePort,omitempty"`

	// The name for the port exposed by the container.
	// +operator-sdk:csv:customresourcedefinitions:order=12,type=spec,displayName="Port Name",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	PortName string `json:"portName,omitempty"`

	// Annotations to be added to the service.
	// +operator-sdk:csv:customresourcedefinitions:order=13,type=spec,displayName="Service Annotations",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	Annotations map[string]string `json:"annotations,omitempty"`

	// The port that the operator assigns to containers inside pods. Defaults to the value of .spec.service.port.
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:validation:Minimum=1
	// +operator-sdk:csv:customresourcedefinitions:order=14,type=spec,displayName="Target Port",xDescriptors="urn:alm:descriptor:com.tectonic.ui:number"
	TargetPort *int32 `json:"targetPort,omitempty"`

	// A name of a secret that already contains TLS key, certificate and CA to be mounted in the pod. The following keys are valid in the secret: ca.crt, tls.crt, and tls.key.
	// +operator-sdk:csv:customresourcedefinitions:order=15,type=spec,displayName="Certificate Secret Reference",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	CertificateSecretRef *string `json:"certificateSecretRef,omitempty"`

	// Configure service certificate.
	// +operator-sdk:csv:customresourcedefinitions:order=16,type=spec,displayName="Service Certificate"
	Certificate *OpenLibertyApplicationCertificate `json:"certificate,omitempty"`

	// An array consisting of service ports.
	// +operator-sdk:csv:customresourcedefinitions:order=17,type=spec
	Ports []corev1.ServicePort `json:"ports,omitempty"`

	// Expose the application as a bindable service. Defaults to false.
	// +operator-sdk:csv:customresourcedefinitions:order=18,type=spec,displayName="Bindable",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	Bindable *bool `json:"bindable,omitempty"`
}

// Configure service certificate.
type OpenLibertyApplicationCertificate struct {
	// Annotations to be added to the service certificate.
	// +operator-sdk:csv:customresourcedefinitions:order=13,type=spec,displayName="Annotations",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Defines the network policy
type OpenLibertyApplicationNetworkPolicy struct {
	// Disable the creation of the network policy. Defaults to false.
	// +operator-sdk:csv:customresourcedefinitions:order=52,type=spec,displayName="Disable",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	Disable *bool `json:"disable,omitempty"`

	// Specify the labels of namespaces that incoming traffic is allowed from.
	// +operator-sdk:csv:customresourcedefinitions:order=53,type=spec,displayName="Namespace Labels",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	NamespaceLabels *map[string]string `json:"namespaceLabels,omitempty"`

	// Specify the labels of pod(s) that incoming traffic is allowed from.
	// +operator-sdk:csv:customresourcedefinitions:order=54,type=spec,displayName="From Labels",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	FromLabels *map[string]string `json:"fromLabels,omitempty"`
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
	// +operator-sdk:csv:customresourcedefinitions:order=23,type=spec,displayName="StatefulSet Update Strategy"
	UpdateStrategy *appsv1.StatefulSetUpdateStrategy `json:"updateStrategy,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=24,type=spec,displayName="Storage"
	Storage *OpenLibertyApplicationStorage `json:"storage,omitempty"`

	// Annotations to be added only to the StatefulSet and resources owned by the StatefulSet.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Defines settings of persisted storage for StatefulSets.
type OpenLibertyApplicationStorage struct {
	// A convenient field to set the size of the persisted storage.
	// +kubebuilder:validation:Pattern=^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$
	// +operator-sdk:csv:customresourcedefinitions:order=25,type=spec,displayName="Storage Size",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	Size string `json:"size,omitempty"`

	// A convenient field to request the storage class of the persisted storage. The name can not be specified or updated after the storage is created.
	// +kubebuilder:validation:Pattern=.+
	// +operator-sdk:csv:customresourcedefinitions:order=26,type=spec,displayName="Storage Class Name",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	ClassName string `json:"className,omitempty"`

	// The directory inside the container where this persisted storage will be bound to.
	// +operator-sdk:csv:customresourcedefinitions:order=27,type=spec,displayName="Storage Mount Path",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	MountPath string `json:"mountPath,omitempty"`

	// A YAML object that represents a volumeClaimTemplate component of a StatefulSet.
	// +operator-sdk:csv:customresourcedefinitions:order=28,type=spec,displayName="Storage Volume Claim Template",xDescriptors="urn:alm:descriptor:com.tectonic.ui:PersistentVolumeClaim"
	VolumeClaimTemplate *corev1.PersistentVolumeClaim `json:"volumeClaimTemplate,omitempty"`
}

// Specifies parameters for Service Monitor.
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

	// Path type to be used for Ingress. This does not apply to Route on OpenShift.
	// +operator-sdk:csv:customresourcedefinitions:order=44,type=spec,displayName="Path Type",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:select:Exact", "urn:alm:descriptor:com.tectonic.ui:select:Prefix", "urn:alm:descriptor:com.tectonic.ui:select:ImplementationSpecific"}
	PathType networkingv1.PathType `json:"pathType,omitempty"`

	// A name of a secret that already contains TLS key, certificate and CA to be used in the route. It can also contain destination CA certificate. The following keys are valid in the secret: ca.crt, destCA.crt, tls.crt, and tls.key.
	// +operator-sdk:csv:customresourcedefinitions:order=45,type=spec,displayName="Certificate Secret Reference",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	CertificateSecretRef *string `json:"certificateSecretRef,omitempty"`

	// TLS termination policy. Can be one of edge, reencrypt and passthrough.
	// +operator-sdk:csv:customresourcedefinitions:order=46,type=spec,displayName="Termination",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:select:edge", "urn:alm:descriptor:com.tectonic.ui:select:reencrypt", "urn:alm:descriptor:com.tectonic.ui:select:passthrough"}
	Termination *routev1.TLSTerminationType `json:"termination,omitempty"`

	// HTTP traffic policy with TLS enabled. Can be one of Allow, Redirect and None.
	// +operator-sdk:csv:customresourcedefinitions:order=47,type=spec,displayName="Insecure Edge Termination Policy",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:select:Allow", "urn:alm:descriptor:com.tectonic.ui:select:Redirect", "urn:alm:descriptor:com.tectonic.ui:select:None"}
	InsecureEdgeTerminationPolicy *routev1.InsecureEdgeTerminationPolicyType `json:"insecureEdgeTerminationPolicy,omitempty"`
}

type OpenLibertyApplicationSemeruCloudCompiler struct {
	// Enable the Semeru Cloud Compiler. Defaults to false.
	// +operator-sdk:csv:customresourcedefinitions:order=52,type=spec,displayName="Enable",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	Enable bool `json:"enable,omitempty"`
	// Number of desired pods for the Semeru Cloud Compiler. Defaults to 1.
	// +operator-sdk:csv:customresourcedefinitions:order=53,type=spec,displayName="Replicas",xDescriptors="urn:alm:descriptor:com.tectonic.ui:podCount"
	Replicas *int32 `json:"replicas,omitempty"`
	// Resource requests and limits for the Semeru Cloud Compiler. The CPU defaults to 100m with a limit of 2000m. The memory defaults to 800Mi, with a limit of 1200Mi.
	// +operator-sdk:csv:customresourcedefinitions:order=54,type=spec,displayName="Resource Requirements",xDescriptors="urn:alm:descriptor:com.tectonic.ui:resourceRequirements"
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// Defines SemeruCompiler status
type SemeruCompilerStatus struct {
	TLSSecretName   string `json:"tlsSecretName,omitempty"`
	ServiceHostname string `json:"serviceHostname,omitempty"`
}

// Defines the observed state of OpenLibertyApplication.
type OpenLibertyApplicationStatus struct {
	// +listType=atomic
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Status Conditions",xDescriptors="urn:alm:descriptor:io.kubernetes.conditions"
	Conditions     []StatusCondition `json:"conditions,omitempty"`
	Endpoints      []StatusEndpoint  `json:"endpoints,omitempty"`
	RouteAvailable *bool             `json:"routeAvailable,omitempty"`
	ImageReference string            `json:"imageReference,omitempty"`
	Versions       StatusVersions    `json:"versions,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=61,type=status,displayName="Service Binding"
	Binding *corev1.LocalObjectReference `json:"binding,omitempty"`

	References common.StatusReferences `json:"references,omitempty"`

	SemeruCompiler *SemeruCompilerStatus `json:"semeruCompiler,omitempty"`

	// The generation identifier of this OpenLibertyApplication instance completely reconciled by the Operator.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Defines possible status conditions.
type StatusCondition struct {
	LastTransitionTime *metav1.Time           `json:"lastTransitionTime,omitempty"`
	Reason             string                 `json:"reason,omitempty"`
	Message            string                 `json:"message,omitempty"`
	Status             corev1.ConditionStatus `json:"status,omitempty"`
	Type               StatusConditionType    `json:"type,omitempty"`
}

// Defines the type of status condition.
type StatusConditionType string

// Reports endpoint information.
type StatusEndpoint struct {
	Name  string              `json:"name,omitempty"`
	Scope StatusEndpointScope `json:"scope,omitempty"`
	Type  string              `json:"type,omitempty"`
	// Exposed URI of the application endpoint
	// +operator-sdk:csv:customresourcedefinitions:order=60,type=status,displayName="Application",xDescriptors={"urn:alm:descriptor:org.w3:link"}
	URI string `json:"uri,omitempty"`
}

// Defines the scope of endpoint information in status.
type StatusEndpointScope string

const (
	// Status Condition Types
	StatusConditionTypeReconciled     StatusConditionType = "Reconciled"
	StatusConditionTypeResourcesReady StatusConditionType = "ResourcesReady"
	StatusConditionTypeReady          StatusConditionType = "Ready"

	// Status Endpoint Scopes
	StatusEndpointScopeExternal StatusEndpointScope = "External"
	StatusEndpointScopeInternal StatusEndpointScope = "Internal"
)

type StatusVersions struct {
	Reconciled string `json:"reconciled,omitempty"`
}

// +kubebuilder:resource:path=openlibertyapplications,scope=Namespaced,shortName=olapp;olapps
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Image",type="string",JSONPath=".spec.applicationImage",priority=0,description="Absolute name of the deployed image containing registry and tag"
// +kubebuilder:printcolumn:name="Exposed",type="boolean",JSONPath=".spec.expose",priority=0,description="Specifies whether deployment is exposed externally via default Route"
// +kubebuilder:printcolumn:name="Reconciled",type="string",JSONPath=".status.conditions[?(@.type=='Reconciled')].status",priority=0,description="Status of the reconcile condition"
// +kubebuilder:printcolumn:name="ReconciledReason",type="string",JSONPath=".status.conditions[?(@.type=='Reconciled')].reason",priority=1,description="Reason for the failure of reconcile condition"
// +kubebuilder:printcolumn:name="ReconciledMessage",type="string",JSONPath=".status.conditions[?(@.type=='Reconciled')].message",priority=1,description="Failure message from reconcile condition"
// +kubebuilder:printcolumn:name="ResourcesReady",type="string",JSONPath=".status.conditions[?(@.type=='ResourcesReady')].status",priority=0,description="Status of the resource ready condition"
// +kubebuilder:printcolumn:name="ResourcesReadyReason",type="string",JSONPath=".status.conditions[?(@.type=='ResourcesReady')].reason",priority=1,description="Reason for the failure of resource ready condition"
// +kubebuilder:printcolumn:name="ResourcesReadyMessage",type="string",JSONPath=".status.conditions[?(@.type=='ResourcesReady')].message",priority=1,description="Failure message from resource ready condition"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status",priority=0,description="Status of the component ready condition"
// +kubebuilder:printcolumn:name="ReadyReason",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].reason",priority=1,description="Reason for the failure of component ready condition"
// +kubebuilder:printcolumn:name="ReadyMessage",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].message",priority=1,description="Failure message from component ready condition"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",priority=0,description="Age of the resource"
// +operator-sdk:csv:customresourcedefinitions:displayName="OpenLibertyApplication",resources={{Deployment,v1},{Service,v1},{StatefulSet,v1},{Route,v1},{HorizontalPodAutoscaler,v1},{ServiceAccount,v1},{Secret,v1},{NetworkPolicy,v1}}

// Represents the deployment of an Open Liberty application
type OpenLibertyApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenLibertyApplicationSpec   `json:"spec,omitempty"`
	Status OpenLibertyApplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenLibertyApplicationList contains a list of OpenLibertyApplication
type OpenLibertyApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenLibertyApplication `json:"items"`
}

// Specifies the configuration for Single Sign-On (SSO) providers to authenticate with.
type OpenLibertyApplicationSSO struct {
	// +listType=atomic
	// +operator-sdk:csv:customresourcedefinitions:order=1,type=spec,displayName="OIDC"
	OIDC []OidcClient `json:"oidc,omitempty"`

	// +listType=atomic
	// +operator-sdk:csv:customresourcedefinitions:order=2,type=spec,displayName="OAuth2"
	Oauth2 []OAuth2Client `json:"oauth2,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:order=3,type=spec,displayName="GitHub"
	Github *GithubLogin `json:"github,omitempty"`

	// Common parameters for all SSO providers

	// Specifies a callback protocol, host and port number.
	// +operator-sdk:csv:customresourcedefinitions:order=4,type=spec,displayName="Redirect to RP Host and Port"
	RedirectToRPHostAndPort string `json:"redirectToRPHostAndPort,omitempty"`

	// Specifies whether to map a user identifier to a registry user. This parameter applies to all providers.
	// +operator-sdk:csv:customresourcedefinitions:order=5,type=spec,displayName="Map to User Registry",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	MapToUserRegistry *bool `json:"mapToUserRegistry,omitempty"`
}

// Represents configuration for an OpenID Connect (OIDC) client.
type OidcClient struct {
	// The unique ID for the provider. Default value is oidc.
	// +operator-sdk:csv:customresourcedefinitions:order=1,type=spec,displayName="ID"
	ID string `json:"id,omitempty"`

	// Specifies a discovery endpoint URL for the OpenID Connect provider. Required field.
	// +operator-sdk:csv:customresourcedefinitions:order=2,type=spec
	DiscoveryEndpoint string `json:"discoveryEndpoint"`

	// Specifies the name of the claim. Use its value as the user group membership.
	// +operator-sdk:csv:customresourcedefinitions:order=3,type=spec
	GroupNameAttribute string `json:"groupNameAttribute,omitempty"`

	// Specifies the name of the claim. Use its value as the authenticated user principal.
	// +operator-sdk:csv:customresourcedefinitions:order=4,type=spec
	UserNameAttribute string `json:"userNameAttribute,omitempty"`

	// The name of the social login configuration for display.
	// +operator-sdk:csv:customresourcedefinitions:order=5,type=spec
	DisplayName string `json:"displayName,omitempty"`

	// Specifies whether the UserInfo endpoint is contacted.
	// +operator-sdk:csv:customresourcedefinitions:order=6,type=spec,displayName="User Info Endpoint Enabled",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	UserInfoEndpointEnabled *bool `json:"userInfoEndpointEnabled,omitempty"`

	// Specifies the name of the claim. Use its value as the subject realm.
	// +operator-sdk:csv:customresourcedefinitions:order=7,type=spec
	RealmNameAttribute string `json:"realmNameAttribute,omitempty"`

	// Specifies one or more scopes to request.
	// +operator-sdk:csv:customresourcedefinitions:order=8,type=spec
	Scope string `json:"scope,omitempty"`

	// Specifies the required authentication method.
	// +operator-sdk:csv:customresourcedefinitions:order=9,type=spec
	TokenEndpointAuthMethod string `json:"tokenEndpointAuthMethod,omitempty"`

	// Specifies whether to enable host name verification when the client contacts the provider.
	// +operator-sdk:csv:customresourcedefinitions:order=10,type=spec,displayName="Host Name Verification Enabled",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	HostNameVerificationEnabled *bool `json:"hostNameVerificationEnabled,omitempty"`
}

// Represents configuration for an OAuth2 client.
type OAuth2Client struct {
	// Specifies the unique ID for the provider. The default value is oauth2.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="ID"
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
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	AccessTokenRequired *bool `json:"accessTokenRequired,omitempty"`

	// Determines whether to support access token authentication if an access token is provided in the request.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	AccessTokenSupported *bool `json:"accessTokenSupported,omitempty"`

	// Indicates which specification to use for the user API.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="User API Type"
	UserApiType string `json:"userApiType,omitempty"`

	// The URL for retrieving the user information.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="User API"
	UserApi string `json:"userApi,omitempty"`
}

// Represents configuration for social login using GitHub.
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

func (cr *OpenLibertyApplication) GetServiceAccount() common.BaseComponentServiceAccount {
	if cr.Spec.ServiceAccount == nil {
		return nil
	}
	return cr.Spec.ServiceAccount
}

// GetMountToken returns whether the service account token should be mounted
func (sa *OpenLibertyApplicationServiceAccount) GetMountToken() *bool {
	return sa.MountToken
}

// GetName returns the service account name
func (sa *OpenLibertyApplicationServiceAccount) GetName() *string {
	return sa.Name
}

// GetReplicas returns number of replicas
func (cr *OpenLibertyApplication) GetReplicas() *int32 {
	return cr.Spec.Replicas
}

func (cr *OpenLibertyApplication) GetProbes() common.BaseComponentProbes {
	if cr.Spec.Probes == nil {
		return nil
	}
	return cr.Spec.Probes
}

// GetLivenessProbe returns liveness probe
func (p *OpenLibertyApplicationProbes) GetLivenessProbe() *corev1.Probe {
	return p.Liveness
}

// GetReadinessProbe returns readiness probe
func (p *OpenLibertyApplicationProbes) GetReadinessProbe() *corev1.Probe {
	return p.Readiness
}

// GetStartupProbe returns startup probe
func (p *OpenLibertyApplicationProbes) GetStartupProbe() *corev1.Probe {
	return p.Startup
}

// GetDefaultLivenessProbe returns default values for liveness probe
func (p *OpenLibertyApplicationProbes) GetDefaultLivenessProbe(ba common.BaseComponent) *corev1.Probe {
	return common.GetDefaultMicroProfileLivenessProbe(ba)
}

// GetDefaultReadinessProbe returns default values for readiness probe
func (p *OpenLibertyApplicationProbes) GetDefaultReadinessProbe(ba common.BaseComponent) *corev1.Probe {
	return common.GetDefaultMicroProfileReadinessProbe(ba)
}

// GetDefaultStartupProbe returns default values for startup probe
func (p *OpenLibertyApplicationProbes) GetDefaultStartupProbe(ba common.BaseComponent) *corev1.Probe {
	return common.GetDefaultMicroProfileStartupProbe(ba)
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
	return cr.Spec.Resources
}

// GetExpose returns expose flag
func (cr *OpenLibertyApplication) GetExpose() *bool {
	return cr.Spec.Expose
}

// GetManageLTPA returns the LTPA key sharing status
func (cr *OpenLibertyApplication) GetManageLTPA() *bool {
	return cr.Spec.ManageLTPA
}

// GetManageTLS returns deployment's node and pod affinity settings
func (cr *OpenLibertyApplication) GetManageTLS() *bool {
	return cr.Spec.ManageTLS
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

// GetAutoscaling returns autoscaling settings
func (cr *OpenLibertyApplication) GetAutoscaling() common.BaseComponentAutoscaling {
	if cr.Spec.Autoscaling == nil {
		return nil
	}
	return cr.Spec.Autoscaling
}

// GetStorage returns storage settings
func (ss *OpenLibertyApplicationStatefulSet) GetStorage() common.BaseComponentStorage {
	if ss.Storage == nil {
		return nil
	}
	return ss.Storage
}

// GetService returns service settings
func (cr *OpenLibertyApplication) GetService() common.BaseComponentService {
	if cr.Spec.Service == nil {
		return nil
	}
	return cr.Spec.Service
}

// GetNetworkPolicy returns network policy settings
func (cr *OpenLibertyApplication) GetNetworkPolicy() common.BaseComponentNetworkPolicy {
	if cr.Spec.NetworkPolicy == nil {
		return nil
	}
	return cr.Spec.NetworkPolicy
}

// GetApplicationVersion returns application version
func (cr *OpenLibertyApplication) GetApplicationVersion() string {
	return cr.Spec.ApplicationVersion
}

// GetApplicationName returns Application name
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
	return "apps.openliberty.io"
}

// GetRoute returns route
func (cr *OpenLibertyApplication) GetRoute() common.BaseComponentRoute {
	if cr.Spec.Route == nil {
		return nil
	}
	return cr.Spec.Route
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

// GetAnnotations returns annotations to be added only to the StatefulSet and its child resources
func (rcss *OpenLibertyApplicationStatefulSet) GetAnnotations() map[string]string {
	return rcss.Annotations
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

func (s *OpenLibertyApplicationStatus) GetReferences() common.StatusReferences {
	if s.References == nil {
		s.References = make(common.StatusReferences)
	}
	return s.References
}

func (s *OpenLibertyApplicationStatus) SetReferences(refs common.StatusReferences) {
	s.References = refs
}

func (s *OpenLibertyApplicationStatus) SetReference(name string, value string) {
	if s.References == nil {
		s.References = make(common.StatusReferences)
	}
	s.References[name] = value
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

// GetClassName returns persistent volume ClassName
func (s *OpenLibertyApplicationStorage) GetClassName() string {
	return s.ClassName
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
	return 9443
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

// GetCertificateSecretRef returns a secret reference with a certificate
func (s *OpenLibertyApplicationService) GetCertificateSecretRef() *string {
	return s.CertificateSecretRef
}

// GetCertificate returns a service certificate configuration
func (s *OpenLibertyApplicationService) GetCertificate() common.BaseComponentCertificate {
	if s.Certificate == nil {
		return nil
	}
	return s.Certificate
}

// GetAnnotations returns annotations to be added to certificate request
func (c *OpenLibertyApplicationCertificate) GetAnnotations() map[string]string {
	return c.Annotations
}

// GetBindable returns whether the application should be exposable as a service
func (s *OpenLibertyApplicationService) GetBindable() *bool {
	return s.Bindable
}

// GetNamespaceLabels returns the namespace selector labels that should be used for the ingress rule
func (np *OpenLibertyApplicationNetworkPolicy) GetNamespaceLabels() map[string]string {
	if np.NamespaceLabels != nil {
		return *np.NamespaceLabels
	}
	return nil
}

// GetFromLabels returns the pod selector labels that should be used for the ingress rule
func (np *OpenLibertyApplicationNetworkPolicy) GetFromLabels() map[string]string {
	if np.FromLabels != nil {
		return *np.FromLabels
	}
	return nil
}

// IsDisabled returns whether the network policy should be created or not
func (np *OpenLibertyApplicationNetworkPolicy) IsDisabled() bool {
	return np.Disable != nil && *np.Disable
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

// GetPathType returns pathType to use for the route
func (r *OpenLibertyApplicationRoute) GetPathType() networkingv1.PathType {
	return r.PathType
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

// GetSecurityContext returns container security context
func (cr *OpenLibertyApplication) GetSecurityContext() *corev1.SecurityContext {
	return cr.Spec.SecurityContext
}

// GetSemeruCloudCompiler returns the Semeru Cloud Compiler configuration
func (cr *OpenLibertyApplication) GetSemeruCloudCompiler() *OpenLibertyApplicationSemeruCloudCompiler {
	return cr.Spec.SemeruCloudCompiler
}

// GetReplicas returns the replicas for Semeru Cloud Compiler if specified, otherwise 1
func (scc *OpenLibertyApplicationSemeruCloudCompiler) GetReplicas() *int32 {
	if scc.Replicas != nil {
		return scc.Replicas
	}
	one := int32(1)
	return &one
}

// GetTopologySpreadConstraints returns the pod topology spread constraints configuration
func (cr *OpenLibertyApplication) GetTopologySpreadConstraints() common.BaseComponentTopologySpreadConstraints {
	if cr.Spec.TopologySpreadConstraints == nil {
		return nil
	}
	return cr.Spec.TopologySpreadConstraints
}

func (cr *OpenLibertyApplicationTopologySpreadConstraints) GetConstraints() *[]corev1.TopologySpreadConstraint {
	if cr.Constraints == nil {
		return nil
	}
	return cr.Constraints
}

func (cr *OpenLibertyApplicationTopologySpreadConstraints) GetDisableOperatorDefaults() *bool {
	return cr.DisableOperatorDefaults
}

// Initialize sets default values
func (cr *OpenLibertyApplication) Initialize() {
	if cr.Spec.PullPolicy == nil {
		pp := corev1.PullIfNotPresent
		cr.Spec.PullPolicy = &pp
	}

	if cr.Spec.Resources == nil {
		cr.Spec.Resources = &corev1.ResourceRequirements{}
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
		if cr.Spec.ManageTLS == nil || *cr.Spec.ManageTLS {
			cr.Spec.Service.Port = 9443
		} else {
			cr.Spec.Service.Port = 9080
		}
	}

	// If TargetPorts on Serviceports are not set, default them to the Port value in the CR
	numOfAdditionalPorts := len(cr.GetService().GetPorts())
	for i := 0; i < numOfAdditionalPorts; i++ {
		if cr.Spec.Service.Ports[i].TargetPort.String() == "0" {
			cr.Spec.Service.Ports[i].TargetPort = intstr.FromInt(int(cr.Spec.Service.Ports[i].Port))
		}
	}
}

// GetLabels returns set of labels to be added to all resources
func (cr *OpenLibertyApplication) GetLabels() map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/instance":     cr.Name,
		"app.kubernetes.io/name":         cr.Name,
		"app.kubernetes.io/managed-by":   "open-liberty-operator",
		"app.kubernetes.io/component":    "backend",
		"app.kubernetes.io/part-of":      cr.Spec.ApplicationName,
		common.GetComponentNameLabel(cr): cr.Name,
	}

	if cr.Spec.ApplicationVersion != "" {
		labels["app.kubernetes.io/version"] = cr.Spec.ApplicationVersion
	}

	for key, value := range cr.Labels {
		if key != "app.kubernetes.io/instance" {
			labels[key] = value
		}
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

// GetLastTransitionTime returns time of last status change
func (c *StatusCondition) GetLastTransitionTime() *metav1.Time {
	return c.LastTransitionTime
}

// SetLastTransitionTime sets time of last status change
func (c *StatusCondition) SetLastTransitionTime(t *metav1.Time) {
	c.LastTransitionTime = t
}

// GetMessage returns condition's message
func (c *StatusCondition) GetMessage() string {
	return c.Message
}

// SetMessage sets condition's message
func (c *StatusCondition) SetMessage(m string) {
	c.Message = m
}

// GetReason returns condition's message
func (c *StatusCondition) GetReason() string {
	return c.Reason
}

// SetReason sets condition's reason
func (c *StatusCondition) SetReason(r string) {
	c.Reason = r
}

// GetStatus returns condition's status
func (c *StatusCondition) GetStatus() corev1.ConditionStatus {
	return c.Status
}

// SetStatus sets condition's status
func (c *StatusCondition) SetStatus(s corev1.ConditionStatus) {
	c.Status = s
}

// SetConditionFields sets status condition fields
func (c *StatusCondition) SetConditionFields(message string, reason string, status corev1.ConditionStatus) common.StatusCondition {
	c.Message = message
	c.Reason = reason
	c.Status = status
	return c
}

// NewCondition returns new condition
func (s *OpenLibertyApplicationStatus) NewCondition(ct common.StatusConditionType) common.StatusCondition {
	c := &StatusCondition{}
	c.Type = convertFromCommonStatusConditionType(ct)
	return c
}

// GetConditions returns slice of conditions
func (s *OpenLibertyApplicationStatus) GetConditions() []common.StatusCondition {
	var conditions = make([]common.StatusCondition, len(s.Conditions))
	for i := range s.Conditions {
		conditions[i] = &s.Conditions[i]
	}
	return conditions
}

// GetCondition returns status condition with status condition type
func (s *OpenLibertyApplicationStatus) GetCondition(t common.StatusConditionType) common.StatusCondition {
	for i := range s.Conditions {
		if s.Conditions[i].GetType() == t {
			return &s.Conditions[i]
		}
	}
	return nil
}

// SetCondition sets status condition
func (s *OpenLibertyApplicationStatus) SetCondition(c common.StatusCondition) {
	condition := &StatusCondition{}
	found := false
	for i := range s.Conditions {
		if s.Conditions[i].GetType() == c.GetType() {
			condition = &s.Conditions[i]
			found = true
			break
		}
	}

	if condition.GetStatus() != c.GetStatus() || condition.GetMessage() != c.GetMessage() {
		condition.SetLastTransitionTime(&metav1.Time{Time: time.Now()})
	}

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
	case StatusConditionTypeResourcesReady:
		return common.StatusConditionTypeResourcesReady
	case StatusConditionTypeReady:
		return common.StatusConditionTypeReady
	default:
		panic(c)
	}
}

func convertFromCommonStatusConditionType(c common.StatusConditionType) StatusConditionType {
	switch c {
	case common.StatusConditionTypeReconciled:
		return StatusConditionTypeReconciled
	case common.StatusConditionTypeResourcesReady:
		return StatusConditionTypeResourcesReady
	case common.StatusConditionTypeReady:
		return StatusConditionTypeReady
	default:
		panic(c)
	}
}

// GetEndpointName returns endpoint name in status
func (e *StatusEndpoint) GetEndpointName() string {
	return e.Name
}

// SetEndpointName sets endpoint name in status
func (e *StatusEndpoint) SetEndpointName(n string) {
	e.Name = n
}

// GetEndpointScope returns endpoint scope in status
func (e *StatusEndpoint) GetEndpointScope() common.StatusEndpointScope {
	return convertToCommonStatusEndpointScope(e.Scope)
}

// SetEndpointScope sets endpoint scope in status
func (e *StatusEndpoint) SetEndpointScope(s common.StatusEndpointScope) {
	e.Scope = convertFromCommonStatusEndpointScope(s)
}

// GetEndpointType returns endpoint type in status
func (e *StatusEndpoint) GetEndpointType() string {
	return e.Type
}

// SetEndpointType sets endpoint type in status
func (e *StatusEndpoint) SetEndpointType(t string) {
	e.Type = t
}

// GetEndpointUri returns endpoint uri in status
func (e *StatusEndpoint) GetEndpointUri() string {
	return e.URI
}

// SetEndpointUri sets endpoint uri in status
func (e *StatusEndpoint) SetEndpointUri(u string) {
	e.URI = u
}

// SetStatusEndpointFields sets endpoint information fields
func (e *StatusEndpoint) SetStatusEndpointFields(eScope common.StatusEndpointScope, eType string, eUri string) common.StatusEndpoint {
	e.Scope = convertFromCommonStatusEndpointScope(eScope)
	e.Type = eType
	e.URI = eUri
	return e
}

// RemoveEndpoint removes endpoint in status
func (s *OpenLibertyApplicationStatus) RemoveStatusEndpoint(endpointName string) {
	endpoints := s.Endpoints
	for i, ep := range endpoints {
		if ep.GetEndpointName() == endpointName {
			s.Endpoints = append(endpoints[:i], endpoints[i+1:]...)
			break
		}
	}
}

// NewStatusEndpoint returns new endpoint information
func (s *OpenLibertyApplicationStatus) NewStatusEndpoint(endpointName string) common.StatusEndpoint {
	e := &StatusEndpoint{}
	e.Name = endpointName
	return e
}

// GetStatusEndpoint returns endpoint information with endpoint name
func (s *OpenLibertyApplicationStatus) GetStatusEndpoint(endpointName string) common.StatusEndpoint {
	for i := range s.Endpoints {
		if s.Endpoints[i].GetEndpointName() == endpointName {
			return &s.Endpoints[i]
		}
	}
	return nil
}

// SetStatusEndpoint sets endpoint in status
func (s *OpenLibertyApplicationStatus) SetStatusEndpoint(c common.StatusEndpoint) {
	endpoint := &StatusEndpoint{}
	found := false
	for i := range s.Endpoints {
		if s.Endpoints[i].GetEndpointName() == c.GetEndpointName() {
			endpoint = &s.Endpoints[i]
			found = true
			break
		}
	}

	endpoint.SetEndpointName(c.GetEndpointName())
	endpoint.SetEndpointScope(c.GetEndpointScope())
	endpoint.SetEndpointType(c.GetEndpointType())
	endpoint.SetEndpointUri(c.GetEndpointUri())
	if !found {
		s.Endpoints = append(s.Endpoints, *endpoint)
	}
}

func convertToCommonStatusEndpointScope(c StatusEndpointScope) common.StatusEndpointScope {
	switch c {
	case StatusEndpointScopeExternal:
		return common.StatusEndpointScopeExternal
	case StatusEndpointScopeInternal:
		return common.StatusEndpointScopeInternal
	default:
		panic(c)
	}
}

func convertFromCommonStatusEndpointScope(c common.StatusEndpointScope) StatusEndpointScope {
	switch c {
	case common.StatusEndpointScopeExternal:
		return StatusEndpointScopeExternal
	case common.StatusEndpointScopeInternal:
		return StatusEndpointScopeInternal
	default:
		panic(c)
	}
}
