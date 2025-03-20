package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	"github.com/OpenLiberty/open-liberty-operator/utils/leader"
	tree "github.com/OpenLiberty/open-liberty-operator/utils/tree"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OpenLibertyTraceResourceSharingFactory struct {
	resourcesFunc              func() (leader.LeaderTrackerMetadataList, error)
	leaderTrackersFunc         func(assetsFolder *string) ([]*unstructured.UnstructuredList, []string, error)
	createOrUpdateFunc         func(obj client.Object, owner metav1.Object, cb func() error) error
	deleteResourcesFunc        func(obj client.Object) error
	leaderTrackerNameFunc      func(map[string]interface{}) (string, error)
	cleanupUnusedResourcesFunc func() bool
	clientFunc                 func() client.Client
}

func (rsf *OpenLibertyTraceResourceSharingFactory) Resources() func() (leader.LeaderTrackerMetadataList, error) {
	return rsf.resourcesFunc
}

func (rsf *OpenLibertyTraceResourceSharingFactory) SetResources(fn func() (leader.LeaderTrackerMetadataList, error)) {
	rsf.resourcesFunc = fn
}

func (rsf *OpenLibertyTraceResourceSharingFactory) LeaderTrackers() func(*string) ([]*unstructured.UnstructuredList, []string, error) {
	return rsf.leaderTrackersFunc
}

func (rsf *OpenLibertyTraceResourceSharingFactory) SetLeaderTrackers(fn func(*string) ([]*unstructured.UnstructuredList, []string, error)) {
	rsf.leaderTrackersFunc = fn
}

func (rsf *OpenLibertyTraceResourceSharingFactory) CreateOrUpdate() func(obj client.Object, owner metav1.Object, cb func() error) error {
	return rsf.createOrUpdateFunc
}

func (rsf *OpenLibertyTraceResourceSharingFactory) SetCreateOrUpdate(fn func(obj client.Object, owner metav1.Object, cb func() error) error) {
	rsf.createOrUpdateFunc = fn
}

func (rsf *OpenLibertyTraceResourceSharingFactory) DeleteResources() func(obj client.Object) error {
	return rsf.deleteResourcesFunc
}

func (rsf *OpenLibertyTraceResourceSharingFactory) SetDeleteResources(fn func(obj client.Object) error) {
	rsf.deleteResourcesFunc = fn
}

func (rsf *OpenLibertyTraceResourceSharingFactory) LeaderTrackerName() func(map[string]interface{}) (string, error) {
	return rsf.leaderTrackerNameFunc
}

func (rsf *OpenLibertyTraceResourceSharingFactory) SetLeaderTrackerName(fn func(map[string]interface{}) (string, error)) {
	rsf.leaderTrackerNameFunc = fn
}

func (rsf *OpenLibertyTraceResourceSharingFactory) CleanupUnusedResources() func() bool {
	return rsf.cleanupUnusedResourcesFunc
}

func (rsf *OpenLibertyTraceResourceSharingFactory) SetCleanupUnusedResources(fn func() bool) {
	rsf.cleanupUnusedResourcesFunc = fn
}

func (rsf *OpenLibertyTraceResourceSharingFactory) Client() func() client.Client {
	return rsf.clientFunc
}

func (rsf *OpenLibertyTraceResourceSharingFactory) SetClient(fn func() client.Client) {
	rsf.clientFunc = fn
}

func (r *ReconcileOpenLibertyTrace) createResourceSharingFactoryBase() tree.ResourceSharingFactoryBase {
	rsf := &OpenLibertyTraceResourceSharingFactory{}
	rsf.SetCreateOrUpdate(func(obj client.Object, owner metav1.Object, cb func() error) error {
		return r.CreateOrUpdate(obj, owner, cb)
	})
	rsf.SetDeleteResources(func(obj client.Object) error {
		return r.DeleteResource(obj)
	})
	rsf.SetCleanupUnusedResources(func() bool {
		return true
	})
	rsf.SetClient(func() client.Client {
		return r.GetClient()
	})
	return rsf
}

func (r *ReconcileOpenLibertyTrace) createResourceSharingFactory(instance *olv1.OpenLibertyTrace, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, leaderTrackerType string) tree.ResourceSharingFactory {
	var rsf *OpenLibertyTraceResourceSharingFactory
	rsfb := r.createResourceSharingFactoryBase()
	rsf = rsfb.(*OpenLibertyTraceResourceSharingFactory)
	rsf.SetLeaderTrackers(func(assetsFolder *string) ([]*unstructured.UnstructuredList, []string, error) {
		return r.OpenLibertyTraceLeaderTrackerGenerator(instance, treeMap, replaceMap, latestOperandVersion, leaderTrackerType, assetsFolder)
	})
	rsf.SetLeaderTrackerName(func(obj map[string]interface{}) (string, error) {
		nameString, _, err := unstructured.NestedString(obj, "spec", "podName") // the Trace CR will use .spec.podName as the leaderTracker key identifier
		return nameString, err
	})
	rsf.SetResources(func() (leader.LeaderTrackerMetadataList, error) {
		return r.OpenLibertyTraceSharedResourceGenerator(instance, treeMap, latestOperandVersion, leaderTrackerType)
	})
	return rsf
}

func (r *ReconcileOpenLibertyTrace) reconcileResourceTrackingState(instance *olv1.OpenLibertyTrace, leaderTrackerType string) (tree.ResourceSharingFactory, leader.LeaderTrackerMetadataList, error) {
	treeMap, replaceMap, err := tree.ParseDecisionTree(leaderTrackerType, nil)
	if err != nil {
		return nil, nil, err
	}

	latestOperandVersion, err := tree.GetLatestOperandVersion(treeMap, "")
	if err != nil {
		return nil, nil, err
	}

	rsf := r.createResourceSharingFactory(instance, treeMap, replaceMap, latestOperandVersion, leaderTrackerType)
	trackerMetadataList, err := tree.ReconcileResourceTrackingState(instance.GetNamespace(), OperatorName, OperatorShortName, leaderTrackerType, rsf, treeMap, replaceMap, latestOperandVersion)
	return rsf, trackerMetadataList, err
}

func (r *ReconcileOpenLibertyTrace) OpenLibertyTraceSharedResourceGenerator(instance *olv1.OpenLibertyTrace, treeMap map[string]interface{}, latestOperandVersion, leaderTrackerType string) (leader.LeaderTrackerMetadataList, error) {
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

// Search the instance's namespace for existing Trace CRs
func (r *ReconcileOpenLibertyTrace) GetTraceResources(instance *olv1.OpenLibertyTrace, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, assetsFolder *string, fileName string) (*unstructured.UnstructuredList, string, error) {
	traceResourceList, _, err := leader.CreateUnstructuredResourceListFromSignature(fileName, assetsFolder)
	if err != nil {
		return nil, "", err
	}
	if err := r.GetClient().List(context.TODO(), traceResourceList, client.InNamespace(instance.GetNamespace())); err != nil {
		return nil, "", err
	}

	// If the Trace CR is not annotated with a resource tracking label, patch the CR instance with a leader tracking label to work on the current resource tracking impl.
	for i := range len(traceResourceList.Items) {
		labelsMap, _, _ := unstructured.NestedMap(traceResourceList.Items[i].Object, "metadata", "labels")
		if labelsMap != nil {
			if _, found := labelsMap[leader.GetResourcePathIndexLabel(lutils.LibertyURI)]; found {
				continue // skip if resource tracking label exists
			}
		}
		// otherwise, create the resource tracking label
		defaultUpdatedPathIndex := ""
		// path is hardcoded to start replaceMap translation at "v1_4_2.name.*"
		if path, err := tree.ReplacePath("v1_4_2.name.*", latestOperandVersion, treeMap, replaceMap); err == nil {
			defaultUpdatedPathIndex = strings.Split(path, ".")[0] + "." + strconv.FormatInt(int64(tree.GetLeafIndex(treeMap, path)), 10)
		}
		if defaultUpdatedPathIndex != "" {
			if err := r.CreateOrUpdate(&traceResourceList.Items[i], nil, func() error {
				// add the GetResourcePathIndexLabel(lutils.LibertyURI)
				labelsMap, _, _ := unstructured.NestedMap(traceResourceList.Items[i].Object, "metadata", "labels")
				if labelsMap == nil {
					labelsMap = make(map[string]interface{})
				}
				labelsMap[leader.GetResourcePathIndexLabel(lutils.LibertyURI)] = defaultUpdatedPathIndex
				if err := unstructured.SetNestedMap(traceResourceList.Items[i].Object, labelsMap, "metadata", "labels"); err != nil {
					return err
				}
				return nil
			}); err != nil {
				return traceResourceList, "", err
			}
		}
	}
	return traceResourceList, "", nil
}
