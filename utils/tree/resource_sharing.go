package tree

import (
	"strconv"
	"strings"

	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ResourceSharingFactory interface {
	Resources() func() (lutils.LeaderTrackerMetadataList, error)
	LeaderTrackers() func() ([]*unstructured.UnstructuredList, []string, error)
	CreateOrUpdate() func(obj client.Object, owner metav1.Object, cb func() error) error
	DeleteResources() func(obj client.Object) error
	LeaderTrackerName() func(map[string]interface{}) (string, error)
}

// If shouldElectNewLeader is set to true, the OpenLibertyApplication instance will be set and returned as the resource leader
// Otherwise, returns the current shared resource leader
func ReconcileLeader(client client.Client, createOrUpdateCallback func(client.Object, metav1.Object, func() error) error, operatorShortName, name, namespace string, leaderMetadata lutils.LeaderTrackerMetadata, leaderTrackerType string, shouldElectNewLeader bool, removeDanglingResources bool) (string, bool, string, error) {
	leaderTracker, leaderTrackers, err := lutils.GetLeaderTracker(namespace, operatorShortName, leaderTrackerType, client)
	if err != nil {
		return "", false, "", err
	}
	return ReconcileLeaderWithState(name, createOrUpdateCallback, leaderTracker, leaderTrackers, leaderMetadata, shouldElectNewLeader, removeDanglingResources)
}

func ReconcileLeaderWithState(name string, createOrUpdateCallback func(client.Object, metav1.Object, func() error) error, leaderTracker *corev1.Secret, leaderTrackers *[]lutils.LeaderTracker, leaderMetadata lutils.LeaderTrackerMetadata, shouldElectNewLeader bool, removeDanglingResources bool) (string, bool, string, error) {
	initialLeaderIndex := -1
	for i, tracker := range *leaderTrackers {
		if tracker.Name == leaderMetadata.GetName() {
			initialLeaderIndex = i
		}
	}
	if removeDanglingResources {
		defer func() {
			removeList := []int{}
			n := len(*leaderTrackers)
			for i := range *leaderTrackers {
				j := n - i - 1
				if (*leaderTrackers)[j].Owner == "" {
					removeList = append(removeList, j)
				}
			}
			for _, ri := range removeList {
				*leaderTrackers = append((*leaderTrackers)[:ri], (*leaderTrackers)[ri+1:]...)
			}
			// save the tracker state
			SaveLeaderTracker(createOrUpdateCallback, leaderTracker, leaderTrackers)
		}()
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
		newLeader := lutils.LeaderTracker{
			Name:      leaderMetadata.GetName(),
			Owner:     name,
			PathIndex: leaderMetadata.GetPathIndex(),
			Path:      leaderMetadata.GetPath(),
			// Sublease:  fmt.Sprint(time.Now().Unix()),
		}
		// append it to the list of leaders
		*leaderTrackers = append(*leaderTrackers, newLeader)
		// save the tracker state
		if err := SaveLeaderTracker(createOrUpdateCallback, leaderTracker, leaderTrackers); err != nil {
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
		if err := SaveLeaderTracker(createOrUpdateCallback, leaderTracker, leaderTrackers); err != nil {
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
	if err := SaveLeaderTracker(createOrUpdateCallback, leaderTracker, leaderTrackers); err != nil {
		return "", false, "", err
	}
	return name, true, pathIndex, nil
}

func SaveLeaderTracker(createOrUpdateCallback func(client.Object, metav1.Object, func() error) error, leaderTracker *corev1.Secret, trackerList *[]lutils.LeaderTracker) error {
	return createOrUpdateCallback(leaderTracker, nil, func() error {
		lutils.CustomizeLeaderTracker(leaderTracker, trackerList)
		return nil
	})
}

// Validates the resource decision tree YAML and generates the leader tracking state (Secret) for maintaining multiple shared resources
func ReconcileResourceTrackingState(namespace, operatorShortName, leaderTrackerType string,
	client client.Client, rsf ResourceSharingFactory,
	treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string) (lutils.LeaderTrackerMetadataList, error) {

	// persist or create a Secret to store the shared resources' state
	err := ReconcileLeaderTracker(namespace, operatorShortName, client, rsf, treeMap, replaceMap, latestOperandVersion, leaderTrackerType, nil)
	if err != nil {
		return nil, err
	}
	return rsf.Resources()()
}

// Reconciles the latest LeaderTracker state to be used by the operator
func ReconcileLeaderTracker(namespace string, operatorShortName string, client client.Client, rsf ResourceSharingFactory, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, leaderTrackerType string, assetsFolder *string) error {
	leaderTracker, _, err := lutils.GetLeaderTracker(namespace, operatorShortName, leaderTrackerType, client)
	// If the Leader Tracker is missing, create from scratch
	if err != nil && kerrors.IsNotFound(err) {
		leaderTracker.Labels[lutils.LeaderVersionLabel] = latestOperandVersion
		leaderTracker.ResourceVersion = ""
		leaderTrackers, err := CreateNewLeaderTrackerList(rsf, treeMap, replaceMap, latestOperandVersion, leaderTrackerType, assetsFolder)
		if err != nil {
			return err
		}
		return SaveLeaderTracker(rsf.CreateOrUpdate(), leaderTracker, leaderTrackers)
	} else if err != nil {
		return err
	}
	// If the Leader Tracker is outdated, delete it so that it gets recreated in another reconcile
	if leaderTracker.Labels[lutils.LeaderVersionLabel] != latestOperandVersion {
		if err := rsf.DeleteResources()(leaderTracker); err != nil {
			return err
		}
	}
	return nil
}

func CreateNewLeaderTrackerList(rsf ResourceSharingFactory, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, leaderTrackerType string, assetsFolder *string) (*[]lutils.LeaderTracker, error) {
	resourcesMatrix, resourcesRootNameList, err := rsf.LeaderTrackers()()
	if err != nil {
		return nil, err
	}
	leaderTracker := make([]lutils.LeaderTracker, 0)
	for i, resourcesList := range resourcesMatrix {
		UpdateLeaderTrackersFromUnstructuredList(rsf, &leaderTracker, resourcesList, treeMap, replaceMap, latestOperandVersion, resourcesRootNameList[i])
	}
	return &leaderTracker, nil
}

// Removes the instance as leader if instance is the leader and if no leaders are being tracked then delete the leader tracking Secret
func RemoveLeader(createOrUpdateCallback func(client.Object, metav1.Object, func() error) error, deleteResourceCallback func(obj client.Object) error, name string, leaderTracker *corev1.Secret, leaderTrackers *[]lutils.LeaderTracker) error {
	changeDetected := false
	noOwners := true
	// If the instance is being tracked, remove it
	for i := range *leaderTrackers {
		if (*leaderTrackers)[i].ClearOwnerIfMatching(name) {
			changeDetected = true
		}
		if (*leaderTrackers)[i].Owner != "" {
			noOwners = false
		}
	}
	if noOwners {
		if err := deleteResourceCallback(leaderTracker); err != nil {
			return err
		}
	} else if changeDetected {
		if err := SaveLeaderTracker(createOrUpdateCallback, leaderTracker, leaderTrackers); err != nil {
			return err
		}
	}
	return nil
}

func UpdateLeaderTrackersFromUnstructuredList(rsf ResourceSharingFactory, leaderTrackers *[]lutils.LeaderTracker, resourceList *unstructured.UnstructuredList, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, resourceRootName string) error {
	for i, resource := range resourceList.Items {
		labelsMap, _, err := unstructured.NestedMap(resource.Object, "metadata", "labels")
		if err != nil {
			return err
		}
		if pathIndexInterface, found := labelsMap[lutils.ResourcePathIndexLabel]; found {
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
			leader := lutils.LeaderTracker{
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
					leader.PathIndex = newPathIndex
					leader.Path = path
					// the path may have changed so the path index reference needs to be updated directly in the resource
					if err := rsf.CreateOrUpdate()(&resourceList.Items[i], nil, func() error {
						labelsMap, _, err := unstructured.NestedMap(resourceList.Items[i].Object, "metadata", "labels")
						if err != nil {
							return err
						}
						labelsMap[lutils.ResourcePathIndexLabel] = newPathIndex
						if err := unstructured.SetNestedMap(resourceList.Items[i].Object, labelsMap, "metadata", "labels"); err != nil {
							return err
						}
						return nil
					}); err != nil {
						return err
					}
				}
			} else if pathErr == nil { // only update the path metadata if this operator's decision tree structure recognizes the resource
				leader.Path = path
			} else {
				// A valid decision tree path could not be found, so it will not be used by the operator and this resource will not be tracked
				continue
			}
			nameString, err := rsf.LeaderTrackerName()(resource.Object) // the leader.Name field value is determined by *_resource_sharing.go impl of the ResourceSharingFactory interface
			if err != nil {
				return err
			}
			leader.Name = nameString[len(resourceRootName):]
			leader.EvictOwner()
			lutils.InsertIntoSortedLeaderTrackers(leaderTrackers, &leader)
		}
	}
	return nil
}

func RemoveLeaderTrackerReference(client client.Client,
	createOrUpdateCallback func(client.Object, metav1.Object, func() error) error,
	deleteResourceCallback func(obj client.Object) error,
	name, namespace, operatorShortName, resourceSharingFileName string) error {
	leaderTracker, leaderTrackers, err := lutils.GetLeaderTracker(namespace, operatorShortName, resourceSharingFileName, client)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return RemoveLeader(createOrUpdateCallback, deleteResourceCallback, name, leaderTracker, leaderTrackers)
}
