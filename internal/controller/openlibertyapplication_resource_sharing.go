package controller

import (
	"fmt"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	tree "github.com/OpenLiberty/open-liberty-operator/utils/tree"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OpenLibertyApplicationResourceSharingFactory struct {
	resourcesFunc              func() (lutils.LeaderTrackerMetadataList, error)
	leaderTrackersFunc         func(assetsFolder *string) ([]*unstructured.UnstructuredList, []string, error)
	createOrUpdateFunc         func(obj client.Object, owner metav1.Object, cb func() error) error
	deleteResourcesFunc        func(obj client.Object) error
	leaderTrackerNameFunc      func(map[string]interface{}) (string, error)
	cleanupUnusedResourcesFunc func() bool
	clientFunc                 func() client.Client
}

func (rsf *OpenLibertyApplicationResourceSharingFactory) Resources() func() (lutils.LeaderTrackerMetadataList, error) {
	return rsf.resourcesFunc
}

func (rsf *OpenLibertyApplicationResourceSharingFactory) LeaderTrackers() func(*string) ([]*unstructured.UnstructuredList, []string, error) {
	return rsf.leaderTrackersFunc
}

func (rsf *OpenLibertyApplicationResourceSharingFactory) CreateOrUpdate() func(obj client.Object, owner metav1.Object, cb func() error) error {
	return rsf.createOrUpdateFunc
}

func (rsf *OpenLibertyApplicationResourceSharingFactory) DeleteResources() func(obj client.Object) error {
	return rsf.deleteResourcesFunc
}

func (rsf *OpenLibertyApplicationResourceSharingFactory) LeaderTrackerName() func(map[string]interface{}) (string, error) {
	return rsf.leaderTrackerNameFunc
}

func (rsf *OpenLibertyApplicationResourceSharingFactory) CleanupUnusedResources() func() bool {
	return rsf.cleanupUnusedResourcesFunc
}

func (rsf *OpenLibertyApplicationResourceSharingFactory) SetCleanupUnusedResources(fn func() bool) {
	rsf.cleanupUnusedResourcesFunc = fn
}

func (rsf *OpenLibertyApplicationResourceSharingFactory) Client() func() client.Client {
	return rsf.clientFunc
}

func (r *ReconcileOpenLiberty) createResourceSharingFactoryBase() tree.ResourceSharingFactoryBase {
	return &OpenLibertyApplicationResourceSharingFactory{
		createOrUpdateFunc: func(obj client.Object, owner metav1.Object, cb func() error) error {
			return r.CreateOrUpdate(obj, owner, cb)
		},
		deleteResourcesFunc: func(obj client.Object) error {
			return r.DeleteResource(obj)
		},
		cleanupUnusedResourcesFunc: func() bool {
			return false
		},
		clientFunc: func() client.Client {
			return r.GetClient()
		},
	}
}

func (r *ReconcileOpenLiberty) createResourceSharingFactory(instance *olv1.OpenLibertyApplication, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, leaderTrackerType string) tree.ResourceSharingFactory {
	return &OpenLibertyApplicationResourceSharingFactory{
		resourcesFunc: func() (lutils.LeaderTrackerMetadataList, error) {
			return r.OpenLibertyApplicationSharedResourceGenerator(instance, treeMap, latestOperandVersion, leaderTrackerType)
		},
		leaderTrackersFunc: func(assetsFolder *string) ([]*unstructured.UnstructuredList, []string, error) {
			return r.OpenLibertyApplicationLeaderTrackerGenerator(instance, treeMap, replaceMap, latestOperandVersion, leaderTrackerType, assetsFolder)
		},
		createOrUpdateFunc: func(obj client.Object, owner metav1.Object, cb func() error) error {
			return r.CreateOrUpdate(obj, owner, cb)
		},
		deleteResourcesFunc: func(obj client.Object) error {
			return r.DeleteResource(obj)
		},
		leaderTrackerNameFunc: func(obj map[string]interface{}) (string, error) {
			nameString, _, err := unstructured.NestedString(obj, "metadata", "name") // the LTPA and Password Encryption Secret will both use their .metadata.name as the leaderTracker key identifier
			return nameString, err
		},
		cleanupUnusedResourcesFunc: func() bool {
			return false
		},
		clientFunc: func() client.Client {
			return r.GetClient()
		},
	}
}

func (r *ReconcileOpenLiberty) reconcileResourceTrackingState(instance *olv1.OpenLibertyApplication, leaderTrackerType string) (tree.ResourceSharingFactory, lutils.LeaderTrackerMetadataList, error) {
	treeMap, replaceMap, err := tree.ParseDecisionTree(leaderTrackerType, nil)
	if err != nil {
		return nil, nil, err
	}
	latestOperandVersion, err := tree.GetLatestOperandVersion(treeMap, "")
	if err != nil {
		return nil, nil, err
	}
	rsf := r.createResourceSharingFactory(instance, treeMap, replaceMap, latestOperandVersion, leaderTrackerType)
	trackerMetadataList, err := tree.ReconcileResourceTrackingState(instance.GetNamespace(), OperatorShortName, leaderTrackerType, rsf, treeMap, replaceMap, latestOperandVersion)
	return rsf, trackerMetadataList, err
}

func (r *ReconcileOpenLiberty) OpenLibertyApplicationSharedResourceGenerator(instance *olv1.OpenLibertyApplication, treeMap map[string]interface{}, latestOperandVersion, leaderTrackerType string) (lutils.LeaderTrackerMetadataList, error) {
	// return the metadata specific to the operator version, instance configuration, and shared resource being reconciled
	if leaderTrackerType == LTPA_RESOURCE_SHARING_FILE_NAME {
		ltpaMetadataList, err := r.reconcileLTPAMetadata(instance, treeMap, latestOperandVersion, nil)
		if err != nil {
			return nil, err
		}
		return ltpaMetadataList, nil
	}
	if leaderTrackerType == PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME {
		passwordEncryptionMetadataList, err := r.reconcilePasswordEncryptionMetadata(treeMap, latestOperandVersion)
		if err != nil {
			return nil, err
		}
		return passwordEncryptionMetadataList, nil
	}
	return nil, fmt.Errorf("a leaderTrackerType was not provided when running reconcileResourceTrackingState")
}

func (r *ReconcileOpenLiberty) OpenLibertyApplicationLeaderTrackerGenerator(instance *olv1.OpenLibertyApplication, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, leaderTrackerType string, assetsFolder *string) ([]*unstructured.UnstructuredList, []string, error) {
	var resourcesMatrix []*unstructured.UnstructuredList
	var resourcesRootNameList []string
	if leaderTrackerType == LTPA_RESOURCE_SHARING_FILE_NAME {
		// 1. Add LTPA key Secret
		resourcesList, resourceRootName, keyErr := r.GetLTPAKeyResources(instance, treeMap, replaceMap, latestOperandVersion, assetsFolder)
		if keyErr != nil {
			return nil, nil, keyErr
		}
		resourcesMatrix = append(resourcesMatrix, resourcesList)
		resourcesRootNameList = append(resourcesRootNameList, resourceRootName)
		// 2. Add LTPA password Secret (config 1)
		resourcesList, resourceRootName, keyErr = r.GetLTPAConfigResources(instance, treeMap, replaceMap, latestOperandVersion, assetsFolder, LTPA_CONFIG_1_RESOURCE_SHARING_FILE_NAME)
		if keyErr != nil {
			return nil, nil, keyErr
		}
		resourcesMatrix = append(resourcesMatrix, resourcesList)
		resourcesRootNameList = append(resourcesRootNameList, resourceRootName)
		// 3. Add LTPA password Secret (config 2)
		resourcesList, resourceRootName, keyErr = r.GetLTPAConfigResources(instance, treeMap, replaceMap, latestOperandVersion, assetsFolder, LTPA_CONFIG_2_RESOURCE_SHARING_FILE_NAME)
		if keyErr != nil {
			return nil, nil, keyErr
		}
		resourcesMatrix = append(resourcesMatrix, resourcesList)
		resourcesRootNameList = append(resourcesRootNameList, resourceRootName)
	} else if leaderTrackerType == PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME {
		resourcesList, resourceRootName, passwordErr := r.GetPasswordEncryptionResources(instance, treeMap, replaceMap, latestOperandVersion, assetsFolder)
		if passwordErr != nil {
			return nil, nil, passwordErr
		}
		resourcesMatrix = append(resourcesMatrix, resourcesList)
		resourcesRootNameList = append(resourcesRootNameList, resourceRootName)
	} else {
		return nil, nil, fmt.Errorf("a valid leaderTrackerType was not specified for createNewLeaderTrackerList")
	}
	return resourcesMatrix, resourcesRootNameList, nil
}

func hasLTPAKeyResourceSuffixesEnv(instance *olv1.OpenLibertyApplication) (string, bool) {
	return hasResourceSuffixesEnv(instance, "LTPA_KEY_RESOURCE_SUFFIXES")
}

func hasLTPAConfigResourceSuffixesEnv(instance *olv1.OpenLibertyApplication) (string, bool) {
	return hasResourceSuffixesEnv(instance, "LTPA_CONFIG_RESOURCE_SUFFIXES")
}

func hasResourceSuffixesEnv(instance *olv1.OpenLibertyApplication, envName string) (string, bool) {
	for _, env := range instance.GetEnv() {
		if env.Name == envName {
			return env.Value, true
		}
	}
	return "", false
}
