package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OperationStatusCondition ...
// +k8s:openapi-gen=true
type OperationStatusCondition struct {
	LastTransitionTime *metav1.Time                 `json:"lastTransitionTime,omitempty"`
	LastUpdateTime     metav1.Time                  `json:"lastUpdateTime,omitempty"`
	Reason             string                       `json:"reason,omitempty"`
	Message            string                       `json:"message,omitempty"`
	Status             corev1.ConditionStatus       `json:"status,omitempty"`
	Type               OperationStatusConditionType `json:"type,omitempty"`
}

// OperatedResource ...
// +k8s:openapi-gen=true
type OperatedResource struct {
	ResourceType string `json:"resourceType,omitempty"`
	ResourceName string `json:"resourceName,omitempty"`
}

// GetOperatedResourceName get the last operated resource name
func (or *OperatedResource) GetOperatedResourceName() string {
	return or.ResourceName
}

// SetOperatedResourceName sets the last operated resource name
func (or *OperatedResource) SetOperatedResourceName(n string) {
	or.ResourceName = n
}

// GetOperatedResourceType get the last operated resource type
func (or *OperatedResource) GetOperatedResourceType() string {
	return or.ResourceType
}

// SetOperatedResourceType sets the last operated resource type
func (or *OperatedResource) SetOperatedResourceType(t string) {
	or.ResourceType = t
}
