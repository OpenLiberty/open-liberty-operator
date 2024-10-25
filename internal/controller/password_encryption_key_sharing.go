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

func init() {
	lutils.LeaderTrackerMutexes.Store(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, &sync.Mutex{})
}

func (r *ReconcileOpenLiberty) reconcilePasswordEncryptionKey(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) (string, string, string, error) {
	if r.isPasswordEncryptionKeySharingEnabled(instance) {
		leaderName, thisInstanceIsLeader, _, err := r.reconcileLeader(instance, passwordEncryptionMetadata, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, true)
		if err != nil && !kerrors.IsNotFound(err) {
			return "", "", "", err
		}
		if thisInstanceIsLeader {
			// Is there a password encryption key to duplicate for internal use?
			if err := r.mirrorEncryptionKeySecretState(instance, passwordEncryptionMetadata); err != nil {
				return "Failed to process the password encryption key Secret", "", "", err
			}
		}

		// Does the namespace already have a password encryption key sharing Secret?
		encryptionSecret, err := r.hasInternalEncryptionKeySecret(instance, passwordEncryptionMetadata)
		if err == nil {
			// Is the password encryption key field in the Secret valid?
			if encryptionKey := string(encryptionSecret.Data["passwordEncryptionKey"]); len(encryptionKey) > 0 {
				// non-leaders should still be able to pass this process to return the encryption secret name
				if thisInstanceIsLeader {
					// Create the Liberty config that will mount into the pods
					err := r.createPasswordEncryptionKeyLibertyConfig(instance, passwordEncryptionMetadata, encryptionKey)
					if err != nil {
						return "Failed to create Liberty resources to share the password encryption key", "", "", err
					}
				} else {
					// non-leaders should yield for the password encryption leader to mirror the encryption key's state
					if !r.encryptionKeySecretMirrored(instance, passwordEncryptionMetadata) {
						return "", "", "", fmt.Errorf("Waiting for OpenLibertyApplication instance '" + leaderName + "' to mirror the shared Password Encryption Key Secret for the namespace '" + instance.Namespace + "'.")
					}
				}
				return "", encryptionSecret.Name, string(encryptionSecret.Data["lastRotation"]), nil
			}
		} else if !kerrors.IsNotFound(err) {
			return "Failed to get the password encryption key Secret", "", "", err
		}
	} else {
		err := r.RemoveLeaderTrackerReference(instance, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME)
		if err != nil {
			return "Failed to remove leader tracking reference to the password encryption key", "", "", err
		}
	}
	return "", "", "", nil
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
		_, err := r.hasUserEncryptionKeySecret(instance, passwordEncryptionMetadata)
		return err == nil
	}
	return false
}

func (r *ReconcileOpenLiberty) getInternalPasswordEncryptionKeyState(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) (string, string, bool, error) {
	if !r.isPasswordEncryptionKeySharingEnabled(instance) {
		return "", "", false, nil
	}
	secret, err := r.hasInternalEncryptionKeySecret(instance, passwordEncryptionMetadata)
	if err != nil {
		return "", "", true, err
	}
	passwordEncryptionKey := ""
	encryptionSecretLastRotation := ""
	if key, found := secret.Data["passwordEncryptionKey"]; found {
		passwordEncryptionKey = string(key)
	}
	if lastRotation, found := secret.Data["lastRotation"]; found {
		encryptionSecretLastRotation = string(lastRotation)
	}
	// TODO: cover when secret does not contain data key-value pairs
	return passwordEncryptionKey, encryptionSecretLastRotation, true, nil
}

// Returns the Secret that contains the password encryption key used internally by the operator
func (r *ReconcileOpenLiberty) hasInternalEncryptionKeySecret(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) (*corev1.Secret, error) {
	return r.getSecret(instance, lutils.LocalPasswordEncryptionKeyRootName+passwordEncryptionMetadata.Name+"-internal")
}

// Returns the Secret that contains the password encryption key provided by the user
func (r *ReconcileOpenLiberty) hasUserEncryptionKeySecret(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) (*corev1.Secret, error) {
	return r.getSecret(instance, lutils.PasswordEncryptionKeyRootName+passwordEncryptionMetadata.Name)
}

func (r *ReconcileOpenLiberty) encryptionKeySecretMirrored(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) bool {
	userEncryptionSecret, err := r.hasUserEncryptionKeySecret(instance, passwordEncryptionMetadata)
	if err != nil {
		return false
	}
	internalEncryptionSecret, err := r.hasInternalEncryptionKeySecret(instance, passwordEncryptionMetadata)
	if err != nil {
		return false
	}
	internalPasswordEncryptionKey := string(internalEncryptionSecret.Data["passwordEncryptionKey"])
	userPasswordEncryptionKey := string(userEncryptionSecret.Data["passwordEncryptionKey"])
	return userPasswordEncryptionKey != "" && internalPasswordEncryptionKey == userPasswordEncryptionKey
}

func (r *ReconcileOpenLiberty) mirrorEncryptionKeySecretState(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) error {
	userEncryptionSecret, userEncryptionSecretErr := r.hasUserEncryptionKeySecret(instance, passwordEncryptionMetadata)
	// Error if there was an issue getting the userEncryptionSecret
	if userEncryptionSecretErr != nil && !kerrors.IsNotFound(userEncryptionSecretErr) {
		return userEncryptionSecretErr
	}
	internalEncryptionSecret, internalEncryptionSecretErr := r.hasInternalEncryptionKeySecret(instance, passwordEncryptionMetadata)
	// Error if there was an issue getting the internalEncryptionSecret
	if internalEncryptionSecretErr != nil && !kerrors.IsNotFound(internalEncryptionSecretErr) {
		return internalEncryptionSecretErr
	}
	// Case 0: no user encryption secret, no internal encryption secret: secrets already mirrored
	// Case 1: no user encryption secret, internal encryption secret exists: so delete internalEncryptionSecret
	if kerrors.IsNotFound(userEncryptionSecretErr) {
		if kerrors.IsNotFound(internalEncryptionSecretErr) {
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
		internalPasswordEncryptionKey := internalEncryptionSecret.Data["passwordEncryptionKey"]
		userPasswordEncryptionKey := userEncryptionSecret.Data["passwordEncryptionKey"]
		if string(internalPasswordEncryptionKey) != string(userPasswordEncryptionKey) {
			internalEncryptionSecret.Data["passwordEncryptionKey"] = userPasswordEncryptionKey
			internalEncryptionSecret.Data["lastRotation"] = []byte(fmt.Sprint(time.Now().Unix()))
		}
		return nil
	})
}

// Deletes the mirrored encryption key secret if the initial encryption key secret no longer exists
func (r *ReconcileOpenLiberty) deleteMirroredEncryptionKeySecret(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) error {
	_, userEncryptionSecretErr := r.hasUserEncryptionKeySecret(instance, passwordEncryptionMetadata)
	// Error if there was an issue getting the userEncryptionSecret
	if userEncryptionSecretErr != nil && !kerrors.IsNotFound(userEncryptionSecretErr) {
		return userEncryptionSecretErr
	}
	internalEncryptionSecret, internalEncryptionSecretErr := r.hasInternalEncryptionKeySecret(instance, passwordEncryptionMetadata)
	// Error if there was an issue getting the internalEncryptionSecret
	if internalEncryptionSecretErr != nil && !kerrors.IsNotFound(internalEncryptionSecretErr) {
		return internalEncryptionSecretErr
	}
	// Case 1: no user encryption secret, internal encryption secret exists: so delete internalEncryptionSecret
	if kerrors.IsNotFound(userEncryptionSecretErr) && !kerrors.IsNotFound(internalEncryptionSecretErr) {
		if err := r.DeleteResource(internalEncryptionSecret); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileOpenLiberty) getSecret(instance *olv1.OpenLibertyApplication, secretName string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	secret.Name = secretName
	secret.Namespace = instance.GetNamespace()
	secret.Labels = lutils.GetRequiredLabels(secret.Name, "")
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, secret)
	return secret, err
}

// Creates the Liberty XML to mount the password encryption keys Secret into the application pods
func (r *ReconcileOpenLiberty) createPasswordEncryptionKeyLibertyConfig(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata, encryptionKey string) error {
	if len(encryptionKey) == 0 {
		return fmt.Errorf("a password encryption key was not specified")
	}

	// The Secret to hold the server.xml that will override the password encryption key for the Liberty server
	// This server.xml will be mounted in /output/resources/liberty-operator/encryptionKey.xml
	encryptionXMLSecretName := OperatorShortName + lutils.ManagedEncryptionServerXML + passwordEncryptionMetadata.Name
	encryptionXMLSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      encryptionXMLSecretName,
			Namespace: instance.GetNamespace(),
			Labels:    lutils.GetRequiredLabels(encryptionXMLSecretName, ""),
		},
	}
	if err := r.CreateOrUpdate(encryptionXMLSecret, nil, func() error {
		return lutils.CustomizeEncryptionKeyXML(encryptionXMLSecret, encryptionKey)
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
