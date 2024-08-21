package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	tree "github.com/OpenLiberty/open-liberty-operator/tree"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

const PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME = "password-encryption"

func (r *ReconcileOpenLiberty) reconcilePasswordEncryptionKey(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) (string, string, error) {
	if r.isPasswordEncryptionKeySharingEnabled(instance) {
		_, thisInstanceIsLeader, _, err := r.reconcileLeader(instance, passwordEncryptionMetadata, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, true)
		if err != nil {
			return "", "", err
		}
		// If this instance is not the leader, exit this procedure
		if !thisInstanceIsLeader {
			return "", "", nil
		}
		// Is there a password encryption key to duplicate for internal use?
		if err := r.mirrorEncryptionKeySecretState(instance, passwordEncryptionMetadata); err != nil {
			return "Failed to process the password encryption key Secret", "", err
		}
		// Does the namespace already have a password encryption key sharing Secret?
		encryptionSecret, err := r.hasInternalEncryptionKeySecret(instance, passwordEncryptionMetadata)
		if err == nil {
			// Is the password encryption key field in the Secret valid?
			if encryptionKey := string(encryptionSecret.Data["passwordEncryptionKey"]); len(encryptionKey) > 0 {
				// Create the Liberty config that will mount into the pods
				err := r.createPasswordEncryptionKeyLibertyConfig(instance, passwordEncryptionMetadata, encryptionKey)
				if err != nil {
					return "Failed to create Liberty resources to share the password encryption key", "", err
				}
				return "", encryptionSecret.Name, nil
			}
		} else if !kerrors.IsNotFound(err) {
			return "Failed to get the password encryption key Secret", "", err
		}
	} else {
		err := r.RemoveLeaderTrackerReference(instance, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME)
		if err != nil {
			return "Failed to remove leader tracking reference to the password encryption key", "", err
		}
	}
	return "", "", nil
}

func (r *ReconcileOpenLiberty) reconcilePasswordEncryptionMetadata(treeMap map[string]interface{}, latestOperandVersion string) (*lutils.PasswordEncryptionMetadata, error) {
	metadata := &lutils.PasswordEncryptionMetadata{}

	pathOptions, pathChoices := r.getPasswordEncryptionPathOptionsAndChoices(latestOperandVersion)

	// convert the path options and choices into a labelString, for a path of length n, the labelString is
	// constructed as a weaved array in format "<pathOptions[0]>.<pathChoices[0]>.<pathOptions[1]>.<pathChoices[1]>...<pathOptions[n-1]>.<pathChoices[n-1]>"
	labelString, err := tree.GetLabelFromDecisionPath(latestOperandVersion, pathOptions, pathChoices)
	if err != nil {
		return metadata, err
	}
	// validate that the decision path such as "v1_4_0.managePasswordEncryption.true" is a valid subpath in treeMap
	// an error here indicates a build time error created by the operator developer or pollution of the password-encryption-decision-tree.yaml
	// NOTE: validSubPath is a substring of labelString and a valid path within treeMap; it will always hold that len(validSubPath) <= len(labelString)
	validSubPath, err := tree.CanTraverseTree(treeMap, labelString, true)
	if err != nil {
		return metadata, err
	}
	// NOTE: Checking the leaderTracker can be skipped assuming there is only one password encryption key per namespace
	// Leader tracker reconcile is only required to prevent overriding other shared resources (i.e. password encryption keys) in the same namespace
	// Uncomment code below to extend to multiple password encryption keys per namespace. See ltpa_keys_sharing.go for an example.

	// // retrieve the password encryption leader tracker to re-use an existing name or to create a new metadata.Name
	// leaderTracker, _, err := lutils.GetLeaderTracker(instance, OperatorShortName, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	// if err != nil {
	// 	return metadata, err
	// }
	// // if the leaderTracker is on a mismatched version, wait for a subsequent reconcile loop to re-create the leader tracker
	// if leaderTracker.Labels[lutils.LeaderVersionLabel] != latestOperandVersion {
	// 	return metadata, fmt.Errorf("waiting for the Leader Tracker to be updated")
	// }

	// to avoid limitation with Kubernetes label values having a max length of 63, translate validSubPath into a path index
	pathIndex := tree.GetLeafIndex(treeMap, validSubPath)
	versionedPathIndex := fmt.Sprintf("%s.%d", latestOperandVersion, pathIndex)

	metadata.Path = validSubPath
	metadata.PathIndex = versionedPathIndex
	metadata.Name = r.getPasswordEncryptionMetadataName() // You could augment this function to extend to multiple password encryption keys per namespace. See ltpa_keys_sharing.go for an example.
	return metadata, nil
}

func (r *ReconcileOpenLiberty) getPasswordEncryptionPathOptionsAndChoices(latestOperandVersion string) ([]string, []string) {
	var pathOptions, pathChoices []string
	if latestOperandVersion == "v1_4_0" {
		pathOptions = []string{"managePasswordEncryption"}
		pathChoices = []string{"true"} // there is only one possible password encryption key per namespace which corresponds to one path only
	}
	return pathOptions, pathChoices
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
	if instance.GetManagePasswordEncryption() == nil || *instance.GetManagePasswordEncryption() {
		return true
	}
	return false
}

func (r *ReconcileOpenLiberty) isUsingPasswordEncryptionKeySharing(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) bool {
	if r.isPasswordEncryptionKeySharingEnabled(instance) {
		_, err := r.hasUserEncryptionKeySecret(instance, passwordEncryptionMetadata)
		return err == nil
	}
	return false
}

// Returns the Secret that contains the password encryption key used internally by the operator
func (r *ReconcileOpenLiberty) hasInternalEncryptionKeySecret(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) (*corev1.Secret, error) {
	return r.getSecret(instance, OperatorShortName+lutils.PasswordEncryptionKeySuffix+passwordEncryptionMetadata.Name+"-internal")
}

// Returns the Secret that contains the password encryption key provided by the user
func (r *ReconcileOpenLiberty) hasUserEncryptionKeySecret(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) (*corev1.Secret, error) {
	return r.getSecret(instance, OperatorShortName+lutils.PasswordEncryptionKeySuffix+passwordEncryptionMetadata.Name)
}

func (r *ReconcileOpenLiberty) mirrorEncryptionKeySecretState(instance *olv1.OpenLibertyApplication, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata) error {
	userEncryptionSecret, userEncryptionSecretErr := r.hasUserEncryptionKeySecret(instance, passwordEncryptionMetadata)
	// Case 0: no user encryption secret, no internal encryption secret: secrets already mirrored
	// Case 1: no user encryption secret, internal encryption secret exists: userEncryptionSecret is the resource owner of internalEncryptionSecret and will be GC'd by k8s controller
	if userEncryptionSecretErr != nil {
		// Error if there was an issue getting the userEncryptionSecret
		if !kerrors.IsNotFound(userEncryptionSecretErr) {
			return userEncryptionSecretErr
		}
		return nil
	}
	internalEncryptionSecret, internalEncryptionSecretErr := r.hasInternalEncryptionKeySecret(instance, passwordEncryptionMetadata)
	// Error if there was an issue getting the internalEncryptionSecret
	if internalEncryptionSecretErr != nil && !kerrors.IsNotFound(internalEncryptionSecretErr) {
		return internalEncryptionSecretErr
	}

	// Case 2: user encryption secret exists, no internal secret: Create internalEncryptionSecret
	// Case 3: user encryption secret exists, internal secret exists: Update internalEncryptionSecret
	return r.CreateOrUpdate(internalEncryptionSecret, userEncryptionSecret, func() error {
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

	encrytionKeySecret, err := r.hasInternalEncryptionKeySecret(instance, passwordEncryptionMetadata)
	if err != nil {
		return err
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
	if err := r.CreateOrUpdate(encryptionXMLSecret, encrytionKeySecret, func() error {
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
	if err := r.CreateOrUpdate(mountingXMLSecret, encrytionKeySecret, func() error {
		mountDir := strings.Replace(lutils.SecureMountPath+"/"+lutils.EncryptionKeyXMLFileName, "/output", "${server.output.dir}", 1)
		return lutils.CustomizeLibertyFileMountXML(mountingXMLSecret, lutils.EncryptionKeyMountXMLFileName, mountDir)
	}); err != nil {
		return err
	}

	return nil
}

// Tracks existing password encryption resources by populating a LeaderTracker array used to initialize the LeaderTracker
func (r *ReconcileOpenLiberty) GetPasswordEncryptionResources(instance *olv1.OpenLibertyApplication, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, assetsFolder *string) (*unstructured.UnstructuredList, string, error) {
	passwordEncryptionResources, err := lutils.CreateUnstructuredResourceListFromSignature(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, assetsFolder, OperatorShortName)
	if err != nil {
		return nil, "", err
	}
	passwordEncryptionResource, passwordEncryptionResourceName, err := lutils.CreateUnstructuredResourceFromSignature(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, assetsFolder, OperatorShortName, "")
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
