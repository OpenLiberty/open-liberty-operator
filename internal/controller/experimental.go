package controller

import olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"

func (r *ReconcileOpenLiberty) isConcurrencyEnabled(instance *olv1.OpenLibertyApplication) bool {
	if instance.GetExperimental() != nil && instance.GetExperimental().GetManageCache() != nil && *instance.GetExperimental().GetManageCache() {
		return true
	}
	return false
}

func (r *ReconcileOpenLiberty) getEphemeralPodWorkerPoolSize(instance *olv1.OpenLibertyApplication) int {
	if instance.GetExperimental() != nil && instance.GetExperimental().GetEphemeralPodWorkerPoolSize() != nil {
		return *instance.GetExperimental().GetEphemeralPodWorkerPoolSize()
	}
	return 3
}

func (r *ReconcileOpenLiberty) isCachingEnabled(instance *olv1.OpenLibertyApplication) bool {
	if instance.GetExperimental() != nil && instance.GetExperimental().GetManageCache() != nil && *instance.GetExperimental().GetManageCache() {
		return true
	}
	return false
}
