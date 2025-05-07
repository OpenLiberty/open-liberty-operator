package tree

import (
	"strconv"
	"strings"

	"github.com/OpenLiberty/open-liberty-operator/utils/leader"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Base interface for resource sharing
type ResourceSharingFactoryBase interface {
	Client() func() client.Client
	SetClient(fn func() client.Client)

	CreateOrUpdate() func(obj client.Object, owner metav1.Object, cb func() error) error
	SetCreateOrUpdate(fn func(obj client.Object, owner metav1.Object, cb func() error) error)

	DeleteResources() func(obj client.Object) error
	SetDeleteResources(fn func(obj client.Object) error)

	CleanupUnusedResources() func() bool
	SetCleanupUnusedResources(fn func() bool)

	LibertyURI() string
	SetLibertyURI(uri string)
}

// Interface for resource sharing
type ResourceSharingFactory interface {
	ResourceSharingFactoryBase
	Resources() func() (leader.LeaderTrackerMetadataList, error)
	SetResources(fn func() (leader.LeaderTrackerMetadataList, error))

	LeaderTrackers() func(assetsFolder *string) ([]*unstructured.UnstructuredList, []string, error)
	SetLeaderTrackers(fn func(assetsFolder *string) ([]*unstructured.UnstructuredList, []string, error))

	LeaderTrackerName() func(map[string]interface{}) (string, error)
	SetLeaderTrackerName(fn func(map[string]interface{}) (string, error))
}

// If shouldElectNewLeader is set to true, this instance will be set and returned as the resource leader
// Otherwise, returns the current shared resource leader
func ReconcileLeader(rsf ResourceSharingFactory, operatorName, operatorShortName, name, namespace string, leaderMetadata leader.LeaderTrackerMetadata, leaderTrackerType string, shouldElectNewLeader bool) (string, bool, string, error) {
	leaderTracker, leaderTrackers, err := leader.GetLeaderTracker(namespace, operatorName, operatorShortName, leaderTrackerType, rsf.Client()())
	if err != nil {
		return "", false, "", err
	}
	return ReconcileLeaderWithState(name, rsf, leaderTracker, leaderTrackers, leaderMetadata, shouldElectNewLeader)
}

func ReconcileLeaderWithState(name string, rsf ResourceSharingFactory, leaderTracker *corev1.Secret, leaderTrackers *[]leader.LeaderTracker, leaderMetadata leader.LeaderTrackerMetadata, shouldElectNewLeader bool) (string, bool, string, error) {
	initialLeaderIndex := -1
	for i, tracker := range *leaderTrackers {
		if tracker.Name == leaderMetadata.GetName() {
			initialLeaderIndex = i
		}
	}

	// if the tracked resource does not exist in resources labels, this instance is leader
	if initialLeaderIndex == -1 {
		if !shouldElectNewLeader {
			return "", false, "", nil
		}
		// clear instance.Name from ownership of any prior resources
		for i := range *leaderTrackers {
			(*leaderTrackers)[i].ClearOwnerIfMatchingAndSharesLastPathParent(name, leaderMetadata.GetPath())
		}
		// make instance.Name the new leader
		newLeader := leader.LeaderTracker{
			Name:      leaderMetadata.GetName(),
			Owner:     name,
			PathIndex: leaderMetadata.GetPathIndex(),
			Path:      leaderMetadata.GetPath(),
			// Sublease:  fmt.Sprint(time.Now().Unix()),
		}
		// append it to the list of leaders
		*leaderTrackers = append(*leaderTrackers, newLeader)
		// save the tracker state
		if err := SaveLeaderTracker(rsf, leaderTracker, leaderTrackers); err != nil {
			return "", false, "", err
		}
		return name, true, leaderMetadata.GetPathIndex(), nil
	}
	// otherwise, the resource is being tracked
	// if the leader of the tracked resource is non empty decide whether or not to return the resource owner
	candidateLeader := (*leaderTrackers)[initialLeaderIndex].Owner
	if len(candidateLeader) > 0 {
		// Return this other instance as the leader (the "other" instance could also be this instance)
		// Before returning, if the candidate instance is not this instance, this instance must clean up its old owner references to avoid an resource owner cycle.
		// A resource owner cycle can occur when instance A points to resource A and instance B points to resource B but then both instance A and B swap pointing to each other's resource.
		if candidateLeader != name {
			// clear instance.Name from ownership of any prior resources and evict the owner if the sublease has expired
			for i := range *leaderTrackers {
				(*leaderTrackers)[i].ClearOwnerIfMatchingAndSharesLastPathParent(name, leaderMetadata.GetPath())
				// (*leaderTrackers)[i].EvictOwnerIfSubleaseHasExpired()
			}
		}
		// else {
		// candidate is this instance, so renew the sublease
		// (*leaderTrackers)[initialLeaderIndex].RenewSublease()
		// }

		// If the current owner has been evicted, use this instance as the new owner
		currentOwner := (*leaderTrackers)[initialLeaderIndex].Owner
		if currentOwner == "" {
			currentOwner = name
			(*leaderTrackers)[initialLeaderIndex].SetOwner(currentOwner)
		}
		// save this new owner list
		if err := SaveLeaderTracker(rsf, leaderTracker, leaderTrackers); err != nil {
			return "", false, "", err
		}
		return currentOwner, currentOwner == name, (*leaderTrackers)[initialLeaderIndex].PathIndex, nil
	}
	if !shouldElectNewLeader {
		return "", false, "", nil
	}
	// there is either no leader (empty string) or the leader was deleted so now this instance is leader
	pathIndex := ""
	for i := range *leaderTrackers {
		if i == initialLeaderIndex {
			pathIndex = (*leaderTrackers)[i].PathIndex
			(*leaderTrackers)[i].SetOwner(name)
		} else {
			(*leaderTrackers)[i].ClearOwnerIfMatchingAndSharesLastPathParent(name, leaderMetadata.GetPath())
		}
	}

	// save this new owner list
	if err := SaveLeaderTracker(rsf, leaderTracker, leaderTrackers); err != nil {
		return "", false, "", err
	}
	return name, true, pathIndex, nil
}

func SaveLeaderTracker(rsf ResourceSharingFactoryBase, leaderTracker *corev1.Secret, trackerList *[]leader.LeaderTracker) error {
	// remove unused resources if applicable
	if rsf.CleanupUnusedResources()() {
		unusedIndices := []int{}
		for i := range *trackerList {
			if (*trackerList)[i].Owner == "" {
				unusedIndices = append(unusedIndices, i)
			}
		}
		for i := len(unusedIndices) - 1; i >= 0; i-- {
			ind := unusedIndices[i]
			trackerListTmp := make([]leader.LeaderTracker, 0)
			if ind == 0 {
				trackerListTmp = (*trackerList)[1:]
			} else if ind == len(*trackerList)-1 {
				trackerListTmp = (*trackerList)[:len(*trackerList)-1]
			} else {
				trackerListTmp = append((*trackerList)[:ind], (*trackerList)[ind+1:]...)
			}
			trackerList = &trackerListTmp
		}
	}
	return rsf.CreateOrUpdate()(leaderTracker, nil, func() error {
		leader.CustomizeLeaderTracker(leaderTracker, trackerList)
		return nil
	})
}

// Validates the resource decision tree YAML and generates the leader tracking state (Secret) for maintaining multiple shared resources
func ReconcileResourceTrackingState(namespace, operatorName, operatorShortName, leaderTrackerType string, rsf ResourceSharingFactory,
	treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string) (leader.LeaderTrackerMetadataList, error) {

	// persist or create a Secret to store the shared resources' state
	err := ReconcileLeaderTracker(namespace, operatorName, operatorShortName, rsf, treeMap, replaceMap, latestOperandVersion, leaderTrackerType, nil)
	if err != nil {
		return nil, err
	}
	return rsf.Resources()()
}

// Reconciles the latest LeaderTracker state to be used by the operator
func ReconcileLeaderTracker(namespace string, operatorName string, operatorShortName string, rsf ResourceSharingFactory, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, leaderTrackerType string, assetsFolder *string) error {
	leaderTracker, _, err := leader.GetLeaderTracker(namespace, operatorName, operatorShortName, leaderTrackerType, rsf.Client()())
	// If the Leader Tracker is missing, create from scratch
	if err != nil && kerrors.IsNotFound(err) {
		leaderTracker.Labels[leader.GetLeaderVersionLabel(rsf.LibertyURI())] = latestOperandVersion
		leaderTracker.ResourceVersion = ""
		leaderTrackers, err := CreateNewLeaderTrackerList(rsf, treeMap, replaceMap, latestOperandVersion, leaderTrackerType, assetsFolder)
		if err != nil {
			return err
		}
		return SaveLeaderTracker(rsf, leaderTracker, leaderTrackers)
	} else if err != nil {
		return err
	}
	// If the Leader Tracker is outdated, delete it so that it gets recreated in another reconcile
	if leaderTracker.Labels[leader.GetLeaderVersionLabel(rsf.LibertyURI())] != latestOperandVersion {
		if err := rsf.DeleteResources()(leaderTracker); err != nil {
			return err
		}
	}
	return nil
}

func CreateNewLeaderTrackerList(rsf ResourceSharingFactory, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, leaderTrackerType string, assetsFolder *string) (*[]leader.LeaderTracker, error) {
	resourcesMatrix, resourcesRootNameList, err := rsf.LeaderTrackers()(assetsFolder)
	if err != nil {
		return nil, err
	}
	leaderTracker := make([]leader.LeaderTracker, 0)
	for i, resourcesList := range resourcesMatrix {
		UpdateLeaderTrackersFromUnstructuredList(rsf, &leaderTracker, resourcesList, treeMap, replaceMap, latestOperandVersion, resourcesRootNameList[i])
	}
	return &leaderTracker, nil
}

// Removes the instance as leader if instance is the leader and if no leaders are being tracked then delete the leader tracking Secret
func RemoveLeader(name string, rsf ResourceSharingFactoryBase, leaderTracker *corev1.Secret, leaderTrackers *[]leader.LeaderTracker) error {
	changeDetected := false
	noOwners := true
	unusedIndices := []int{}
	cleanupUnusedResources := rsf.CleanupUnusedResources()()
	// If the instance is being tracked, remove it
	for i := range *leaderTrackers {
		if (*leaderTrackers)[i].ClearOwnerIfMatching(name) {
			changeDetected = true
		}
		if (*leaderTrackers)[i].Owner != "" {
			noOwners = false
		} else if cleanupUnusedResources {
			unusedIndices = append(unusedIndices, i)
		}
	}
	if noOwners {
		if err := rsf.DeleteResources()(leaderTracker); err != nil {
			return err
		}
	} else {
		if cleanupUnusedResources && len(unusedIndices) > 0 {
			changeDetected = true
		}
		if changeDetected {
			if err := SaveLeaderTracker(rsf, leaderTracker, leaderTrackers); err != nil {
				return err
			}
		}
	}
	return nil
}

func UpdateLeaderTrackersFromUnstructuredList(rsf ResourceSharingFactory, leaderTrackers *[]leader.LeaderTracker, resourceList *unstructured.UnstructuredList, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, resourceRootName string) error {
	for i, resource := range resourceList.Items {
		labelsMap, _, err := unstructured.NestedMap(resource.Object, "metadata", "labels")
		if err != nil {
			return err
		}
		if pathIndexInterface, found := labelsMap[leader.GetResourcePathIndexLabel(rsf.LibertyURI())]; found {
			pathIndex := pathIndexInterface.(string)
			// Skip this resource if path index does not contain a period separating delimeter
			if !strings.Contains(pathIndex, ".") {
				continue
			}
			labelVersionArray := strings.Split(pathIndex, ".")
			// Skip this resource if the path index is not a tuple representing the version and index
			if len(labelVersionArray) != 2 {
				continue
			}
			leaderTracker := leader.LeaderTracker{
				PathIndex: pathIndex,
			}
			indexIntVal, _ := strconv.ParseInt(labelVersionArray[1], 10, 64)
			path, pathErr := GetPathFromLeafIndex(treeMap, labelVersionArray[0], int(indexIntVal))
			// If path comes from a different operand version, the path needs to be upgraded/downgraded to the latestOperandVersion
			if pathErr == nil && labelVersionArray[0] != latestOperandVersion {
				// If user error has occurred, based on whether or not a dev deleted the decision tree structure of an older version
				// allow this condition to error (when err != nil) so that a deleted (older) revision of the decision tree that may be missing
				// won't halt the operator when the ReplacePath validation is performed
				if path, err := ReplacePath(path, latestOperandVersion, treeMap, replaceMap); err == nil {
					newPathIndex := strings.Split(path, ".")[0] + "." + strconv.FormatInt(int64(GetLeafIndex(treeMap, path)), 10)
					leaderTracker.PathIndex = newPathIndex
					leaderTracker.Path = path
					// the path may have changed so the path index reference needs to be updated directly in the resource
					if err := rsf.CreateOrUpdate()(&resourceList.Items[i], nil, func() error {
						labelsMap, _, err := unstructured.NestedMap(resourceList.Items[i].Object, "metadata", "labels")
						if err != nil {
							return err
						}
						labelsMap[leader.GetResourcePathIndexLabel(rsf.LibertyURI())] = newPathIndex
						if err := unstructured.SetNestedMap(resourceList.Items[i].Object, labelsMap, "metadata", "labels"); err != nil {
							return err
						}
						return nil
					}); err != nil {
						return err
					}
				}
			} else if pathErr == nil { // only update the path metadata if this operator's decision tree structure recognizes the resource
				leaderTracker.Path = path
			} else {
				// A valid decision tree path could not be found, so it will not be used by the operator and this resource will not be tracked
				continue
			}
			nameString, err := rsf.LeaderTrackerName()(resource.Object) // the leader.Name field value is determined by *_resource_sharing.go impl of the ResourceSharingFactory interface
			if err != nil {
				return err
			}
			leaderTracker.Name = nameString[len(resourceRootName):]
			leaderTracker.EvictOwner()
			leader.InsertIntoSortedLeaderTrackers(leaderTrackers, &leaderTracker)
		}
	}
	return nil
}

func RemoveLeaderTrackerReference(rsf ResourceSharingFactoryBase, name, namespace, operatorName, operatorShortName, resourceSharingFileName string) error {
	leaderTracker, leaderTrackers, err := leader.GetLeaderTracker(namespace, operatorName, operatorShortName, resourceSharingFileName, rsf.Client()())
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return RemoveLeader(name, rsf, leaderTracker, leaderTrackers)
}
