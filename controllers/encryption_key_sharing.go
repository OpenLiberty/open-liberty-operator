package controllers

import (
	"context"
	"fmt"
	"strings"

	lutils "github.com/OpenLiberty/open-liberty-operator/utils"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcileOpenLiberty) reconcileEncryptionKeySharing(instance *olv1.OpenLibertyApplication) (string, string, error) {
	if r.isPasswordEncryptionKeySharingEnabled(instance) {
		// Does the namespace already have a password encryption key sharing Secret?
		encryptionSecret, err := r.hasEncryptionKeySecret(instance)
		if err == nil {
			// Is the password encryption key field in the Secret valid?
			if encryptionKey := string(encryptionSecret.Data["passwordEncryptionKey"]); len(encryptionKey) > 0 {
				// Create the Liberty config that will mount into the pods
				err := r.createEncryptionKeyLibertyConfig(instance, encryptionKey)
				if err != nil {
					return "Failed to create Liberty resources to share the password encryption key", "", err
				}
				return "", encryptionSecret.Name, nil
			}
		}
	}
	err := r.deleteEncryptionKeyResources(instance)
	if err != nil {
		return "Failed to delete cluster resources for sharing the password encryption key", "", err
	}
	return "", "", nil
}

// Returns true if the OpenLibertyApplication instance initiated the password encryption keys sharing process or sets the instance as the leader if the password encryption keys are not yet shared
func (r *ReconcileOpenLiberty) getEncryptionKeySharingLeader(instance *olv1.OpenLibertyApplication, createServiceAccount bool) (string, bool, string, error) {
	encryptionSA := &corev1.ServiceAccount{}
	encryptionSA.Name = OperatorShortName + "-password-encryption"
	encryptionSA.Namespace = instance.GetNamespace()
	encryptionSA.Labels = lutils.GetRequiredLabels(encryptionSA.Name, "")
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: encryptionSA.Name, Namespace: encryptionSA.Namespace}, encryptionSA)
	if err != nil {
		if kerrors.IsNotFound(err) {
			if createServiceAccount {
				r.CreateOrUpdate(encryptionSA, instance, func() error {
					return nil
				})
				return instance.Name, true, encryptionSA.Name, nil
			}
			return "", false, "", nil
		}
		return "", false, encryptionSA.Name, err
	}
	encryptionKeySharingLeaderName := ""
	for _, ownerReference := range encryptionSA.OwnerReferences {
		if ownerReference.Name == instance.Name {
			return instance.Name, true, encryptionSA.Name, nil
		}
		encryptionKeySharingLeaderName = ownerReference.Name
	}
	return encryptionKeySharingLeaderName, false, encryptionSA.Name, nil
}

func (r *ReconcileOpenLiberty) isPasswordEncryptionKeySharingEnabled(instance *olv1.OpenLibertyApplication) bool {
	if instance.GetManagePasswordEncryption() == nil || *instance.GetManagePasswordEncryption() {
		return true
	}
	return false
}

func (r *ReconcileOpenLiberty) isUsingPasswordEncryptionKeySharing(instance *olv1.OpenLibertyApplication) bool {
	if r.isPasswordEncryptionKeySharingEnabled(instance) {
		_, err := r.hasEncryptionKeySecret(instance)
		return err == nil
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
		return fmt.Errorf("a password encryption key was not specified")
	}

	_, isEncryptionKeySharingLeader, _, err := r.getEncryptionKeySharingLeader(instance, true)
	if err != nil {
		return err
	}
	if !isEncryptionKeySharingLeader {
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
	_, isEncryptionKeySharingLeader, encryptionServiceAccountName, err := r.getEncryptionKeySharingLeader(instance, false)
	if err != nil {
		return err
	}
	if !isEncryptionKeySharingLeader {
		return nil
	}

	encryptionSA := &corev1.ServiceAccount{}
	encryptionSA.Name = encryptionServiceAccountName
	encryptionSA.Namespace = instance.GetNamespace()
	err = r.DeleteResource(encryptionSA)
	if err != nil {
		return err
	}
	return nil
}
