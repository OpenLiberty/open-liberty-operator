package controllers

import (
	"context"
	"strings"

	lutils "github.com/OpenLiberty/open-liberty-operator/utils"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcileOpenLiberty) reconcileEncryptionKeySharing(instance *olv1.OpenLibertyApplication) (string, string, error) {
	// Does this Liberty application instance enable password encryption key sharing?
	keySharingEnabled := r.isPasswordEncryptionKeySharingEnabled(instance)
	if keySharingEnabled {
		// Does the namespace already have a password encryption key sharing Secret?
		encryptionSecret, err := r.hasEncryptionKeySecret(instance)
		if err == nil {
			// Is the password encryption key field in the Secret valid?
			if encryptionKey := string(encryptionSecret.Data["passwordEncryptionKey"]); len(encryptionKey) > 0 {
				// Create the Liberty config that will mount into the pods
				err := r.createEncryptionKeyLibertyConfig(instance, encryptionKey)
				if err != nil {
					return "Failed to create Liberty resources to share the Encryption Key", encryptionSecret.Name, err
				}
				return "", encryptionSecret.Name, nil
			}
		}
	}
	// Delete the Liberty config that previously was mounted in the pod.
	err := r.deleteEncryptionKeyResources(instance)
	if err != nil {
		return "Failed to delete Liberty resources sharing the old Encryption Key", "", err
	}

	return "", "", nil
}

func (r *ReconcileOpenLiberty) isPasswordEncryptionKeySharingEnabled(instance *olv1.OpenLibertyApplication) bool {
	if instance.GetManagePasswordEncryption() == nil || *instance.GetManagePasswordEncryption() {
		return true
	}
	return false
}

func (r *ReconcileOpenLiberty) hasEncryptionKeySecret(instance *olv1.OpenLibertyApplication) (*corev1.Secret, error) {
	// The Secret that contains the password encryption key
	passwordKeySecret := &corev1.Secret{}
	passwordKeySecret.Name = OperatorShortName + lutils.PasswordEncryptionKeySuffix
	passwordKeySecret.Namespace = instance.GetNamespace()
	passwordKeySecret.Labels = lutils.GetRequiredLabels(passwordKeySecret.Name, "")
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: passwordKeySecret.Name, Namespace: passwordKeySecret.Namespace}, passwordKeySecret)
	return passwordKeySecret, err
}

// Gets the password encryption keys Secret and returns the name of the Secret storing its metadata
func (r *ReconcileOpenLiberty) createEncryptionKeyLibertyConfig(instance *olv1.OpenLibertyApplication, encryptionKey string) error {
	if len(encryptionKey) == 0 {
		return nil
	}

	// The Secret to hold the server.xml that will override the password encryption key for the Liberty server
	// This server.xml will be mounted in /output/resources/liberty-operator/encryptionKey.xml
	encryptionXMLSecretName := OperatorShortName + lutils.ManagedEncryptionServerXML
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
	mountingXMLSecretName := OperatorShortName + lutils.ManagedEncryptionMountServerXML
	mountingXMLSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mountingXMLSecretName,
			Namespace: instance.GetNamespace(),
			Labels:    lutils.GetRequiredLabels(mountingXMLSecretName, ""),
		},
	}
	if err := r.CreateOrUpdate(mountingXMLSecret, nil, func() error {
		mountDir := strings.Replace(lutils.SecureMountPath+"/"+lutils.EncryptionKeyXMLFileName, "/output", "${server.output.dir}", 1)
		return lutils.CustomizeEncryptionKeyMountXML(mountingXMLSecret, mountDir)
	}); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileOpenLiberty) deleteEncryptionKeyResources(instance *olv1.OpenLibertyApplication) error {
	// The Secret to hold the server.xml that will override the password encryption key for the Liberty server
	// This server.xml will be mounted in /output/resources/liberty-operator/encryptionKey.xml
	encryptionXMLSecret := &corev1.Secret{}
	encryptionXMLSecret.Name = OperatorShortName + lutils.ManagedEncryptionServerXML
	encryptionXMLSecret.Namespace = instance.GetNamespace()
	err := r.DeleteResource(encryptionXMLSecret)
	if err != nil {
		return err
	}

	// The Secret to hold the server.xml that will import the password encryption key into the Liberty server
	// This server.xml will be mounted in /config/configDropins/overrides/encryptionKeyMount.xml
	mountingXMLSecret := &corev1.Secret{}
	mountingXMLSecret.Name = OperatorShortName + lutils.ManagedEncryptionMountServerXML
	mountingXMLSecret.Namespace = instance.GetNamespace()
	err = r.DeleteResource(mountingXMLSecret)
	if err != nil {
		return err
	}
	return nil
}
