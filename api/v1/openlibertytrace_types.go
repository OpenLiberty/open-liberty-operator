package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Defines the desired state of OpenLibertyTrace
type OpenLibertyTraceSpec struct {
	// The name of the Pod, which must be in the same namespace as the OpenLibertyTrace CR.
	PodName string `json:"podName"`

	// The trace string to be used to selectively enable trace. The default is *=info.
	TraceSpecification string `json:"traceSpecification"`

	// The maximum size (in MB) that a log file can reach before it is rolled. To disable this attribute, set the value to 0.
	MaxFileSize *int32 `json:"maxFileSize,omitempty"`

	// If an enforced maximum file size exists, this setting is used to determine how many of each of the logs files are kept.
	MaxFiles *int32 `json:"maxFiles,omitempty"`

	// Set to true to stop tracing.
	Disable *bool `json:"disable,omitempty"`
}

// Defines the observed state of OpenLibertyTrace operation
type OpenLibertyTraceStatus struct {
	// +listType=atomic
	Conditions       []OperationStatusCondition `json:"conditions,omitempty"`
	OperatedResource OperatedResource           `json:"operatedResource,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:path=openlibertytraces,scope=Namespaced,shortName=oltrace;oltraces
// +kubebuilder:printcolumn:name="PodName",type="string",JSONPath=".status.operatedResource.resourceName",priority=0,description="Name of the last operated pod"
// +kubebuilder:printcolumn:name="Tracing",type="string",JSONPath=".status.conditions[?(@.type=='Enabled')].status",priority=0,description="Status of the trace condition"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type=='Enabled')].reason",priority=1,description="Reason for the failure of trace condition"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type=='Enabled')].message",priority=1,description="Failure message from trace condition"
// +operator-sdk:csv:customresourcedefinitions:displayName="OpenLibertyTrace"
// Day-2 operation for gathering server traces
type OpenLibertyTrace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenLibertyTraceSpec   `json:"spec,omitempty"`
	Status OpenLibertyTraceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// OpenLibertyTraceList contains a list of OpenLibertyTrace
type OpenLibertyTraceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenLibertyTrace `json:"items"`
}

// GetType returns status condition type
func (c *OperationStatusCondition) GetType() OperationStatusConditionType {
	return c.Type
}

// SetType sets status condition type
func (c *OperationStatusCondition) SetType(ct OperationStatusConditionType) {
	c.Type = ct
}

// GetLastTransitionTime return time of last status change
func (c *OperationStatusCondition) GetLastTransitionTime() *metav1.Time {
	return c.LastTransitionTime
}

// SetLastTransitionTime sets time of last status change
func (c *OperationStatusCondition) SetLastTransitionTime(t *metav1.Time) {
	c.LastTransitionTime = t
}

// GetLastUpdateTime return time of last status update
func (c *OperationStatusCondition) GetLastUpdateTime() metav1.Time {
	return c.LastUpdateTime
}

// SetLastUpdateTime sets time of last status update
func (c *OperationStatusCondition) SetLastUpdateTime(t metav1.Time) {
	c.LastUpdateTime = t
}

// GetMessage return condition's message
func (c *OperationStatusCondition) GetMessage() string {
	return c.Message
}

// SetMessage sets condition's message
func (c *OperationStatusCondition) SetMessage(m string) {
	c.Message = m
}

// GetReason return condition's message
func (c *OperationStatusCondition) GetReason() string {
	return c.Reason
}

// SetReason sets condition's reason
func (c *OperationStatusCondition) SetReason(r string) {
	c.Reason = r
}

// GetStatus return condition's status
func (cr *OpenLibertyTrace) GetStatus() *OpenLibertyTraceStatus {
	return &cr.Status
}

// GetStatus return condition's status
func (c *OperationStatusCondition) GetStatus() corev1.ConditionStatus {
	return c.Status
}

// SetStatus sets condition's status
func (c *OperationStatusCondition) SetStatus(s corev1.ConditionStatus) {
	c.Status = s
}

// NewCondition returns new condition
func (s *OpenLibertyTraceStatus) NewCondition() OperationStatusCondition {
	return OperationStatusCondition{}
}

// GetConditions returns slice of conditions
func (s *OpenLibertyTraceStatus) GetConditions() []OperationStatusCondition {
	var conditions = []OperationStatusCondition{}
	for i := range s.Conditions {
		conditions[i] = s.Conditions[i]
	}
	return conditions
}

// GetCondition ...
func (s *OpenLibertyTraceStatus) GetCondition(t OperationStatusConditionType) OperationStatusCondition {

	for i := range s.Conditions {
		if s.Conditions[i].GetType() == t {
			return s.Conditions[i]
		}
	}
	return OperationStatusCondition{LastUpdateTime: metav1.Time{}} //revisit
}

// SetCondition ...
func (s *OpenLibertyTraceStatus) SetCondition(c OperationStatusCondition) {

	condition := &OperationStatusCondition{}
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

// GetOperatedResource ...
func (s *OpenLibertyTraceStatus) GetOperatedResource() *OperatedResource {
	return &s.OperatedResource
}

// SetOperatedResource ...
func (s *OpenLibertyTraceStatus) SetOperatedResource(or OperatedResource) {
	s.OperatedResource = or
}

func (cr *OpenLibertyTrace) Initialize() {
	if cr.Spec.Disable == nil {
		disable := false
		cr.Spec.Disable = &disable
	}
}

func init() {
	SchemeBuilder.Register(&OpenLibertyTrace{}, &OpenLibertyTraceList{})
}
