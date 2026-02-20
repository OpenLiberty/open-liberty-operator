package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	tree "github.com/OpenLiberty/open-liberty-operator/utils/tree"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

const PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME = "password-encryption"
const PasswordEncryptionKey = "passwordEncryptionKey"

const AES_ENCRYPTION_RESOURCE_SHARING_FILE_NAME = "aes-encryption"
const AESEncryptionKey = "aesEncryptionKey"

func init() {
	lutils.LeaderTrackerMutexes.Store(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, &sync.Mutex{})
}

func (r *ReconcileOpenLiberty) reconcileEncryptionKey(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) (string, string, string, error) {
	if r.isPasswordEncryptionKeySharingEnabled(instance) {
		leaderName, thisInstanceIsLeader, _, err := r.reconcileLeader(instance, passwordEncryptionMetadata, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, true)
		if err != nil && !kerrors.IsNotFound(err) {
			return "", "", "", err
		}
		if thisInstanceIsLeader {
			// Is there a password encryption key to duplicate for internal use?
			if err := r.mirrorEncryptionKeySecretState(instance, passwordEncryptionMetadata, r.hasUserAESEncryptionKeySecret, r.hasInternalAESEncryptionKeySecret, AESEncryptionKey); err != nil {
				return "Failed to process the password encryption key (aes) Secret", "", "", err
			}
			if err := r.mirrorEncryptionKeySecretState(instance, passwordEncryptionMetadata, r.hasUserEncryptionKeySecret, r.hasInternalEncryptionKeySecret, PasswordEncryptionKey); err != nil {
				return "Failed to process the password encryption key (password) Secret", "", "", err
			}
		}
		aesEncryptionErrMessage, aesEncryptionSecretName, aesEncryptionLastRotation, err := r.reconcileAESEncryptionKey(instance, passwordEncryptionMetadata, thisInstanceIsLeader, leaderName)
		if err == nil {
			// no error so return the aes encryption key
			return aesEncryptionErrMessage, aesEncryptionSecretName, aesEncryptionLastRotation, err
		}
		return r.reconcilePasswordEncryptionKey(instance, passwordEncryptionMetadata, thisInstanceIsLeader, leaderName)
	} else {
		err := r.RemoveLeaderTrackerReference(instance, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME)
		if err != nil {
			return "Failed to remove leader tracking reference to the encryption key", "", "", err
		}
	}
	return "", "", "", nil
}

func (r *ReconcileOpenLiberty) reconcileAESEncryptionKey(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata, thisInstanceIsLeader bool, leaderName string) (string, string, string, error) {
	// Does the namespace already have a password encryption key sharing Secret?
	encryptionSecret, _, err := r.hasInternalAESEncryptionKeySecret(instance, passwordEncryptionMetadata)
	if err == nil {
		// Return err if the password encryption key does not exist
		if _, found := encryptionSecret.Data[AESEncryptionKey]; !found {
			return "Failed to get the password encryption key Secret because " + AESEncryptionKey + " key is missing", "", "", err
		}
		// Is the password encryption key field in the Secret valid?
		if encryptionKey := string(encryptionSecret.Data[AESEncryptionKey]); len(encryptionKey) > 0 {
			// non-leaders should still be able to pass this process to return the encryption secret name
			if thisInstanceIsLeader {
				// Create the Liberty config that will mount into the pods
				err := r.createAESEncryptionKeyLibertyConfig(instance, passwordEncryptionMetadata, encryptionKey)
				if err != nil {
					return "Failed to create Liberty resources to share the AES encryption key", "", "", err
				}
			} else {
				// non-leaders should yield for the password encryption leader to mirror the encryption key's state
				if !r.isSecretMirrored(instance, passwordEncryptionMetadata, r.hasUserAESEncryptionKeySecret, r.hasInternalAESEncryptionKeySecret, AESEncryptionKey) {
					return "", "", "", fmt.Errorf("Waiting for OpenLibertyApplication instance '%s' to mirror the shared Password Encryption Key (aes) Secret for the namespace '%s'.", leaderName, instance.Namespace)
				}
			}
			return "", encryptionSecret.Name, string(encryptionSecret.Data["lastRotation"]), nil
		}
	}
	return "Failed to get the AES encryption key Secret", "", "", err
}

func (r *ReconcileOpenLiberty) reconcilePasswordEncryptionKey(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata, thisInstanceIsLeader bool, leaderName string) (string, string, string, error) {
	// Does the namespace already have a password encryption key sharing Secret?
	encryptionSecret, _, err := r.hasInternalEncryptionKeySecret(instance, passwordEncryptionMetadata)
	if err == nil {
		// Return err if the password encryption key does not exist
		if _, found := encryptionSecret.Data[PasswordEncryptionKey]; !found {
			return "Failed to get the password encryption key Secret because " + PasswordEncryptionKey + " key is missing", "", "", err
		}
		// Is the password encryption key field in the Secret valid?
		if encryptionKey := string(encryptionSecret.Data[PasswordEncryptionKey]); len(encryptionKey) > 0 {
			// non-leaders should still be able to pass this process to return the encryption secret name
			if thisInstanceIsLeader {
				// Create the Liberty config that will mount into the pods
				err := r.createPasswordEncryptionKeyLibertyConfig(instance, passwordEncryptionMetadata, encryptionKey)
				if err != nil {
					return "Failed to create Liberty resources to share the password encryption key", "", "", err
				}
			} else {
				// non-leaders should yield for the password encryption leader to mirror the encryption key's state
				if !r.isSecretMirrored(instance, passwordEncryptionMetadata, r.hasUserEncryptionKeySecret, r.hasInternalEncryptionKeySecret, PasswordEncryptionKey) {
					return "", "", "", fmt.Errorf("Waiting for OpenLibertyApplication instance '%s' to mirror the shared Password Encryption Key (password) Secret for the namespace '%s'.", leaderName, instance.Namespace)
				}
			}
			return "", encryptionSecret.Name, string(encryptionSecret.Data["lastRotation"]), nil
		}
	}
	return "Failed to get the password encryption key Secret", "", "", err
}

func (r *ReconcileOpenLiberty) reconcilePasswordEncryptionMetadata(treeMap map[string]interface{}, latestOperandVersion string) (lutils.LeaderTrackerMetadataList, error) {
	metadataList := &lutils.PasswordEncryptionMetadataList{}
	metadataList.Items = []lutils.LeaderTrackerMetadata{}

	pathOptionsList, pathChoicesList := r.getPasswordEncryptionPathOptionsAndChoices(latestOperandVersion)
	for i := range pathOptionsList {
		metadata := &lutils.PasswordEncryptionMetadata{}
		pathOptions := pathOptionsList[i]
		pathChoices := pathChoicesList[i]

		// convert the path options and choices into a labelString, for a path of length n, the labelString is
		// constructed as a weaved array in format "<pathOptions[0]>.<pathChoices[0]>.<pathOptions[1]>.<pathChoices[1]>...<pathOptions[n-1]>.<pathChoices[n-1]>"
		labelString, err := tree.GetLabelFromDecisionPath(latestOperandVersion, pathOptions, pathChoices)
		if err != nil {
			return metadataList, err
		}
		// validate that the decision path such as "v1_4_0.managePasswordEncryption.true" is a valid subpath in treeMap
		// an error here indicates a build time error created by the operator developer or pollution of the password-encryption-decision-tree.yaml
		// NOTE: validSubPath is a substring of labelString and a valid path within treeMap; it will always hold that len(validSubPath) <= len(labelString)
		validSubPath, err := tree.CanTraverseTree(treeMap, labelString, true)
		if err != nil {
			return metadataList, err
		}
		// NOTE: Checking the leaderTracker can be skipped assuming there is only one password encryption key per namespace
		// Leader tracker reconcile is only required to prevent overriding other shared resources (i.e. password encryption keys) in the same namespace
		// Uncomment code below to extend to multiple password encryption keys per namespace. See ltpa_keys_sharing.go for an example.

		// // retrieve the password encryption leader tracker to re-use an existing name or to create a new metadata.Name
		// leaderTracker, _, err := lutils.GetLeaderTracker(instance, OperatorShortName, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, r.GetClient())
		// if err != nil {
		// 	return metadataList, err
		// }
		// // if the leaderTracker is on a mismatched version, wait for a subsequent reconcile loop to re-create the leader tracker
		// if leaderTracker.Labels[lutils.LeaderVersionLabel] != latestOperandVersion {
		// 	return metadataList, fmt.Errorf("waiting for the Leader Tracker to be updated")
		// }

		// to avoid limitation with Kubernetes label values having a max length of 63, translate validSubPath into a path index
		pathIndex := tree.GetLeafIndex(treeMap, validSubPath)
		versionedPathIndex := fmt.Sprintf("%s.%d", latestOperandVersion, pathIndex)

		metadata.Path = validSubPath
		metadata.PathIndex = versionedPathIndex
		metadata.Name = r.getPasswordEncryptionMetadataName() // You could augment this function to extend to multiple password encryption keys per namespace. See ltpa_keys_sharing.go for an example.
		metadataList.Items = append(metadataList.Items, metadata)
	}
	return metadataList, nil
}

func (r *ReconcileOpenLiberty) getPasswordEncryptionPathOptionsAndChoices(latestOperandVersion string) ([][]string, [][]string) {
	var pathOptionsList, pathChoicesList [][]string
	if latestOperandVersion == "v1_4_0" {
		pathOptions := []string{"managePasswordEncryption"}
		pathChoices := []string{"true"} // there is only one possible password encryption key per namespace which corresponds to one path only
		pathOptionsList = append(pathOptionsList, pathOptions)
		pathChoicesList = append(pathChoicesList, pathChoices)
	}
	return pathOptionsList, pathChoicesList
}

func (r *ReconcileOpenLiberty) getPasswordEncryptionMetadataName() string {
	// NOTE: there is only one possible password encryption key per namespace which corresponds to one shared resource name from password-encryption-signature.yaml
	// If you would like to have more than one password encryption key in a single namespace, use ltpa-signature.yaml as a template
	//
	// _, sharedResourceName, err := lutils.CreateUnstructuredResourceFromSignature(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, OperatorShortName, "")
	// if err != nil {
	// 	return "", err
	// }
	// return sharedResourceName, nil
	return "" // there is only one password encryption key per namespace which is represented by the empty string suffix
}

func (r *ReconcileOpenLiberty) isPasswordEncryptionKeySharingEnabled(instance *olv1.OpenLibertyApplication) bool {
	return instance.GetManagePasswordEncryption() != nil && *instance.GetManagePasswordEncryption()
}

func (r *ReconcileOpenLiberty) isUsingPasswordEncryptionKeySharing(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) bool {
	if r.isPasswordEncryptionKeySharingEnabled(instance) {
		_, _, err := r.hasUserEncryptionKeySecret(instance, passwordEncryptionMetadata)
		return err == nil
	}
	return false
}

func (r *ReconcileOpenLiberty) getEncryptionKeyData(encryptionSecret *corev1.Secret, matchedKey string) (string, string, bool) {
	encryptionKey := ""
	encryptionSecretLastRotation := ""
	if key, found := encryptionSecret.Data[matchedKey]; found {
		encryptionKey = string(key)
	}
	if lastRotation, found := encryptionSecret.Data["lastRotation"]; found {
		encryptionSecretLastRotation = string(lastRotation)
	}
	if encryptionKey == "" || encryptionSecretLastRotation == "" {
		// don't need to delete this misconfigured Secret because mirrorEncryptionKeySecretState will create/update it later
		return "", "", false
	}
	return encryptionKey, encryptionSecretLastRotation, true
}

func (r *ReconcileOpenLiberty) getValidInternalEncryptionKey(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) (*corev1.Secret, bool, bool, error, error) {
	sharingEnabled := r.isPasswordEncryptionKeySharingEnabled(instance)
	if !sharingEnabled {
		return nil, sharingEnabled, false, nil, nil
	}

	aesSecret, aesFound, err := r.hasInternalAESEncryptionKeySecret(instance, passwordEncryptionMetadata)
	if aesFound && err != nil {
		return nil, sharingEnabled, aesFound, err, nil
	}
	passwordSecret, passwordFound, err := r.hasInternalEncryptionKeySecret(instance, passwordEncryptionMetadata)
	if passwordFound && err != nil {
		return nil, sharingEnabled, aesFound, err, nil
	}

	_, _, aesValid := r.getEncryptionKeyData(aesSecret, AESEncryptionKey)
	_, _, passwordValid := r.getEncryptionKeyData(passwordSecret, PasswordEncryptionKey)

	aesFoundAndValid := aesFound && aesValid
	passwordFoundAndValid := passwordFound && passwordValid
	if aesFoundAndValid && passwordFoundAndValid {
		// use AES but provide a warning that password should be deleted
		return aesSecret, sharingEnabled, aesFound, fmt.Errorf("to avoid unexpected app downtime from Secret instability delete Secret wlp-password-encryption-key to continue using wlp-aes-encryption-key"), nil
	} else if passwordFoundAndValid {
		// use password
		return passwordSecret, sharingEnabled, aesFound, nil, nil
	} else if aesFoundAndValid {
		return aesSecret, sharingEnabled, aesFound, nil, nil
	}

	// if aes/password were found but not valid then return a warning
	if aesFound {
		return nil, sharingEnabled, aesFound, fmt.Errorf("the wlp-aes-encryption-key Secret was found but contained an invalid field"), err
	} else if passwordFound {
		return nil, sharingEnabled, aesFound, fmt.Errorf("the wlp-password-encryption-key Secret was found but contained an invalid field"), err
	}
	// do not error if aes and password were not found
	return nil, sharingEnabled, aesFound, nil, nil
}

func (r *ReconcileOpenLiberty) getInternalEncryptionKeyState(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) (string, string, bool, bool, error, error) {
	encryptionSecret, sharingEnabled, usingAES, err, warning := r.getValidInternalEncryptionKey(instance, passwordEncryptionMetadata)
	if !sharingEnabled {
		return "", "", sharingEnabled, false, nil, nil
	}
	matchedKey := ""
	if usingAES {
		matchedKey = AESEncryptionKey
	} else {
		matchedKey = PasswordEncryptionKey
	}

	key, lastRotation, valid := r.getEncryptionKeyData(encryptionSecret, matchedKey)
	if valid {
		return key, lastRotation, sharingEnabled, usingAES, err, warning
	}
	return "", "", sharingEnabled, false, fmt.Errorf("a password encryption key Secret was either not found or misconfigured"), nil
}

// Returns the Secret that contains the aes encryption key used internally by the operator
func (r *ReconcileOpenLiberty) hasInternalAESEncryptionKeySecret(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) (*corev1.Secret, bool, error) {
	return r.getSecret(instance, lutils.LocalAESEncryptionKeyRootName+passwordEncryptionMetadata.Name+"-internal")
}

// Returns the Secret that contains the aes encryption key provided by the user
func (r *ReconcileOpenLiberty) hasUserAESEncryptionKeySecret(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) (*corev1.Secret, bool, error) {
	return r.getSecret(instance, lutils.AESEncryptionKeyRootName+passwordEncryptionMetadata.Name)
}

// Returns the Secret that contains the password encryption key used internally by the operator
func (r *ReconcileOpenLiberty) hasInternalEncryptionKeySecret(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) (*corev1.Secret, bool, error) {
	return r.getSecret(instance, lutils.LocalPasswordEncryptionKeyRootName+passwordEncryptionMetadata.Name+"-internal")
}

// Returns the Secret that contains the password encryption key provided by the user
func (r *ReconcileOpenLiberty) hasUserEncryptionKeySecret(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) (*corev1.Secret, bool, error) {
	return r.getSecret(instance, lutils.PasswordEncryptionKeyRootName+passwordEncryptionMetadata.Name)
}

// Returns true if a user secret is mirrored to a corresponding "<user>-internal" secret
func (r *ReconcileOpenLiberty) isSecretMirrored(instance *olv1.OpenLibertyApplication,
	passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata,
	hasUserSecretFunc func(*olv1.OpenLibertyApplication, *lutils.PasswordEncryptionMetadata) (*corev1.Secret, bool, error),
	hasInternalSecretFunc func(*olv1.OpenLibertyApplication, *lutils.PasswordEncryptionMetadata) (*corev1.Secret, bool, error),
	matchedKey string) bool {
	userSecret, _, err := hasUserSecretFunc(instance, passwordEncryptionMetadata)
	if err != nil {
		return false
	}
	internalSecret, _, err := hasInternalSecretFunc(instance, passwordEncryptionMetadata)
	if err != nil {
		return false
	}
	internalKey := string(internalSecret.Data[matchedKey])
	userKey := string(userSecret.Data[matchedKey])
	return userKey != "" && internalKey == userKey
}

// Mirrors an internal and user secret that syncs the value of syncedKey
func (r *ReconcileOpenLiberty) mirrorEncryptionKeySecretState(instance *olv1.OpenLibertyApplication,
	passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata,
	hasUserSecretFunc func(*olv1.OpenLibertyApplication, *lutils.PasswordEncryptionMetadata) (*corev1.Secret, bool, error),
	hasInternalSecretFunc func(*olv1.OpenLibertyApplication, *lutils.PasswordEncryptionMetadata) (*corev1.Secret, bool, error),
	syncedKey string) error {
	userEncryptionSecret, userEncryptionFound, userEncryptionSecretErr := hasUserSecretFunc(instance, passwordEncryptionMetadata)
	// Error if there was an issue getting the userEncryptionSecret
	if !userEncryptionFound {
		return userEncryptionSecretErr
	}
	internalEncryptionSecret, internalEncryptionFound, internalEncryptionSecretErr := hasInternalSecretFunc(instance, passwordEncryptionMetadata)
	// Error if there was an issue getting the internalEncryptionSecret
	if !internalEncryptionFound {
		return internalEncryptionSecretErr
	}
	// Case 0: no user encryption secret, no internal encryption secret: secrets already mirrored
	// Case 1: no user encryption secret, internal encryption secret exists: so delete internalEncryptionSecret
	if !userEncryptionFound {
		if !internalEncryptionFound {
			return nil
		} else {
			if err := r.DeleteResource(internalEncryptionSecret); err != nil {
				return err
			}
		}
	}

	// Case 2: user encryption secret exists, no internal secret: Create internalEncryptionSecret
	// Case 3: user encryption secret exists, internal secret exists: Update internalEncryptionSecret
	return r.CreateOrUpdate(internalEncryptionSecret, nil, func() error {
		if internalEncryptionSecret.Data == nil {
			internalEncryptionSecret.Data = make(map[string][]byte)
		}
		if userEncryptionSecret.Data == nil {
			userEncryptionSecret.Data = make(map[string][]byte)
		}
		internalPasswordEncryptionKey := internalEncryptionSecret.Data[syncedKey]
		userPasswordEncryptionKey := userEncryptionSecret.Data[syncedKey]
		if string(internalPasswordEncryptionKey) != string(userPasswordEncryptionKey) {
			internalEncryptionSecret.Data[syncedKey] = userPasswordEncryptionKey
			internalEncryptionSecret.Data["lastRotation"] = []byte(fmt.Sprint(time.Now().Unix()))
		}
		return nil
	})
}

func (r *ReconcileOpenLiberty) getSecret(instance *olv1.OpenLibertyApplication, secretName string) (*corev1.Secret, bool, error) {
	secret := &corev1.Secret{}
	secret.Name = secretName
	secret.Namespace = instance.GetNamespace()
	secret.Labels = lutils.GetRequiredLabels(secret.Name, "")
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, secret)
	return secret, !kerrors.IsNotFound(err), err
}

// Creates the Liberty XML to mount the password encryption keys Secret into the application pods
func (r *ReconcileOpenLiberty) createPasswordEncryptionKeyLibertyConfig(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata, encryptionKey string) error {
	if len(encryptionKey) == 0 {
		return fmt.Errorf("a password encryption key was not specified")
	}

	// The Secret to hold the server.xml that will override the password encryption key for the Liberty server
	// This server.xml will be mounted in /output/liberty-operator/encryptionKey.xml
	encryptionXMLSecretName := OperatorShortName + lutils.ManagedEncryptionServerXML + passwordEncryptionMetadata.Name
	encryptionXMLSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      encryptionXMLSecretName,
			Namespace: instance.GetNamespace(),
			Labels:    lutils.GetRequiredLabels(encryptionXMLSecretName, ""),
		},
	}
	if err := r.CreateOrUpdate(encryptionXMLSecret, nil, func() error {
		return lutils.CustomizePasswordEncryptionKeyXML(encryptionXMLSecret, encryptionKey)
	}); err != nil {
		return err
	}

	// The Secret to hold the server.xml that will import the password encryption key into the Liberty server
	// This server.xml will be mounted in /config/configDropins/overrides/encryptionKeyMount.xml
	mountingXMLSecretName := OperatorShortName + lutils.ManagedEncryptionMountServerXML + passwordEncryptionMetadata.Name
	mountingXMLSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mountingXMLSecretName,
			Namespace: instance.GetNamespace(),
			Labels:    lutils.GetRequiredLabels(mountingXMLSecretName, ""),
		},
	}
	if err := r.CreateOrUpdate(mountingXMLSecret, nil, func() error {
		mountDir := strings.Replace(lutils.SecureMountPath+"/"+lutils.EncryptionKeyXMLFileName, "/output", "${server.output.dir}", 1)
		return lutils.CustomizeLibertyFileMountXML(mountingXMLSecret, lutils.EncryptionKeyMountXMLFileName, mountDir)
	}); err != nil {
		return err
	}

	return nil
}

// Creates the Liberty XML to mount the aes encryption keys Secret into the application pods
func (r *ReconcileOpenLiberty) createAESEncryptionKeyLibertyConfig(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata, encryptionKey string) error {
	if len(encryptionKey) == 0 {
		return fmt.Errorf("an AES encryption key was not specified")
	}

	// The Secret to hold the server.xml that will override the password encryption key for the Liberty server
	// This server.xml will be mounted in /output/liberty-operator/encryptionKey.xml
	encryptionXMLSecretName := OperatorShortName + lutils.ManagedEncryptionServerXML + passwordEncryptionMetadata.Name
	encryptionXMLSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      encryptionXMLSecretName,
			Namespace: instance.GetNamespace(),
			Labels:    lutils.GetRequiredLabels(encryptionXMLSecretName, ""),
		},
	}
	if err := r.CreateOrUpdate(encryptionXMLSecret, nil, func() error {
		return lutils.CustomizeAESEncryptionKeyXML(encryptionXMLSecret, encryptionKey)
	}); err != nil {
		return err
	}

	// The Secret to hold the server.xml that will import the password encryption key into the Liberty server
	// This server.xml will be mounted in /config/configDropins/overrides/encryptionKeyMount.xml
	mountingXMLSecretName := OperatorShortName + lutils.ManagedEncryptionMountServerXML + passwordEncryptionMetadata.Name
	mountingXMLSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mountingXMLSecretName,
			Namespace: instance.GetNamespace(),
			Labels:    lutils.GetRequiredLabels(mountingXMLSecretName, ""),
		},
	}
	if err := r.CreateOrUpdate(mountingXMLSecret, nil, func() error {
		mountDir := strings.Replace(lutils.SecureMountPath+"/"+lutils.EncryptionKeyXMLFileName, "/output", "${server.output.dir}", 1)
		return lutils.CustomizeLibertyFileMountXML(mountingXMLSecret, lutils.EncryptionKeyMountXMLFileName, mountDir)
	}); err != nil {
		return err
	}

	return nil
}

// Tracks existing password encryption resources by populating a LeaderTracker array used to initialize the LeaderTracker
func (r *ReconcileOpenLiberty) GetPasswordEncryptionResources(instance *olv1.OpenLibertyApplication, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, assetsFolder *string) (*unstructured.UnstructuredList, string, error) {
	passwordEncryptionResources, _, err := lutils.CreateUnstructuredResourceListFromSignature(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, assetsFolder, "") // TODO: replace prefix "" to specify operator precedence such as with prefix "olo-"
	if err != nil {
		return nil, "", err
	}
	passwordEncryptionResource, passwordEncryptionResourceName, err := lutils.CreateUnstructuredResourceFromSignature(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, assetsFolder, "", "") // TODO: replace prefix "" to specify operator precedence such as with prefix "olo-"
	if err != nil {
		return nil, "", err
	}
	if err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: passwordEncryptionResourceName, Namespace: instance.GetNamespace()}, passwordEncryptionResource); err == nil {
		passwordEncryptionResources.Items = append(passwordEncryptionResources.Items, *passwordEncryptionResource)
	} else if !kerrors.IsNotFound(err) {
		return nil, "", err
	}
	return passwordEncryptionResources, passwordEncryptionResourceName, nil
}
