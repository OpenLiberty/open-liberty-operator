package v1beta1

import (
	"github.com/appsody/appsody-operator/pkg/common"
	prometheusv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// LibertyApplicationSpec defines the desired state of LibertyApplication
// +k8s:openapi-gen=true
type LibertyApplicationSpec struct {
	Version          string                         `json:"version,omitempty"`
	ApplicationImage string                         `json:"applicationImage"`
	Replicas         *int32                         `json:"replicas,omitempty"`
	Autoscaling      *LibertyApplicationAutoScaling `json:"autoscaling,omitempty"`
	PullPolicy       *corev1.PullPolicy             `json:"pullPolicy,omitempty"`
	PullSecret       *string                        `json:"pullSecret,omitempty"`

	// +listType=map
	// +listMapKey=name
	Volumes []corev1.Volume `json:"volumes,omitempty"`
	// +listType=map
	// +listMapKey=name
	VolumeMounts        []corev1.VolumeMount         `json:"volumeMounts,omitempty"`
	ResourceConstraints *corev1.ResourceRequirements `json:"resourceConstraints,omitempty"`
	ReadinessProbe      *corev1.Probe                `json:"readinessProbe,omitempty"`
	LivenessProbe       *corev1.Probe                `json:"livenessProbe,omitempty"`
	Service             LibertyApplicationService    `json:"service,omitempty"`
	Expose              *bool                        `json:"expose,omitempty"`
	// +listType=atomic
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`
	// +listType=map
	// +listMapKey=name
	Env                []corev1.EnvVar `json:"env,omitempty"`
	ServiceAccountName *string         `json:"serviceAccountName,omitempty"`
	// +listType=set
	Architecture         []string                      `json:"architecture,omitempty"`
	Storage              *LibertyApplicationStorage    `json:"storage,omitempty"`
	CreateKnativeService *bool                         `json:"createKnativeService,omitempty"`
	Monitoring           *LibertyApplicationMonitoring `json:"monitoring,omitempty"`
	CreateAppDefinition  *bool                         `json:"createAppDefinition,omitempty"`
	// +listType=map
	// +listMapKey=name
	InitContainers []corev1.Container `json:"initContainers,omitempty"`
}

// LibertyApplicationAutoScaling ...
// +k8s:openapi-gen=true
type LibertyApplicationAutoScaling struct {
	TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`
	MinReplicas                    *int32 `json:"minReplicas,omitempty"`

	// +kubebuilder:validation:Minimum=1
	MaxReplicas int32 `json:"maxReplicas,omitempty"`
}

// LibertyApplicationService ...
// +k8s:openapi-gen=true
type LibertyApplicationService struct {
	Type corev1.ServiceType `json:"type,omitempty"`

	// +kubebuilder:validation:Maximum=65536
	// +kubebuilder:validation:Minimum=1
	Port int32 `json:"port,omitempty"`

	Annotations map[string]string `json:"annotations,omitempty"`
	// +listType=atomic
	Consumes []ServiceBindingConsumes `json:"consumes,omitempty"`
	Provides *ServiceBindingProvides  `json:"provides,omitempty"`
}

// LibertyApplicationStorage ...
// +k8s:openapi-gen=true
type LibertyApplicationStorage struct {
	// +kubebuilder:validation:Pattern=^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$
	Size                string                        `json:"size,omitempty"`
	MountPath           string                        `json:"mountPath,omitempty"`
	VolumeClaimTemplate *corev1.PersistentVolumeClaim `json:"volumeClaimTemplate,omitempty"`
}

// LibertyApplicationMonitoring ...
type LibertyApplicationMonitoring struct {
	Labels map[string]string `json:"labels,omitempty"`
	// +listType=atomic
	Endpoints []prometheusv1.Endpoint `json:"endpoints,omitempty"`
}

// LibertyApplicationStatus defines the observed state of LibertyApplication
// +k8s:openapi-gen=true
type LibertyApplicationStatus struct {
	// +listType=map
	// +listMapKey=type
	Conditions       []StatusCondition       `json:"conditions,omitempty"`
	ConsumedServices common.ConsumedServices `json:"consumedServices,omitempty"`
}

// StatusCondition ...
// +k8s:openapi-gen=true
type StatusCondition struct {
	LastTransitionTime *metav1.Time               `json:"lastTransitionTime,omitempty"`
	LastUpdateTime     metav1.Time                `json:"lastUpdateTime,omitempty"`
	Reason             string                     `json:"reason,omitempty"`
	Message            string                     `json:"message,omitempty"`
	Status             corev1.ConditionStatus     `json:"status,omitempty"`
	Type               common.StatusConditionType `json:"type,omitempty"`
}

// ServiceBindingAuth allows a service to provide authentication information
type ServiceBindingAuth struct {
	// The secret that contains the username for authenticating
	Username corev1.SecretKeySelector `json:"username,omitempty"`
	// The secret that contains the password for authenticating
	Password corev1.SecretKeySelector `json:"password,omitempty"`
}

// ServiceBindingProvides represents information about
// +k8s:openapi-gen=true
type ServiceBindingProvides struct {
	Category common.ServiceBindingCategory `json:"category"`
	Context  string                        `json:"context,omitempty"`
	Protocol string                        `json:"protocol,omitempty"`
	Auth     *ServiceBindingAuth           `json:"auth,omitempty"`
}

// ServiceBindingConsumes represents a service to be consumed
// +k8s:openapi-gen=true
type ServiceBindingConsumes struct {
	Name      string                        `json:"name"`
	Namespace string                        `json:"namespace,omitempty"`
	Category  common.ServiceBindingCategory `json:"category"`
	MountPath string                        `json:"mountPath,omitempty"`
}

// StatusConditionType ...
type StatusConditionType string

const (
	// StatusConditionTypeReconciled ...
	StatusConditionTypeReconciled StatusConditionType = "Reconciled"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LibertyApplication is the Schema for the LibertyApplications API
// +k8s:openapi-gen=true
// +kubebuilder:resource:path=libertyapplications,scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Image",type="string",JSONPath=".spec.applicationImage",priority=0,description="Absolute name of the deployed image containing registry and tag"
// +kubebuilder:printcolumn:name="Exposed",type="boolean",JSONPath=".spec.expose",priority=0,description="Specifies whether deployment is exposed externally via default Route"
// +kubebuilder:printcolumn:name="Reconciled",type="string",JSONPath=".status.conditions[?(@.type=='Reconciled')].status",priority=0,description="Status of the reconcile condition"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type=='Reconciled')].reason",priority=1,description="Reason for the failure of reconcile condition"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type=='Reconciled')].message",priority=1,description="Failure message from reconcile condition"
// +kubebuilder:printcolumn:name="DependenciesSatisfied",type="string",JSONPath=".status.conditions[?(@.type=='DependenciesSatisfied')].status",priority=1,description="Status of the application dependencies"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",priority=0,description="Age of the resource"
type LibertyApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LibertyApplicationSpec   `json:"spec,omitempty"`
	Status LibertyApplicationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LibertyApplicationList contains a list of LibertyApplication
type LibertyApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LibertyApplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LibertyApplication{}, &LibertyApplicationList{})
}

// GetApplicationImage returns application image
func (cr *LibertyApplication) GetApplicationImage() string {
	return cr.Spec.ApplicationImage
}

// GetPullPolicy returns image pull policy
func (cr *LibertyApplication) GetPullPolicy() *corev1.PullPolicy {
	return cr.Spec.PullPolicy
}

// GetPullSecret returns secret name for docker registry credentials
func (cr *LibertyApplication) GetPullSecret() *string {
	return cr.Spec.PullSecret
}

// GetServiceAccountName returns service account name
func (cr *LibertyApplication) GetServiceAccountName() *string {
	return cr.Spec.ServiceAccountName
}

// GetReplicas returns number of replicas
func (cr *LibertyApplication) GetReplicas() *int32 {
	return cr.Spec.Replicas
}

// GetLivenessProbe returns liveness probe
func (cr *LibertyApplication) GetLivenessProbe() *corev1.Probe {
	return cr.Spec.LivenessProbe
}

// GetReadinessProbe returns readiness probe
func (cr *LibertyApplication) GetReadinessProbe() *corev1.Probe {
	return cr.Spec.ReadinessProbe
}

// GetVolumes returns volumes slice
func (cr *LibertyApplication) GetVolumes() []corev1.Volume {
	return cr.Spec.Volumes
}

// GetVolumeMounts returns volume mounts slice
func (cr *LibertyApplication) GetVolumeMounts() []corev1.VolumeMount {
	return cr.Spec.VolumeMounts
}

// GetResourceConstraints returns resource constraints
func (cr *LibertyApplication) GetResourceConstraints() *corev1.ResourceRequirements {
	return cr.Spec.ResourceConstraints
}

// GetExpose returns expose flag
func (cr *LibertyApplication) GetExpose() *bool {
	return cr.Spec.Expose
}

// GetEnv returns slice of environment variables
func (cr *LibertyApplication) GetEnv() []corev1.EnvVar {
	return cr.Spec.Env
}

// GetEnvFrom returns slice of environment variables from source
func (cr *LibertyApplication) GetEnvFrom() []corev1.EnvFromSource {
	return cr.Spec.EnvFrom
}

// GetCreateKnativeService returns flag that toggles Knative service
func (cr *LibertyApplication) GetCreateKnativeService() *bool {
	return cr.Spec.CreateKnativeService
}

// GetArchitecture returns slice of architectures
func (cr *LibertyApplication) GetArchitecture() []string {
	return cr.Spec.Architecture
}

// GetAutoscaling returns autoscaling settings
func (cr *LibertyApplication) GetAutoscaling() common.BaseApplicationAutoscaling {
	if cr.Spec.Autoscaling == nil {
		return nil
	}
	return cr.Spec.Autoscaling
}

// GetStorage returns storage settings
func (cr *LibertyApplication) GetStorage() common.BaseApplicationStorage {
	if cr.Spec.Storage == nil {
		return nil
	}
	return cr.Spec.Storage
}

// GetService returns service settings
func (cr *LibertyApplication) GetService() common.BaseApplicationService {
	return &cr.Spec.Service
}

// GetVersion returns application version
func (cr *LibertyApplication) GetVersion() string {
	return cr.Spec.Version
}

// GetAnnotations returns application annotation
func (cr *LibertyApplication) GetAnnotations() map[string]string {
	return cr.Annotations
}

// GetCreateAppDefinition returns a toggle for integration with kAppNav
func (cr *LibertyApplication) GetCreateAppDefinition() *bool {
	return cr.Spec.CreateAppDefinition
}

// GetMonitoring returns monitoring settings
func (cr *LibertyApplication) GetMonitoring() common.BaseApplicationMonitoring {
	if cr.Spec.Monitoring == nil {
		return nil
	}
	return cr.Spec.Monitoring
}

// GetStatus returns LibertyApplication status
func (cr *LibertyApplication) GetStatus() common.BaseApplicationStatus {
	return &cr.Status
}

// GetInitContainers returns list of init containers
func (cr *LibertyApplication) GetInitContainers() []corev1.Container {
	return cr.Spec.InitContainers
}

// GetGroupName returns group name to be used in labels and annotation
func (cr *LibertyApplication) GetGroupName() string {
	return "openliberty.io"
}

// GetConsumedServices returns a map of all the service names to be consumed by the application
func (s *LibertyApplicationStatus) GetConsumedServices() common.ConsumedServices {
	if s.ConsumedServices == nil {
		return nil
	}
	return s.ConsumedServices
}

// SetConsumedServices sets ConsumedServices
func (s *LibertyApplicationStatus) SetConsumedServices(c common.ConsumedServices) {
	s.ConsumedServices = c
}

// GetMinReplicas returns minimum replicas
func (a *LibertyApplicationAutoScaling) GetMinReplicas() *int32 {
	return a.MinReplicas
}

// GetMaxReplicas returns maximum replicas
func (a *LibertyApplicationAutoScaling) GetMaxReplicas() int32 {
	return a.MaxReplicas
}

// GetTargetCPUUtilizationPercentage returns target cpu usage
func (a *LibertyApplicationAutoScaling) GetTargetCPUUtilizationPercentage() *int32 {
	return a.TargetCPUUtilizationPercentage
}

// GetSize returns pesistent volume size
func (s *LibertyApplicationStorage) GetSize() string {
	return s.Size
}

// GetMountPath returns mount path for persistent volume
func (s *LibertyApplicationStorage) GetMountPath() string {
	return s.MountPath
}

// GetVolumeClaimTemplate returns a template representing requested persitent volume
func (s *LibertyApplicationStorage) GetVolumeClaimTemplate() *corev1.PersistentVolumeClaim {
	return s.VolumeClaimTemplate
}

// GetAnnotations returns a set of annotations to be added to the service
func (s *LibertyApplicationService) GetAnnotations() map[string]string {
	return s.Annotations
}

// GetPort returns service port
func (s *LibertyApplicationService) GetPort() int32 {
	if s != nil && s.Port != 0 {
		return s.Port
	}
	return 9080
}

// GetType returns service type
func (s *LibertyApplicationService) GetType() *corev1.ServiceType {
	return &s.Type
}

// GetProvides returns service provider configuration
func (s *LibertyApplicationService) GetProvides() common.ServiceBindingProvides {
	if s.Provides == nil {
		return nil
	}
	return s.Provides
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
func (s *LibertyApplicationService) GetConsumes() []common.ServiceBindingConsumes {
	consumes := make([]common.ServiceBindingConsumes, len(s.Consumes))
	for i := range s.Consumes {
		consumes[i] = &s.Consumes[i]
	}
	return consumes
}

// GetLabels returns labels to be added on ServiceMonitor
func (m *LibertyApplicationMonitoring) GetLabels() map[string]string {
	return m.Labels
}

// GetEndpoints returns endpoints to be added to ServiceMonitor
func (m *LibertyApplicationMonitoring) GetEndpoints() []prometheusv1.Endpoint {
	return m.Endpoints
}

// GetLabels returns set of labels to be added to all resources
func (cr *LibertyApplication) GetLabels() map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/name":       cr.Name,
		"app.kubernetes.io/managed-by": "open-liberty-operator",
	}

	if cr.Spec.Version != "" {
		labels["app.kubernetes.io/version"] = cr.Spec.Version
	}

	for key, value := range cr.Labels {
		if key != "app.kubernetes.io/instance" {
			labels[key] = value
		}
	}

	return labels
}

// GetType returns status condition type
func (c *StatusCondition) GetType() common.StatusConditionType {
	return c.Type
}

// SetType returns status condition type
func (c *StatusCondition) SetType(ct common.StatusConditionType) {
	c.Type = ct
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
func (s *LibertyApplicationStatus) NewCondition() common.StatusCondition {
	return &StatusCondition{}
}

// GetConditions returns slice of conditions
func (s *LibertyApplicationStatus) GetConditions() []common.StatusCondition {
	var conditions = []common.StatusCondition{}
	for i := range s.Conditions {
		conditions[i] = &s.Conditions[i]
	}
	return conditions
}

// GetCondition ...
func (s *LibertyApplicationStatus) GetCondition(t common.StatusConditionType) common.StatusCondition {

	for i := range s.Conditions {
		if s.Conditions[i].GetType() == t {
			return &s.Conditions[i]
		}
	}
	return nil
}

// SetCondition ...
func (s *LibertyApplicationStatus) SetCondition(c common.StatusCondition) {

	condition := &StatusCondition{}
	found := false
	for i := range s.Conditions {
		if s.Conditions[i].GetType() == c.GetType() {
			condition = &s.Conditions[i]
			found = true
		}
	}

	condition.SetLastTransitionTime(c.GetLastTransitionTime())
	condition.SetLastUpdateTime(c.GetLastUpdateTime())
	condition.SetReason(c.GetReason())
	condition.SetMessage(c.GetMessage())
	condition.SetStatus(c.GetStatus())
	condition.SetType(c.GetType())
	if !found {
		s.Conditions = append(s.Conditions, *condition)
	}
}

// Initialize sets default values
func (cr *LibertyApplication) Initialize() {
	if cr.Spec.Service.Port == 0 {
		cr.Spec.Service.Port = 9080
	}

	if cr.Spec.Service.Type == "" {
		cr.Spec.Service.Type = corev1.ServiceTypeClusterIP
	}

	pp := corev1.PullIfNotPresent

	if cr.Spec.PullPolicy == nil {
		cr.Spec.PullPolicy = &pp
	}
}
