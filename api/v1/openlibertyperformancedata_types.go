package v1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OpenLibertyPerformanceDataSpec defines the desired state of OpenLibertyPerformanceData
type OpenLibertyPerformanceDataSpec struct {
	// The name of the Pod, which must be in the same namespace as the OpenLibertyPerformanceData CR.
	PodName string `json:"podName"`

	// The total time, in seconds, for gathering performance data. The minimum value is 10 seconds. The maximum value is 600 seconds (10 minutes). Defaults to 240 seconds (4 minutes).
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=600
	Timespan *int `json:"timespan,omitempty"`

	// The time, in seconds, between executions. The minimum value is 1 second. Defaults to 30 seconds.
	// +kubebuilder:validation:Minimum=1
	Interval *int `json:"interval,omitempty"`
}

// Defines the observed state of OpenLibertyPerformanceData
type OpenLibertyPerformanceDataStatus struct {
	// +listType=atomic
	Conditions []OperationStatusCondition    `json:"conditions,omitempty"`
	Versions   PerformanceDataStatusVersions `json:"versions,omitempty"`
	// Location of the generated performance data file
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Performance Data File Path",xDescriptors="urn:alm:descriptor:com.tectonic.ui:text"
	PerformanceDataFile string `json:"performanceDataFile,omitempty"`
	// The generation identifier of this OpenLibertyPerformanceData instance completely reconciled by the Operator.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

type PerformanceDataStatusVersions struct {
	Reconciled string `json:"reconciled,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:path=openlibertyperformancedata,scope=Namespaced,shortName=olperfdata
// +kubebuilder:printcolumn:name="Started",type="string",JSONPath=".status.conditions[?(@.type=='Started')].status",priority=0,description="Indicates if performance data operation has started"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type=='Started')].reason",priority=1,description="Reason for performance data operation failing to start"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type=='Started')].message",priority=1,description="Message for performance data operation failing to start"
// +kubebuilder:printcolumn:name="Completed",type="string",JSONPath=".status.conditions[?(@.type=='Completed')].status",priority=0,description="Indicates if performance data operation has completed"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type=='Completed')].reason",priority=1,description="Reason for performance data operation failing to complete"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type=='Completed')].message",priority=1,description="Message for performance data operation failing to complete"
// +kubebuilder:printcolumn:name="Performance Data file",type="string",JSONPath=".status.performanceDataFile",priority=0,description="Indicates filename of the server performance data"
// +operator-sdk:csv:customresourcedefinitions:displayName="OpenLibertyPerformanceData"
// Day-2 operation for generating server performance data
type OpenLibertyPerformanceData struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenLibertyPerformanceDataSpec   `json:"spec,omitempty"`
	Status OpenLibertyPerformanceDataStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// OpenLibertyPerformanceDataList contains a list of OpenLibertyPerformanceData
type OpenLibertyPerformanceDataList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenLibertyPerformanceData `json:"items"`
}

func getIntValueOrDefault(value *int, defaultValue int) int {
	if value == nil {
		return defaultValue
	}
	return *value
}

// GetTimespan returns the timespan in seconds for running a performance data operation. Defaults to 240.
func (cr *OpenLibertyPerformanceData) GetTimespan() int {
	defaultTimespan := 240
	return getIntValueOrDefault(cr.Spec.Timespan, defaultTimespan)
}

// GetInterval returns the time interval in seconds between performance data operations. Defaults to 30.
func (cr *OpenLibertyPerformanceData) GetInterval() int {
	defaultInterval := 30
	return getIntValueOrDefault(cr.Spec.Interval, defaultInterval)
}

// GetStatus return condition's status
func (cr *OpenLibertyPerformanceData) GetStatus() *OpenLibertyPerformanceDataStatus {
	return &cr.Status
}

// NewCondition returns new condition
func (s *OpenLibertyPerformanceDataStatus) NewCondition() OperationStatusCondition {
	return OperationStatusCondition{}
}

// GetConditions returns slice of conditions
func (s *OpenLibertyPerformanceDataStatus) GetConditions() []OperationStatusCondition {
	var conditions = []OperationStatusCondition{}
	for i := range s.Conditions {
		conditions[i] = s.Conditions[i]
	}
	return conditions
}

// GetCondition ...
func (s *OpenLibertyPerformanceDataStatus) GetCondition(t OperationStatusConditionType) OperationStatusCondition {

	for i := range s.Conditions {
		if s.Conditions[i].GetType() == t {
			return s.Conditions[i]
		}
	}
	return OperationStatusCondition{LastUpdateTime: metav1.Time{}} //revisit
}

// SetCondition ...
func (s *OpenLibertyPerformanceDataStatus) SetCondition(c OperationStatusCondition) {
	condition := &OperationStatusCondition{}
	found := false
	for i := range s.Conditions {
		if s.Conditions[i].GetType() == c.GetType() {
			condition = &s.Conditions[i]
			found = true
			break
		}
	}

	if condition.GetStatus() != c.GetStatus() || condition.GetMessage() != c.GetMessage() || condition.GetReason() != c.GetReason() {
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

func init() {
	SchemeBuilder.Register(&OpenLibertyPerformanceData{}, &OpenLibertyPerformanceDataList{})
}
