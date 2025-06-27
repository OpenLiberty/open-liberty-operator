package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	tree "github.com/OpenLiberty/open-liberty-operator/utils/tree"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const LTPA_RESOURCE_SHARING_FILE_NAME = "ltpa"
const LTPA_KEY_RESOURCE_SHARING_FILE_NAME = LTPA_RESOURCE_SHARING_FILE_NAME
const LTPA_CONFIG_RESOURCE_SHARING_FILE_NAME = "ltpa-config"
const LTPA_CONFIG_1_RESOURCE_SHARING_FILE_NAME = "ltpa-config-1"
const LTPA_CONFIG_2_RESOURCE_SHARING_FILE_NAME = "ltpa-config-2"

type LTPAResource int

const (
	LTPAKey LTPAResource = iota
	LTPAConfig
)

func init() {
	lutils.LeaderTrackerMutexes.Store(LTPA_RESOURCE_SHARING_FILE_NAME, &sync.Mutex{})
}

func (r *ReconcileOpenLiberty) reconcileLTPAMetadata(instance *olv1.OpenLibertyApplication, treeMap map[string]interface{}, latestOperandVersion string, assetsFolder *string) (lutils.LeaderTrackerMetadataList, error) {
	metadataList := &lutils.LTPAMetadataList{}
	metadataList.Items = []lutils.LeaderTrackerMetadata{}

	// During runtime, the OpenLibertyApplication instance will decide what LTPA related resources to track by populating arrays of pathOptions and pathChoices
	pathOptionsList, pathChoicesList := r.getLTPAPathOptionsAndChoices(instance, latestOperandVersion)

	for i := range pathOptionsList {
		metadata := &lutils.LTPAMetadata{}
		pathOptions := pathOptionsList[i]
		pathChoices := pathChoicesList[i]

		// convert the path options and choices into a labelString, for a path of length n, the labelString is
		// constructed as a weaved array in format "<pathOptions[0]>.<pathChoices[0]>.<pathOptions[1]>.<pathChoices[1]>...<pathOptions[n-1]>.<pathChoices[n-1]>"
		labelString, err := tree.GetLabelFromDecisionPath(latestOperandVersion, pathOptions, pathChoices)
		if err != nil {
			return metadataList, err
		}
		// validate that the decision path such as "v1_4_0.managePasswordEncryption.<pathChoices[n-1]>" is a valid subpath in treeMap
		// an error here indicates a build time error created by the operator developer or pollution of the ltpa-decision-tree.yaml
		// Note: validSubPath is a substring of labelString and a valid path within treeMap; it will always hold that len(validSubPath) <= len(labelString)
		validSubPath, err := tree.CanTraverseTree(treeMap, labelString, true)
		if err != nil {
			return metadataList, err
		}
		// retrieve the LTPA leader tracker to re-use an existing name or to create a new metadata.Name
		leaderTracker, _, err := lutils.GetLeaderTracker(instance, OperatorShortName, LTPA_RESOURCE_SHARING_FILE_NAME, r.GetClient())
		if err != nil {
			return metadataList, err
		}
		// if the leaderTracker is on a mismatched version, wait for a subsequent reconcile loop to re-create the leader tracker
		if leaderTracker.Labels[lutils.LeaderVersionLabel] != latestOperandVersion {
			return metadataList, fmt.Errorf("waiting for the Leader Tracker to be updated")
		}

		// to avoid limitation with Kubernetes label values having a max length of 63, translate validSubPath into a path index
		pathIndex := tree.GetLeafIndex(treeMap, validSubPath)
		versionedPathIndex := fmt.Sprintf("%s.%d", latestOperandVersion, pathIndex)

		metadata.Path = validSubPath
		metadata.PathIndex = versionedPathIndex
		metadata.Name = r.getLTPAMetadataName(instance, leaderTracker, validSubPath, assetsFolder, LTPAResource(i))
		metadataList.Items = append(metadataList.Items, metadata)
	}
	return metadataList, nil
}

func (r *ReconcileOpenLiberty) getLTPAPathOptionsAndChoices(instance *olv1.OpenLibertyApplication, latestOperandVersion string) ([][]string, [][]string) {
	var pathOptionsList, pathChoicesList [][]string
	if latestOperandVersion == "v1_4_0" {
		isKeySharingEnabled := r.isLTPAKeySharingEnabled(instance)
		if isKeySharingEnabled {
			// 1. Generate a path option/choice for a leader to manage the LTPA key
			pathOptions := []string{"key"}  // ordering matters, it must follow the nodes of the LTPA decision tree in ltpa-decision-tree.yaml
			pathChoices := []string{"true"} // fix LTPA to use the default password encryption key (no suffix)
			pathOptionsList = append(pathOptionsList, pathOptions)
			pathChoicesList = append(pathChoicesList, pathChoices)

			// 2. Generate a path option/choice for a leader to manage the Liberty config
			pathOptions = []string{"config"}
			configChoice := "default"
			if r.isUsingPasswordEncryptionKeySharing(instance, &lutils.PasswordEncryptionMetadata{Name: ""}) {
				configChoice = "passwordencryption"
			}
			pathChoices = []string{configChoice} // fix LTPA to use the default password encryption key (no suffix)
			pathOptionsList = append(pathOptionsList, pathOptions)
			pathChoicesList = append(pathChoicesList, pathChoices)
		}
	}
	// else if latestOperandVersion == "v1_4_1" {
	// 	// for instance, say v1_4_1 introduces a new "type" variable with options "aes", "xor" or "hash"
	// 	// The sequence must match .tree.v1_4_1.type.aes.managePasswordEncryption -> false located in the ltpa-decision-tree.yaml file
	// 	// It is also possible that "type" is set to "xor" which will look like .tree.v1_4_1.type.xor.managePasswordEncryption -> false
	// 	// Since CanTraverseTree checks for a subpath and ".tree.v1_4_1.type.xor" terminates at a leaf, .tree.v1_4_1.type.xor.managePasswordEncryption will pass validation
	// 	pathOptions = []string{"type", "managePasswordEncryption"} // ordering matters, it must follow the nodes of the LTPA decision tree in ltpa-decision-tree.yaml
	// 	pathChoices = []string{"aes", strconv.FormatBool(r.isPasswordEncryptionKeySharingEnabled(instance))}
	// }
	return pathOptionsList, pathChoicesList
}

func (r *ReconcileOpenLiberty) getLTPAMetadataName(instance *olv1.OpenLibertyApplication, leaderTracker *corev1.Secret, validSubPath string, assetsFolder *string, ltpaResourceType LTPAResource) string {
	// if an existing resource name (suffix) for this key combination already exists, use it
	loc := lutils.CommaSeparatedStringContains(string(leaderTracker.Data[lutils.ResourcePathsKey]), validSubPath)
	if loc != -1 {
		suffix, _ := lutils.GetCommaSeparatedString(string(leaderTracker.Data[lutils.ResourcesKey]), loc)
		return suffix
	}

	if ltpaResourceType == LTPAKey {
		// For example, if the env variable LTPA_KEY_RESOURCE_SUFFIXES is set,
		// it can provide a comma separated string of length lutils.ResourceSuffixLength suffixes to exhaust
		//
		// spec:
		//   env:
		//     - name: LTPA_KEY_RESOURCE_SUFFIXES
		//       value: "aaaaa,bbbbb,ccccc,zzzzz,a1b2c"
		if predeterminedSuffixes, hasEnv := hasLTPAKeyResourceSuffixesEnv(instance); hasEnv {
			predeterminedSuffixesArray := lutils.GetCommaSeparatedArray(predeterminedSuffixes)
			for _, suffix := range predeterminedSuffixesArray {
				if len(suffix) == lutils.ResourceSuffixLength && lutils.IsLowerAlphanumericSuffix(suffix) && !strings.Contains(string(leaderTracker.Data[lutils.ResourcesKey]), suffix) {
					return "-" + suffix
				}
			}
		}
	} else if ltpaResourceType == LTPAConfig {
		// For example, if the env variable LTPA_CONFIG_RESOURCE_SUFFIXES is set,
		// it can provide a comma separated string of length lutils.ResourceSuffixLength suffixes to exhaust
		//
		// spec:
		//   env:
		//     - name: LTPA_CONFIG_RESOURCE_SUFFIXES
		//       value: "aaaaa,bbbbb,ccccc,zzzzz,a1b2c"
		if predeterminedSuffixes, hasEnv := hasLTPAConfigResourceSuffixesEnv(instance); hasEnv {
			predeterminedSuffixesArray := lutils.GetCommaSeparatedArray(predeterminedSuffixes)
			for _, suffix := range predeterminedSuffixesArray {
				if len(suffix) == lutils.ResourceSuffixLength && lutils.IsLowerAlphanumericSuffix(suffix) && !strings.Contains(string(leaderTracker.Data[lutils.ResourcesKey]), suffix) {
					return "-" + suffix
				}
			}
		}
	}

	// otherwise, generate a random suffix of length lutils.ResourceSuffixLength
	randomSuffix := lutils.GetRandomLowerAlphanumericSuffix(lutils.ResourceSuffixLength)
	suffixFoundInCluster := true // MUST check that the operator is not overriding another instance's untracked shared resource
	for strings.Contains(string(leaderTracker.Data[lutils.ResourcesKey]), randomSuffix) || suffixFoundInCluster {
		randomSuffix = lutils.GetRandomLowerAlphanumericSuffix(lutils.ResourceSuffixLength)
		// create the unstructured object; parse and obtain the sharedResourceName via the internal/controller/assets/ltpa-signature.yaml
		if sharedResource, sharedResourceName, err := lutils.CreateUnstructuredResourceFromSignature(LTPA_RESOURCE_SHARING_FILE_NAME, assetsFolder, OperatorShortName, randomSuffix); err == nil {
			err := r.GetClient().Get(context.TODO(), types.NamespacedName{Namespace: instance.GetNamespace(), Name: sharedResourceName}, sharedResource)
			if err != nil && kerrors.IsNotFound(err) {
				suffixFoundInCluster = false
			}
		}
	}
	return randomSuffix
}

func hasLTPAKeyResourceSuffixesEnv(instance *olv1.OpenLibertyApplication) (string, bool) {
	return hasResourceSuffixesEnv(instance, "LTPA_KEY_RESOURCE_SUFFIXES")
}

func hasLTPAConfigResourceSuffixesEnv(instance *olv1.OpenLibertyApplication) (string, bool) {
	return hasResourceSuffixesEnv(instance, "LTPA_CONFIG_RESOURCE_SUFFIXES")
}

// Create or use an existing LTPA Secret identified by LTPA metadata for the OpenLibertyApplication instance
func (r *ReconcileOpenLiberty) reconcileLTPAKeys(instance *olv1.OpenLibertyApplication, ltpaKeysMetadata *lutils.LTPAMetadata) (string, string, string, error) {
	ltpaSecretName := ""
	ltpaKeysLastRotation := ""
	if r.isLTPAKeySharingEnabled(instance) {
		ltpaSecretNameTemp, ltpaKeysLastRotationTemp, _, err := r.generateLTPAKeys(instance, ltpaKeysMetadata)
		ltpaKeysLastRotation = ltpaKeysLastRotationTemp
		ltpaSecretName = ltpaSecretNameTemp
		if err != nil {
			return "Failed to generate the shared LTPA keys Secret", ltpaSecretName, ltpaKeysLastRotation, err
		}
	} else {
		err := r.RemoveLeaderTrackerReference(instance, LTPA_RESOURCE_SHARING_FILE_NAME)
		if err != nil {
			return "Failed to remove leader tracking reference to the LTPA keys", ltpaSecretName, ltpaKeysLastRotation, err
		}
	}
	return "", ltpaSecretName, ltpaKeysLastRotation, nil
}

// Create or use an existing LTPA Secret identified by LTPA metadata for the OpenLibertyApplication instance
func (r *ReconcileOpenLiberty) reconcileLTPAConfig(instance *olv1.OpenLibertyApplication, ltpaKeysMetadata *lutils.LTPAMetadata, ltpaConfigMetadata *lutils.LTPAMetadata, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata, ltpaKeysLastRotation string, lastKeyRelatedRotation string) (string, string, error) {
	var err error
	var ltpaXMLSecretName string
	if r.isLTPAKeySharingEnabled(instance) {
		ltpaXMLSecretName, err = r.generateLTPAConfig(instance, ltpaKeysMetadata, ltpaConfigMetadata, passwordEncryptionMetadata, ltpaKeysLastRotation, lastKeyRelatedRotation)
		if err != nil {
			return "Failed to generate the shared LTPA config Secret", ltpaXMLSecretName, err
		}
	} else {
		err := r.RemoveLeaderTrackerReference(instance, LTPA_RESOURCE_SHARING_FILE_NAME)
		if err != nil {
			return "Failed to remove leader tracking reference to the LTPA config", "", err
		}
	}
	return "", ltpaXMLSecretName, nil
}

// Generates the LTPA keys file and returns the name of the Secret storing its metadata
func (r *ReconcileOpenLiberty) generateLTPAKeys(instance *olv1.OpenLibertyApplication, ltpaMetadata *lutils.LTPAMetadata) (string, string, string, error) {
	// Initialize LTPA resources
	passwordEncryptionMetadata := &lutils.PasswordEncryptionMetadata{Name: ""}

	ltpaXMLSecret := &corev1.Secret{}
	ltpaXMLSecretRootName := OperatorShortName + lutils.LTPAServerXMLSuffix
	ltpaXMLSecret.Name = ltpaXMLSecretRootName + ltpaMetadata.Name
	ltpaXMLSecret.Namespace = instance.GetNamespace()
	ltpaXMLSecret.Labels = lutils.GetRequiredLabels(ltpaXMLSecretRootName, ltpaXMLSecret.Name)

	ltpaXMLMountSecret := &corev1.Secret{}
	ltpaXMLMountSecretRootName := OperatorShortName + lutils.LTPAServerXMLMountSuffix
	ltpaXMLMountSecret.Name = ltpaXMLMountSecretRootName + ltpaMetadata.Name
	ltpaXMLMountSecret.Namespace = instance.GetNamespace()
	ltpaXMLMountSecret.Labels = lutils.GetRequiredLabels(ltpaXMLMountSecretRootName, ltpaXMLSecret.Name)

	ltpaSecret := &corev1.Secret{}
	ltpaSecretRootName := OperatorShortName + "-managed-ltpa"
	ltpaSecret.Name = ltpaSecretRootName + ltpaMetadata.Name
	ltpaSecret.Namespace = instance.GetNamespace()
	ltpaSecret.Labels = lutils.GetRequiredLabels(ltpaSecretRootName, ltpaSecret.Name)
	// If the LTPA Secret does not exist, run the Kubernetes Job to generate the shared ltpa.keys file and Secret
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaSecret.Name, Namespace: ltpaSecret.Namespace}, ltpaSecret)
	if err != nil && kerrors.IsNotFound(err) {
		leaderName, thisInstanceIsLeader, _, err := r.reconcileLeader(instance, ltpaMetadata, LTPA_RESOURCE_SHARING_FILE_NAME, true)
		if err != nil {
			return "", "", leaderName, err
		}
		// If this instance is not the leader, exit the reconcile loop
		if !thisInstanceIsLeader {
			return "", "", leaderName, fmt.Errorf("Waiting for OpenLibertyApplication instance '%s' to generate the shared LTPA keys file for the namespace '%s'.", leaderName, instance.Namespace)
		}

		// Check the password encryption key
		passwordEncryptionKey, encryptionSecretLastRotation, encryptionKeySharingEnabled, err := r.getInternalPasswordEncryptionKeyState(instance, passwordEncryptionMetadata)
		if encryptionKeySharingEnabled && err != nil {
			return "", "", "", err
		}

		password := lutils.GetRandomAlphanumeric(15)
		var currentPasswordEncryptionKey *string
		if len(passwordEncryptionKey) > 0 {
			currentPasswordEncryptionKey = &passwordEncryptionKey
		} else {
			currentPasswordEncryptionKey = nil
		}
		rawLTPAKeysStringData, err := createLTPAKeys(password, currentPasswordEncryptionKey)
		if err != nil {
			return "", "", "", err
		}
		ltpaKeysStringData, err := base64.StdEncoding.DecodeString(string(rawLTPAKeysStringData))
		if err != nil {
			return "", "", "", err
		}

		ltpaSecret.Labels[lutils.ResourcePathIndexLabel] = ltpaMetadata.PathIndex
		ltpaSecret.Data = make(map[string][]byte)
		if passwordEncryptionKey != "" && encryptionSecretLastRotation != "" {
			ltpaSecret.Data["encryptionSecretLastRotation"] = []byte(encryptionSecretLastRotation)
		}
		lastRotation := strconv.FormatInt(time.Now().Unix(), 10)
		ltpaSecret.Data["lastRotation"] = []byte(lastRotation)
		ltpaSecret.Data["rawPassword"] = []byte(password)
		ltpaSecret.Data[lutils.LTPAKeysFileName] = ltpaKeysStringData

		if err := r.CreateOrUpdate(ltpaSecret, nil, func() error {
			return nil
		}); err != nil {
			return "", "", "", err
		}
		return ltpaSecret.Name, lastRotation, leaderName, nil
	} else if err != nil {
		return "", "", "", err
	}
	leaderName, _, _, err := r.reconcileLeader(instance, ltpaMetadata, LTPA_RESOURCE_SHARING_FILE_NAME, true)
	if err != nil {
		return "", "", leaderName, err
	}
	lastRotation := string(ltpaSecret.Data["lastRotation"])
	return ltpaSecret.Name, lastRotation, leaderName, nil
}

// Generates the LTPA keys file and returns the name of the Secret storing its metadata
func (r *ReconcileOpenLiberty) generateLTPAConfig(instance *olv1.OpenLibertyApplication, ltpaKeysMetadata *lutils.LTPAMetadata, ltpaConfigMetadata *lutils.LTPAMetadata, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata, ltpaKeysLastRotation string, lastKeyRelatedRotation string) (string, error) {
	ltpaXMLSecret := &corev1.Secret{}
	ltpaXMLSecretRootName := OperatorShortName + lutils.LTPAServerXMLSuffix
	ltpaXMLSecret.Name = ltpaXMLSecretRootName + ltpaConfigMetadata.Name
	ltpaXMLSecret.Namespace = instance.GetNamespace()
	ltpaXMLSecret.Labels = lutils.GetRequiredLabels(ltpaXMLSecretRootName, ltpaXMLSecret.Name)

	ltpaXMLMountSecret := &corev1.Secret{}
	ltpaXMLMountSecretRootName := OperatorShortName + lutils.LTPAServerXMLMountSuffix
	ltpaXMLMountSecret.Name = ltpaXMLMountSecretRootName + ltpaConfigMetadata.Name
	ltpaXMLMountSecret.Namespace = instance.GetNamespace()
	ltpaXMLMountSecret.Labels = lutils.GetRequiredLabels(ltpaXMLMountSecretRootName, ltpaXMLSecret.Name)

	ltpaSecret := &corev1.Secret{}
	ltpaSecretRootName := OperatorShortName + "-managed-ltpa"
	ltpaSecret.Name = ltpaSecretRootName + ltpaKeysMetadata.Name
	ltpaSecret.Namespace = instance.GetNamespace()
	ltpaSecret.Labels = lutils.GetRequiredLabels(ltpaSecretRootName, ltpaSecret.Name)
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaSecret.Name, Namespace: ltpaSecret.Namespace}, ltpaSecret)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return ltpaXMLSecret.Name, err
		}
		leaderName, thisInstanceIsLeader, _, err := r.reconcileLeader(instance, ltpaKeysMetadata, LTPA_RESOURCE_SHARING_FILE_NAME, false) // false, since this function should not elect leader for LTPA keys generation
		if err != nil {
			return ltpaXMLSecret.Name, err
		}
		// If this instance is not the leader, exit the reconcile loop
		if !thisInstanceIsLeader {
			return ltpaXMLSecret.Name, fmt.Errorf("Waiting for OpenLibertyApplication instance '%s' to generate the shared LTPA keys file for the namespace '%s'.", leaderName, instance.Namespace)
		}
		return ltpaXMLSecret.Name, fmt.Errorf("An unknown error has occurred generating the LTPA Secret for namespace '%s'.", instance.Namespace)
	}

	// LTPA config leader starts here
	leaderName, thisInstanceIsLeader, _, err := r.reconcileLeader(instance, ltpaConfigMetadata, LTPA_RESOURCE_SHARING_FILE_NAME, true)
	if err != nil {
		return ltpaXMLSecret.Name, err
	}
	if !thisInstanceIsLeader {
		err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaXMLSecret.Name, Namespace: ltpaXMLSecret.Namespace}, ltpaXMLSecret)
		if err != nil {
			if kerrors.IsNotFound(err) {
				return ltpaXMLSecret.Name, fmt.Errorf("Waiting for OpenLibertyApplication instance '%s' to generate the shared LTPA config for the namespace '%s'.", leaderName, instance.Namespace)
			}
			return ltpaXMLSecret.Name, err
		}
		// check that the last rotation label has been set
		lastRotationLabel, found := ltpaXMLSecret.Labels[lutils.GetLastRotationLabelKey(LTPA_CONFIG_RESOURCE_SHARING_FILE_NAME)]
		if !found {
			// the label was not found, but the LTPA config leader is responsible for updating this label
			return ltpaXMLSecret.Name, fmt.Errorf("Waiting for OpenLibertyApplication instance '%s' to update the shared LTPA config for the namespace '%s'.", leaderName, instance.Namespace)
		}
		// non-leaders should only stop yielding (blocking) to the leader if the Liberty XML Secret has been updated to a later time than lastKeyRelatedRotation
		lastRotationUpdated, err := lutils.CompareStringTimeGreaterThanOrEqual(lastRotationLabel, lastKeyRelatedRotation)
		if err != nil {
			return ltpaXMLSecret.Name, err
		}
		if lastRotationUpdated {
			return ltpaXMLSecret.Name, nil
		}
		return ltpaXMLSecret.Name, fmt.Errorf("Waiting for OpenLibertyApplication instance '%s' to update the shared LTPA config for the namespace '%s'.", leaderName, instance.Namespace)
	}

	ltpaConfigSecret := &corev1.Secret{}
	ltpaConfigSecretRootName := OperatorShortName + "-managed-ltpa"
	isPasswordEncryptionKeySharing := r.isUsingPasswordEncryptionKeySharing(instance, passwordEncryptionMetadata)
	if isPasswordEncryptionKeySharing {
		ltpaConfigSecretRootName += "-keyed-password"
		ltpaConfigSecret.Name = ltpaConfigSecretRootName + ltpaConfigMetadata.Name
	} else {
		ltpaConfigSecretRootName += "-password"
		ltpaConfigSecret.Name = ltpaConfigSecretRootName + ltpaConfigMetadata.Name
	}
	ltpaConfigSecret.Namespace = instance.GetNamespace()
	ltpaConfigSecret.Labels = lutils.GetRequiredLabels(ltpaConfigSecretRootName, ltpaConfigSecret.Name)
	// If the LTPA password Secret does not exist, run the Kubernetes Job to generate the LTPA password Secret
	err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaConfigSecret.Name, Namespace: ltpaConfigSecret.Namespace}, ltpaConfigSecret)
	if err != nil && kerrors.IsNotFound(err) {
		leaderName, thisInstanceIsLeader, _, err := r.reconcileLeader(instance, ltpaConfigMetadata, LTPA_RESOURCE_SHARING_FILE_NAME, true)
		if err != nil {
			return ltpaXMLSecret.Name, err
		}
		// If this instance is not the leader, exit the reconcile loop
		if !thisInstanceIsLeader {
			return ltpaXMLSecret.Name, fmt.Errorf("Waiting for OpenLibertyApplication instance '%s' to generate the shared LTPA password Secret for the namespace '%s'.", leaderName, instance.Namespace)
		}

		// 1,3,3 patch - if rawPassword field is not present, create the Secret directly or delete LTPA Secret when user attempts to use password encryption
		if _, foundRawPassword := ltpaSecret.Data["rawPassword"]; !foundRawPassword {
			if r.isPasswordEncryptionKeySharingEnabled(instance) {
				// Because there is no rawPassword field, there is no way to generate an encrypted password from the LTPA Secret
				// Historically, 1,3,3 operator created LTPA Secrets with an already encrypted password under field .data.password
				// Whereas 1,4,0 and greater the LTPA Secrets are unencrypted in field .data.rawPassword
				// generateLTPAKeys() MUST continue to set the rawPassword field, otherwise a create/delete loop will occur here when password encryption is enabled
				if err := r.DeleteResource(ltpaSecret); err != nil {
					return ltpaXMLSecret.Name, err
				}
			} else {
				defaultLTPASecretPassword, foundPassword := ltpaSecret.Data["password"]
				defaultLTPASecretLastRotation, foundLastRotation := ltpaSecret.Data["lastRotation"]
				if foundPassword && foundLastRotation {
					ltpaConfigSecret.Data = make(map[string][]byte)
					ltpaConfigSecret.Data["password"] = []byte(defaultLTPASecretPassword)
					ltpaConfigSecret.Data["lastRotation"] = []byte(defaultLTPASecretLastRotation)
					ltpaConfigSecret.Labels = lutils.GetRequiredLabels(ltpaConfigSecretRootName, ltpaConfigSecret.Name)
					ltpaConfigSecret.Labels[lutils.ResourcePathIndexLabel] = ltpaConfigMetadata.PathIndex
					if err := r.CreateOrUpdate(ltpaConfigSecret, nil, func() error { return nil }); err != nil {
						return ltpaXMLSecret.Name, err
					}
				}
			}

		} else { // otherwise, create the LTPA Config
			password := string(ltpaSecret.Data["rawPassword"])

			// Check the password encryption key
			passwordEncryptionKey, encryptionSecretLastRotation, encryptionKeySharingEnabled, err := r.getInternalPasswordEncryptionKeyState(instance, passwordEncryptionMetadata)
			if encryptionKeySharingEnabled && err != nil {
				return "", err
			}

			var currentPasswordEncryptionKey *string
			if len(passwordEncryptionKey) > 0 {
				currentPasswordEncryptionKey = &passwordEncryptionKey
			} else {
				currentPasswordEncryptionKey = nil
			}
			encodedPassword, err := encode(password, currentPasswordEncryptionKey)
			if err != nil {
				return "", err
			}

			ltpaConfigSecret.Labels[lutils.ResourcePathIndexLabel] = ltpaConfigMetadata.PathIndex
			ltpaConfigSecret.Data = make(map[string][]byte)
			if passwordEncryptionKey != "" && encryptionSecretLastRotation != "" {
				ltpaConfigSecret.Data["encryptionKeyLastRotation"] = []byte(encryptionSecretLastRotation)
			}
			ltpaConfigSecret.Data["lastRotation"] = []byte(ltpaSecret.Data["lastRotation"])
			ltpaConfigSecret.Data["password"] = encodedPassword

			if err := r.CreateOrUpdate(ltpaConfigSecret, nil, func() error {
				return nil
			}); err != nil {
				return "", err
			}
		}
	} else if err != nil {
		return ltpaXMLSecret.Name, err
	}

	// if the LTPA password is outdated from the LTPA Secret, delete the LTPA password
	lastRotation, found := ltpaConfigSecret.Data["lastRotation"]
	if !found || string(lastRotation) != string(ltpaKeysLastRotation) {
		// lastRotation field is not present so the Secret was not initialized correctly
		err := r.DeleteResource(ltpaConfigSecret)
		if err != nil {
			return ltpaXMLSecret.Name, err
		}
		if !found {
			return ltpaXMLSecret.Name, fmt.Errorf("the LTPA password does not contain field 'lastRotation'")
		}
		return ltpaXMLSecret.Name, fmt.Errorf("the LTPA password is out of sync with the generated LTPA Secret; waiting for a new LTPA password to be generated")
	}

	// if using encryption key, check if the key has been rotated and requires a regeneration of the LTPA keyed password
	if isPasswordEncryptionKeySharing {
		internalEncryptionKeySecret, err := r.hasInternalEncryptionKeySecret(instance, passwordEncryptionMetadata)
		if err != nil {
			return ltpaXMLSecret.Name, err
		}
		lastRotation, found := internalEncryptionKeySecret.Data["lastRotation"]
		if !found {
			// lastRotation field is not present so the Secret was not initialized correctly
			err := r.DeleteResource(internalEncryptionKeySecret)
			if err != nil {
				return ltpaXMLSecret.Name, err
			}
			return ltpaXMLSecret.Name, fmt.Errorf("the internal encryption key secret does not contain field 'lastRotation'")
		}

		if encryptionKeyLastRotation, found := ltpaConfigSecret.Data["encryptionKeyLastRotation"]; found {
			if string(encryptionKeyLastRotation) != string(lastRotation) {
				err := r.DeleteResource(ltpaConfigSecret)
				if err != nil {
					return ltpaXMLSecret.Name, err
				}
				return ltpaXMLSecret.Name, fmt.Errorf("the encryption key has been modified; waiting for a new LTPA password to be generated")
			}
		}
	}

	// Create/update the Secret to hold the server.xml that will import the LTPA keys into the Liberty server
	// This server.xml will be mounted in /config/configDropins/overrides/ltpaKeysMount.xml
	serverXMLMountSecretErr := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaXMLMountSecret.Name, Namespace: ltpaXMLMountSecret.Namespace}, ltpaXMLMountSecret)
	if serverXMLMountSecretErr != nil && !kerrors.IsNotFound(serverXMLMountSecretErr) {
		return ltpaXMLSecret.Name, serverXMLMountSecretErr
	}
	if err := r.CreateOrUpdate(ltpaXMLMountSecret, nil, func() error {
		mountDir := strings.Replace(lutils.SecureMountPath+"/"+lutils.LTPAKeysXMLFileName, "/output", "${server.output.dir}", 1)
		return lutils.CustomizeLibertyFileMountXML(ltpaXMLMountSecret, lutils.LTPAKeysMountXMLFileName, mountDir)
	}); err != nil {
		return ltpaXMLSecret.Name, err
	}

	// Create/update the Liberty Server XML Secret
	serverXMLSecretErr := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaXMLSecret.Name, Namespace: ltpaXMLSecret.Namespace}, ltpaXMLSecret)
	if serverXMLSecretErr != nil && !kerrors.IsNotFound(serverXMLSecretErr) {
		return ltpaXMLSecret.Name, serverXMLSecretErr
	}
	// NOTE: Update is important here for compatibility with an operator upgrade from version 1,3,3 that did not use ltpaXMLMountSecret
	if err := r.CreateOrUpdate(ltpaXMLSecret, nil, func() error {
		// get the latest config rotation time, if it exists
		var latestRotationTime int
		lastRotationTime, err := strconv.Atoi(string(lastRotation))
		if err != nil {
			return fmt.Errorf("failed to convert last rotation time from string to integer")
		}
		latestRotationTime = lastRotationTime
		if encryptionKeyLastRotation, found := ltpaConfigSecret.Data["encryptionKeyLastRotation"]; found {
			encryptionKeyLastRotationTime, err := strconv.Atoi(string(encryptionKeyLastRotation))
			if err != nil {
				return fmt.Errorf("failed to convert encryption key last rotation time from string to integer")
			}
			if encryptionKeyLastRotationTime >= latestRotationTime {
				latestRotationTime = encryptionKeyLastRotationTime
			}
		}
		ltpaXMLSecret.Labels[lutils.GetLastRotationLabelKey(LTPA_CONFIG_RESOURCE_SHARING_FILE_NAME)] = strconv.Itoa(latestRotationTime)
		return lutils.CustomizeLTPAServerXML(ltpaXMLSecret, instance, string(ltpaConfigSecret.Data["password"]))
	}); err != nil {
		return ltpaXMLSecret.Name, err
	}
	return ltpaXMLSecret.Name, nil
}

func (r *ReconcileOpenLiberty) isLTPAKeySharingEnabled(instance *olv1.OpenLibertyApplication) bool {
	if instance.GetManageLTPA() != nil && *instance.GetManageLTPA() {
		return true
	}
	return false
}

// Search the cluster namespace for existing LTPA keys
func (r *ReconcileOpenLiberty) GetLTPAKeyResources(instance *olv1.OpenLibertyApplication, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, assetsFolder *string) (*unstructured.UnstructuredList, string, error) {
	ltpaResourceList, ltpaResourceRootName, err := lutils.CreateUnstructuredResourceListFromSignature(LTPA_KEY_RESOURCE_SHARING_FILE_NAME, assetsFolder, OperatorShortName)
	if err != nil {
		return nil, "", err
	}
	if err := r.GetClient().List(context.TODO(), ltpaResourceList, client.MatchingLabels{
		"app.kubernetes.io/name": ltpaResourceRootName,
	}, client.InNamespace(instance.GetNamespace())); err != nil {
		return nil, "", err
	}
	// check once for an unlabeled default LTPA key to append
	if defaultLTPAKeyIndex := defaultLTPAKeyExists(ltpaResourceList, ltpaResourceRootName); defaultLTPAKeyIndex == -1 {
		defaultLTPAKeySecret, _, err := lutils.CreateUnstructuredResourceFromSignature(LTPA_RESOURCE_SHARING_FILE_NAME, assetsFolder, OperatorShortName, "")
		defaultLTPAKeySecret.SetName(ltpaResourceRootName)
		defaultLTPAKeySecret.SetNamespace(instance.GetNamespace())
		if err != nil {
			return nil, "", err
		}
		if err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: defaultLTPAKeySecret.GetName(), Namespace: defaultLTPAKeySecret.GetNamespace()}, defaultLTPAKeySecret); err == nil {
			ltpaResourceList.Items = append(ltpaResourceList.Items, *defaultLTPAKeySecret)
		}
	}

	// If "olo-managed-ltpa" exists and there is no collision, patch the olo-managed-ltpa with a leader tracking label to work on the current resource tracking impl.
	if defaultLTPAKeyIndex := defaultLTPAKeyExists(ltpaResourceList, ltpaResourceRootName); defaultLTPAKeyIndex != -1 {
		defaultUpdatedPathIndex := ""
		// the "olo-managed-ltpa" would only exist on 1.3.3, so the path is hardcoded to start replaceMap translation at "v1_3_3.default"
		if path, err := tree.ReplacePath("v1_3_3.default", latestOperandVersion, treeMap, replaceMap); err == nil {
			defaultUpdatedPathIndex = strings.Split(path, ".")[0] + "." + strconv.FormatInt(int64(tree.GetLeafIndex(treeMap, path)), 10)
		}
		// to prevent collisions, for each LTPA key, check that the default LTPA key does not already exist
		if defaultUpdatedPathIndex != "" {
			// does "olo-managed-ltpa" already exist in a future operand version? - no backward compatibility currently exists without caching the decision tree
			hasKeyCollision, err := r.HasDefaultLTPAKeyCollision(ltpaResourceList, treeMap, replaceMap, ltpaResourceRootName, latestOperandVersion, defaultUpdatedPathIndex)
			if err != nil {
				return ltpaResourceList, ltpaResourceRootName, err
			}
			// no collisions for the default key exists, so "olo-managed-ltpa" can be migrated and reused in this operator version
			if !hasKeyCollision {
				if err := r.CreateOrUpdate(&ltpaResourceList.Items[defaultLTPAKeyIndex], nil, func() error {
					// add the ResourcePathIndexLabel which did not exist in 1,3,3
					labelsMap, _, _ := unstructured.NestedMap(ltpaResourceList.Items[defaultLTPAKeyIndex].Object, "metadata", "labels")
					if labelsMap == nil {
						labelsMap = make(map[string]interface{})
					}
					labelsMap[lutils.ResourcePathIndexLabel] = defaultUpdatedPathIndex
					if err := unstructured.SetNestedMap(ltpaResourceList.Items[defaultLTPAKeyIndex].Object, labelsMap, "metadata", "labels"); err != nil {
						return err
					}
					return nil
				}); err != nil {
					return ltpaResourceList, ltpaResourceRootName, err
				}
			}
		}
	}
	return ltpaResourceList, ltpaResourceRootName, nil
}

func (r *ReconcileOpenLiberty) HasDefaultLTPAKeyCollision(ltpaResourceList *unstructured.UnstructuredList, treeMap map[string]interface{}, replaceMap map[string]map[string]string, ltpaResourceRootName string, latestOperandVersion string, defaultUpdatedPathIndex string) (bool, error) {
	for _, resource := range ltpaResourceList.Items {
		if resource.GetName() != ltpaResourceRootName {
			labelsMap, _, err := unstructured.NestedMap(resource.Object, "metadata", "labels")
			if err != nil {
				return false, err
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
				// if the label matches the current operand version, check for a collision directly from the label
				if labelVersionArray[0] == latestOperandVersion {
					if defaultUpdatedPathIndex == pathIndex {
						return true, nil
					}
				} else {
					// otherwise, the label is on a different operand version, so translate via replaceMap to check if there is a collision
					indexIntVal, _ := strconv.ParseInt(labelVersionArray[1], 10, 64)
					path, pathErr := tree.GetPathFromLeafIndex(treeMap, labelVersionArray[0], int(indexIntVal))
					if pathErr == nil && labelVersionArray[0] != latestOperandVersion {
						if path, err := tree.ReplacePath(path, latestOperandVersion, treeMap, replaceMap); err == nil {
							updatedPathIndex := strings.Split(path, ".")[0] + "." + strconv.FormatInt(int64(tree.GetLeafIndex(treeMap, path)), 10)
							if defaultUpdatedPathIndex == updatedPathIndex {
								return true, nil
							}
						}
					}
				}
			}
		}
	}
	return false, nil
}

// Search the cluster namespace for existing LTPA password Secrets
func (r *ReconcileOpenLiberty) GetLTPAConfigResources(instance *olv1.OpenLibertyApplication, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, assetsFolder *string, fileName string) (*unstructured.UnstructuredList, string, error) {
	ltpaResourceList, ltpaResourceRootName, err := lutils.CreateUnstructuredResourceListFromSignature(fileName, assetsFolder, OperatorShortName)
	if err != nil {
		return nil, "", err
	}
	if err := r.GetClient().List(context.TODO(), ltpaResourceList, client.MatchingLabels{
		"app.kubernetes.io/name": ltpaResourceRootName,
	}, client.InNamespace(instance.GetNamespace())); err != nil {
		return nil, "", err
	}
	return ltpaResourceList, ltpaResourceRootName, nil
}

func defaultLTPAKeyExists(ltpaResourceList *unstructured.UnstructuredList, defaultKeyName string) int {
	for i, resource := range ltpaResourceList.Items {
		if resource.GetName() == defaultKeyName {
			return i
		}
	}
	return -1
}
