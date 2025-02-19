package controller

import (
	"context"
	"fmt"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	tree "github.com/OpenLiberty/open-liberty-operator/utils/tree"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ReconcileOpenLibertyTrace) reconcileResourceTrackingState(instance *olv1.OpenLibertyTrace, leaderTrackerType string) (lutils.LeaderTrackerMetadataList, error) {
	treeMap, replaceMap, err := tree.ParseDecisionTree(leaderTrackerType, nil)
	if err != nil {
		return nil, err
	}

	latestOperandVersion, err := tree.GetLatestOperandVersion(treeMap, "")
	if err != nil {
		return nil, err
	}

	return tree.ReconcileResourceTrackingState(instance.GetNamespace(), OperatorShortName, leaderTrackerType, r.GetClient(),
		func() (lutils.LeaderTrackerMetadataList, error) {
			return r.OpenLibertyTraceSharedResourceGenerator(instance, treeMap, latestOperandVersion, leaderTrackerType)
		},
		func() ([]*unstructured.UnstructuredList, []string, error) {
			return r.OpenLibertyTraceLeaderTrackerGenerator(instance, treeMap, replaceMap, latestOperandVersion, leaderTrackerType, nil)
		},
		func(obj client.Object, owner metav1.Object, cb func() error) error {
			return r.CreateOrUpdate(obj, owner, cb)
		},
		func(obj client.Object) error {
			return r.DeleteResource(obj)
		},
		treeMap, replaceMap, latestOperandVersion)
}

func (r *ReconcileOpenLibertyTrace) OpenLibertyTraceSharedResourceGenerator(instance *olv1.OpenLibertyTrace, treeMap map[string]interface{}, latestOperandVersion, leaderTrackerType string) (lutils.LeaderTrackerMetadataList, error) {
	// return the metadata specific to the operator version, instance configuration, and shared resource being reconciled
	if leaderTrackerType == TRACE_RESOURCE_SHARING_FILE_NAME {
		traceMetadataList, err := r.reconcileTraceMetadata(instance, treeMap, latestOperandVersion, nil)
		if err != nil {
			return nil, err
		}
		return traceMetadataList, nil
	}
	return nil, fmt.Errorf("a leaderTrackerType was not provided when running reconcileResourceTrackingState")
}

func (r *ReconcileOpenLibertyTrace) OpenLibertyTraceLeaderTrackerGenerator(instance *olv1.OpenLibertyTrace, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, leaderTrackerType string, assetsFolder *string) ([]*unstructured.UnstructuredList, []string, error) {
	var resourcesMatrix []*unstructured.UnstructuredList
	var resourcesRootNameList []string
	if leaderTrackerType == TRACE_RESOURCE_SHARING_FILE_NAME {
		resourcesList, resourceRootName, traceErr := r.GetTraceResources(instance, treeMap, replaceMap, latestOperandVersion, assetsFolder, TRACE_RESOURCE_SHARING_FILE_NAME)
		if traceErr != nil {
			return nil, nil, traceErr
		}
		resourcesMatrix = append(resourcesMatrix, resourcesList)
		resourcesRootNameList = append(resourcesRootNameList, resourceRootName)
	} else {
		return nil, nil, fmt.Errorf("a valid leaderTrackerType was not specified for createNewLeaderTrackerList")
	}
	return resourcesMatrix, resourcesRootNameList, nil
}

// Search the cluster namespace for existing Trace CRs
func (r *ReconcileOpenLibertyTrace) GetTraceResources(instance *olv1.OpenLibertyTrace, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, assetsFolder *string, fileName string) (*unstructured.UnstructuredList, string, error) {
	traceResourceList, _, err := lutils.CreateUnstructuredResourceListFromSignature(fileName, assetsFolder, OperatorShortName)
	if err != nil {
		return nil, "", err
	}
	if err := r.GetClient().List(context.TODO(), traceResourceList, client.InNamespace(instance.GetNamespace())); err != nil {
		return nil, "", err
	}
	return traceResourceList, "", nil
}
