package controllers

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	tree "github.com/OpenLiberty/open-liberty-operator/tree"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Validates the LTPA decision tree YAML and generates the leader tracking state (ConfigMap) for maintaining multiple LTPA Secrets
// Returns the LTPA metadata that identifies the type of LTPA key that the OpenLibertyApplication instance wants to use
func (r *ReconcileOpenLiberty) reconcileLTPAState(instance *olv1.OpenLibertyApplication) (*lutils.LTPAMetadata, error) {
	treeMap, replaceMap, err := tree.ParseLTPADecisionTree(nil)
	if err != nil {
		return nil, err
	}

	// TODO: uncomment when operator version switches to 1.4.0
	// latestOperandVersion, err := tree.GetLatestOperandVersion(treeMap, "")
	// if err != nil {
	// 	return nil, err
	// }
	latestOperandVersion := "v1_4_0" // remove this when operator version switches to 1.4.0

	// generate a ConfigMap to store the shared LTPA Secrets' state
	err = r.initializeLTPALeaderTracker(instance, treeMap, replaceMap, latestOperandVersion)
	if err != nil {
		return nil, err
	}

	// get the versioned LTPA decision tree and LTPA metadata specific to the operator and instance being reconciled
	ltpaMetadata, err := r.reconcileLTPAMetadata(instance, treeMap, latestOperandVersion)
	if err != nil {
		return nil, err
	}
	return ltpaMetadata, nil
}

func (r *ReconcileOpenLiberty) reconcileLTPAMetadata(instance *olv1.OpenLibertyApplication, treeMap map[string]interface{}, latestOperandVersion string) (*lutils.LTPAMetadata, error) {
	metadata := &lutils.LTPAMetadata{}

	var pathOptions, pathChoices []string
	if latestOperandVersion == "v1_4_0" {
		pathOptions = []string{"managePasswordEncryption"} // ordering matters, it must follow the nodes of the LTPA decision tree in ltpa-decision-tree.yaml
		pathChoices = []string{strconv.FormatBool(r.isUsingPasswordEncryptionKeySharing(instance))}
	}
	// else if latestOperandVersion == "v1_4_1" {
	// 	// for instance, say v1_4_1 introduces a new "type" variable with options "aes", "xor" or "hash"
	// 	// The sequence must match .tree.v1_4_1.type.aes.managePasswordEncryption -> false located in the ltpa-decision-tree.yaml file
	// 	// It is also possible that "type" is set to "xor" which will look like .tree.v1_4_1.type.xor.managePasswordEncryption -> false
	// 	// Since CanTraverseTree checks for a subpath and ".tree.v1_4_1.type.xor" terminates at a leaf, .tree.v1_4_1.type.xor.managePasswordEncryption will pass validation
	// 	pathOptions = []string{"type", "managePasswordEncryption"} // ordering matters, it must follow the nodes of the LTPA decision tree in ltpa-decision-tree.yaml
	// 	pathChoices = []string{"aes", strconv.FormatBool(r.isPasswordEncryptionKeySharingEnabled(instance))}
	// }

	// convert the path options and choices into a labelKey such as "v1_4_0.managePasswordEncryption" and labelValue "<pathChoices[0]>"
	labelString, err := tree.GetLabelFromDecisionPath(latestOperandVersion, pathOptions, pathChoices)
	if err != nil {
		return metadata, err
	}
	// validate that the decision path such as "v1_4_0.managePasswordEncryption:<pathChoices[0]>" is a valid subpath in treeMap
	validSubPath, err := tree.CanTraverseTree(treeMap, labelString, true)
	if err != nil {
		return metadata, err
	}

	// check Secrets to see if LTPA keys already exist, select by lutils.LTPAPathLabel
	leaderTracker, err := r.getLTPALeaderTracker(instance)
	if err != nil {
		return metadata, err
	}
	// prevent multiple LTPA keys from being created due to version changes
	if leaderTracker.Labels[lutils.LTPAVersionLabel] != latestOperandVersion {
		return metadata, fmt.Errorf("waiting for the LTPA leader tracker to be updated")
	}

	pathIndex := tree.GetLeafIndex(treeMap, validSubPath)
	versionedPathIndex := fmt.Sprintf("%s.%d", latestOperandVersion, pathIndex)

	metadata.Path = validSubPath
	metadata.PathIndex = versionedPathIndex

	// if an existing LTPA suffix for this key combination already exists, use it
	loc := tree.CommaSeparatedStringContains(leaderTracker.Data[lutils.ResourcePathsKey], validSubPath)
	if loc != -1 {
		suffix, _ := tree.GetCommaSeparatedString(leaderTracker.Data[lutils.ResourcesKey], loc)
		metadata.NameSuffix = suffix
		return metadata, nil
	}

	// if the env variable LTPA_RESOURCE_SUFFIXES is set, it can provide a comma separated string of length 5 suffixes to exhaust (can be used in test and production to provide predictability to LTPA Secret naming)
	// Example:
	// spec:
	//   env:
	//     - name: LTPA_RESOURCE_SUFFIXES
	//       value: "aaaaa,bbbbb,ccccc,zzzzz,a1b2c"
	if predeterminedSuffixes, hasEnv := hasLTPAResourceSuffixesEnv(instance); hasEnv {
		predeterminedSuffixesArray := tree.GetCommaSeparatedArray(predeterminedSuffixes)
		for _, suffix := range predeterminedSuffixesArray {
			if len(suffix) == 5 && tree.IsLowerAlphanumericSuffix(suffix) && !strings.Contains(leaderTracker.Data[lutils.ResourcesKey], suffix) {
				metadata.NameSuffix = "-" + suffix
				return metadata, nil
			}
		}
	}

	// otherwise, generate a random suffix
	randomSuffix := tree.GetRandomLowerAlphanumericSuffix(5)
	for strings.Contains(leaderTracker.Data[lutils.ResourcesKey], randomSuffix) {
		randomSuffix = tree.GetRandomLowerAlphanumericSuffix(5)
	}
	metadata.NameSuffix = randomSuffix
	return metadata, nil
}

func hasLTPAResourceSuffixesEnv(instance *olv1.OpenLibertyApplication) (string, bool) {
	for _, env := range instance.GetEnv() {
		if env.Name == "LTPA_RESOURCE_SUFFIXES" {
			return env.Value, true
		}
	}
	return "", false
}

// Create or use an existing LTPA Secret identified by LTPA metadata for the OpenLibertyApplication instance
func (r *ReconcileOpenLiberty) reconcileLTPASecret(instance *olv1.OpenLibertyApplication, ltpaMetadata *lutils.LTPAMetadata) (string, string, error) {
	var err error
	ltpaSecretName := ""
	if r.isLTPAKeySharingEnabled(instance) {
		ltpaSecretName, err = r.generateLTPAKeys(instance, ltpaMetadata)
		if err != nil {
			return "Failed to generate the shared LTPA Keys file", ltpaSecretName, err
		}
	} else {
		err := r.deleteLTPAKeysResources(instance)
		if err != nil {
			return "Failed to delete LTPA Keys Resource", ltpaSecretName, err
		}
	}
	return "", ltpaSecretName, nil
}

// If the LTPA Secret is being created but does not exist yet, the LTPA instance leader will halt the process and restart creation of LTPA keys
func (r *ReconcileOpenLiberty) restartLTPAKeysGeneration(instance *olv1.OpenLibertyApplication, ltpaMetadata *lutils.LTPAMetadata) error {
	_, isLTPAKeySharingLeader, _, _, err := r.getLTPAKeysSharingLeader(instance, ltpaMetadata, false)
	if err != nil {
		return err
	}
	if isLTPAKeySharingLeader {
		ltpaSecret := &corev1.Secret{}
		ltpaSecret.Name = OperatorShortName + "-managed-ltpa" + ltpaMetadata.NameSuffix
		ltpaSecret.Namespace = instance.GetNamespace()
		err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaSecret.Name, Namespace: ltpaSecret.Namespace}, ltpaSecret)
		if err != nil && kerrors.IsNotFound(err) {
			// Deleting the job request removes existing LTPA resources and restarts the LTPA generation process
			ltpaJobRequest := &corev1.ConfigMap{}
			ltpaJobRequest.Name = OperatorShortName + "-managed-ltpa-job-request"
			ltpaJobRequest.Namespace = instance.GetNamespace()
			err = r.DeleteResource(ltpaJobRequest)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Generates the LTPA keys file and returns the name of the Secret storing its metadata
func (r *ReconcileOpenLiberty) generateLTPAKeys(instance *olv1.OpenLibertyApplication, ltpaMetadata *lutils.LTPAMetadata) (string, error) {
	// Initialize LTPA resources
	ltpaXMLSecret := &corev1.Secret{}
	ltpaXMLSecretRootName := OperatorShortName + lutils.LTPAServerXMLSuffix
	ltpaXMLSecret.Name = ltpaXMLSecretRootName + ltpaMetadata.NameSuffix
	ltpaXMLSecret.Namespace = instance.GetNamespace()
	ltpaXMLSecret.Labels = lutils.GetRequiredLabels(ltpaXMLSecretRootName, ltpaXMLSecret.Name)

	ltpaXMLMountSecret := &corev1.Secret{}
	ltpaXMLMountSecretRootName := OperatorShortName + lutils.LTPAServerXMLMountSuffix
	ltpaXMLMountSecret.Name = ltpaXMLMountSecretRootName + ltpaMetadata.NameSuffix
	ltpaXMLMountSecret.Namespace = instance.GetNamespace()
	ltpaXMLMountSecret.Labels = lutils.GetRequiredLabels(ltpaXMLMountSecretRootName, ltpaXMLSecret.Name)

	generateLTPAKeysJob := &v1.Job{}
	generateLTPAKeysJobRootName := OperatorShortName + "-managed-ltpa-keys-generation"
	generateLTPAKeysJob.Name = generateLTPAKeysJobRootName + ltpaMetadata.NameSuffix
	generateLTPAKeysJob.Namespace = instance.GetNamespace()
	generateLTPAKeysJob.Labels = lutils.GetRequiredLabels(generateLTPAKeysJobRootName, generateLTPAKeysJob.Name)

	deletePropagationBackground := metav1.DeletePropagationBackground

	ltpaJobRequest := &corev1.ConfigMap{}
	ltpaJobRequestRootName := OperatorShortName + "-managed-ltpa-job-request"
	ltpaJobRequest.Name = ltpaJobRequestRootName + ltpaMetadata.NameSuffix
	ltpaJobRequest.Namespace = instance.GetNamespace()
	ltpaJobRequest.Labels = lutils.GetRequiredLabels(ltpaJobRequestRootName, ltpaJobRequest.Name)

	ltpaKeysCreationScriptConfigMap := &corev1.ConfigMap{}
	ltpaKeysCreationScriptConfigMapRootName := OperatorShortName + "-managed-ltpa-script"
	ltpaKeysCreationScriptConfigMap.Name = ltpaKeysCreationScriptConfigMapRootName + ltpaMetadata.NameSuffix
	ltpaKeysCreationScriptConfigMap.Namespace = instance.GetNamespace()
	ltpaKeysCreationScriptConfigMap.Labels = lutils.GetRequiredLabels(ltpaKeysCreationScriptConfigMapRootName, ltpaKeysCreationScriptConfigMap.Name)

	ltpaSecret := &corev1.Secret{}
	ltpaSecretRootName := OperatorShortName + "-managed-ltpa"
	ltpaSecret.Name = ltpaSecretRootName + ltpaMetadata.NameSuffix
	ltpaSecret.Namespace = instance.GetNamespace()
	ltpaSecret.Labels = lutils.GetRequiredLabels(ltpaSecretRootName, ltpaSecret.Name)
	// If the LTPA Secret does not exist, run the Kubernetes Job to generate the shared ltpa.keys file and Secret
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaSecret.Name, Namespace: ltpaSecret.Namespace}, ltpaSecret)
	if err != nil && kerrors.IsNotFound(err) {
		ltpaKeySharingLeaderName, isLTPAKeySharingLeader, ltpaServiceAccountName, _, err := r.getLTPAKeysSharingLeader(instance, ltpaMetadata, true)
		if err != nil {
			return "", err
		}
		// If this instance is not the leader, exit the reconcile loop
		if !isLTPAKeySharingLeader {
			return "", fmt.Errorf("Waiting for OpenLibertyApplication instance '" + ltpaKeySharingLeaderName + "' to generate the shared LTPA keys file for the namespace '" + instance.Namespace + "'.")
		}

		err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaJobRequest.Name, Namespace: ltpaJobRequest.Namespace}, ltpaJobRequest)
		if err != nil {
			// Create the Job Request if it doesn't exist
			if kerrors.IsNotFound(err) {
				// Clear all LTPA-related resources from a prior reconcile
				err = r.DeleteResource(ltpaXMLSecret)
				if err != nil {
					return "", err
				}
				err = r.DeleteResource(ltpaXMLMountSecret)
				if err != nil {
					return "", err
				}
				err = r.DeleteResource(ltpaKeysCreationScriptConfigMap)
				if err != nil {
					return "", err
				}
				err = r.GetClient().Delete(context.TODO(), generateLTPAKeysJob, &client.DeleteOptions{PropagationPolicy: &deletePropagationBackground})
				if err != nil && !kerrors.IsNotFound(err) {
					return "", err
				}
				err := r.CreateOrUpdate(ltpaJobRequest, instance, func() error {
					return nil
				})
				if err != nil {
					return "", fmt.Errorf("Failed to create ConfigMap " + ltpaJobRequest.Name)
				}
			} else {
				return "", fmt.Errorf("Failed to get ConfigMap " + ltpaJobRequest.Name)
			}
		} else {
			// Create the Role/RoleBinding
			ltpaRole := &rbacv1.Role{}
			ltpaRole.Name = OperatorShortName + "-managed-ltpa-role"
			ltpaRole.Namespace = instance.GetNamespace()
			ltpaRole.Rules = []rbacv1.PolicyRule{
				{
					Verbs:     []string{"create", "get"},
					APIGroups: []string{""},
					Resources: []string{"secrets"},
				},
			}
			ltpaRole.Labels = lutils.GetRequiredLabels(ltpaRole.Name, "")
			r.CreateOrUpdate(ltpaRole, instance, func() error {
				return nil
			})

			ltpaRoleBinding := &rbacv1.RoleBinding{}
			ltpaRoleBinding.Name = OperatorShortName + "-managed-ltpa-rolebinding"
			ltpaRoleBinding.Namespace = instance.GetNamespace()
			ltpaRoleBinding.Subjects = []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      ltpaServiceAccountName,
					Namespace: instance.GetNamespace(),
				},
			}
			ltpaRoleBinding.RoleRef = rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     ltpaRole.Name,
			}
			ltpaRoleBinding.Labels = lutils.GetRequiredLabels(ltpaRoleBinding.Name, "")
			r.CreateOrUpdate(ltpaRoleBinding, instance, func() error {
				return nil
			})

			// Create a ConfigMap to store the controllers/assets/create_ltpa_keys.sh script
			err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaKeysCreationScriptConfigMap.Name, Namespace: ltpaKeysCreationScriptConfigMap.Namespace}, ltpaKeysCreationScriptConfigMap)
			if err != nil && kerrors.IsNotFound(err) {
				ltpaKeysCreationScriptConfigMap.Data = make(map[string]string)
				script, err := os.ReadFile("controllers/assets/create_ltpa_keys.sh")
				if err != nil {
					return "", err
				}
				ltpaKeysCreationScriptConfigMap.Data["create_ltpa_keys.sh"] = string(script)
				r.CreateOrUpdate(ltpaKeysCreationScriptConfigMap, instance, func() error {
					return nil
				})
			}

			// Verify the controllers/assets/create_ltpa_keys.sh script has been loaded before starting the LTPA Job
			err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaKeysCreationScriptConfigMap.Name, Namespace: ltpaKeysCreationScriptConfigMap.Namespace}, ltpaKeysCreationScriptConfigMap)
			if err == nil {
				// Run the Kubernetes Job to generate the shared ltpa.keys file and LTPA Secret
				err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: generateLTPAKeysJob.Name, Namespace: generateLTPAKeysJob.Namespace}, generateLTPAKeysJob)
				if err != nil && kerrors.IsNotFound(err) {
					err = r.CreateOrUpdate(generateLTPAKeysJob, instance, func() error {
						ltpaConfig := &lutils.LTPAConfig{
							Metadata:                    ltpaMetadata,
							SecretName:                  ltpaSecretRootName,
							SecretInstanceName:          ltpaSecret.Name,
							ServiceAccountName:          ltpaServiceAccountName,
							ConfigMapName:               ltpaKeysCreationScriptConfigMap.Name,
							FileName:                    lutils.LTPAKeysFileName,
							EncryptionKeySecretName:     OperatorShortName + lutils.PasswordEncryptionKeySuffix,
							EncryptionKeySharingEnabled: r.isUsingPasswordEncryptionKeySharing(instance),
						}
						lutils.CustomizeLTPAJob(generateLTPAKeysJob, instance, ltpaConfig)
						return nil
					})
					if err != nil {
						return "", fmt.Errorf("Failed to create Job %s: %s"+generateLTPAKeysJob.Name, err)
					}
				} else if err == nil {
					// If the LTPA Secret is not yet created (LTPA Job has not successfully completed)
					// and the LTPA Job's configuration is outdated, retry LTPA generation with the new configuration
					if lutils.IsLTPAJobConfigurationOutdated(generateLTPAKeysJob, instance) {
						// Delete the Job request to restart the entire LTPA generation process (i.e. reloading the script, ltpa.xml, and Job)
						err = r.DeleteResource(ltpaJobRequest)
						if err != nil {
							return ltpaSecret.Name, err
						}
					}
				} else {
					return "", fmt.Errorf("Failed to get Job " + generateLTPAKeysJob.Name)
				}
			}
		}

		// Reconcile the Job
		err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: generateLTPAKeysJob.Name, Namespace: generateLTPAKeysJob.Namespace}, generateLTPAKeysJob)
		if err != nil && kerrors.IsNotFound(err) {
			return "", fmt.Errorf("Waiting for the LTPA key to be generated by Job '" + generateLTPAKeysJob.Name + "'.")
		} else if err != nil {
			return "", fmt.Errorf("Failed to get Job " + generateLTPAKeysJob.Name)
		}
		if len(generateLTPAKeysJob.Status.Conditions) > 0 && generateLTPAKeysJob.Status.Conditions[0].Type == v1.JobFailed {
			return "", fmt.Errorf("Job " + generateLTPAKeysJob.Name + " has failed. Manually clean up hung resources by setting .spec.manageLTPA to false in the " + ltpaKeySharingLeaderName + " instance.")
		}
		return "", fmt.Errorf("Waiting for the LTPA key to be generated by Job '" + generateLTPAKeysJob.Name + "'.")
	} else if err != nil {
		return "", err
	} else {
		_, isLTPAKeySharingLeader, _, _, err := r.getLTPAKeysSharingLeader(instance, ltpaMetadata, true)
		if err != nil {
			return "", err
		}
		if !isLTPAKeySharingLeader {
			return ltpaSecret.Name, nil
		}
	}

	// The LTPA Secret is created (in other words, the LTPA Job has completed) so delete the Job request
	err = r.DeleteResource(ltpaJobRequest)
	if err != nil {
		return ltpaSecret.Name, err
	}

	// The Secret to hold the server.xml that will import the LTPA keys into the Liberty server
	// This server.xml will be mounted in /config/configDropins/overrides/ltpaKeysMount.xml
	serverXMLMountSecretErr := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaXMLMountSecret.Name, Namespace: ltpaXMLMountSecret.Namespace}, ltpaXMLMountSecret)
	if serverXMLMountSecretErr != nil {
		if kerrors.IsNotFound(serverXMLMountSecretErr) {
			if err := r.CreateOrUpdate(ltpaXMLMountSecret, nil, func() error {
				mountDir := strings.Replace(lutils.SecureMountPath+"/"+lutils.LTPAKeysXMLFileName, "/output", "${server.output.dir}", 1)
				return lutils.CustomizeLibertyFileMountXML(ltpaXMLMountSecret, lutils.LTPAKeysMountXMLFileName, mountDir)
			}); err != nil {
				return "", err
			}
		} else {
			return "", serverXMLMountSecretErr
		}
	}

	// Create the Liberty Server XML Secret if it doesn't exist
	serverXMLSecretErr := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaXMLSecret.Name, Namespace: ltpaXMLSecret.Namespace}, ltpaXMLSecret)
	if serverXMLSecretErr != nil {
		if kerrors.IsNotFound(serverXMLSecretErr) {
			r.CreateOrUpdate(ltpaXMLSecret, nil, func() error {
				return lutils.CustomizeLTPAServerXML(ltpaXMLSecret, instance, string(ltpaSecret.Data["password"]))
			})
		} else {
			return "", serverXMLSecretErr
		}
	}

	// Validate whether or not password encryption settings match the way LTPA keys were created
	hasConfigurationMismatch := false
	ltpaEncryptionRV, ltpaEncryptionRVFound := ltpaSecret.Data["encryptionSecretResourceVersion"]
	if r.isPasswordEncryptionKeySharingEnabled(instance) {
		if encryptionKeySecret, err := r.hasEncryptionKeySecret(instance); err == nil {
			if !ltpaEncryptionRVFound || string(ltpaEncryptionRV) != encryptionKeySecret.ResourceVersion {
				hasConfigurationMismatch = true // managePasswordEncryption is true, the shared encryption key exists but LTPA keys are either not encrypted or not updated
			}
		} else if kerrors.IsNotFound(err) && ltpaEncryptionRVFound {
			hasConfigurationMismatch = true // managePasswordEncryption is true, the shared encryption key is missing but LTPA keys are still encrypted
		}
	} else if ltpaEncryptionRVFound {
		hasConfigurationMismatch = true // managePasswordEncryption is false but LTPA keys are encrypted
	}

	// Delete the LTPA Secret and depend on the create_ltpa_keys.sh script to add/remove/update the encryptionSecretResourceVersion field
	if hasConfigurationMismatch {
		err = r.DeleteResource(ltpaSecret)
		if err != nil {
			return "", err
		}
	}
	return ltpaSecret.Name, nil
}

func (r *ReconcileOpenLiberty) isLTPAKeySharingEnabled(instance *olv1.OpenLibertyApplication) bool {
	if instance.GetManageLTPA() != nil && *instance.GetManageLTPA() {
		return true
	}
	return false
}

// Deletes resources used to create the LTPA keys file
func (r *ReconcileOpenLiberty) deleteLTPAKeysResources(instance *olv1.OpenLibertyApplication) error {
	leaderTracker, err := r.getLTPALeaderTracker(instance)
	if err != nil {
		// when not found, assume there is nothing to delete because no LTPA secrets are being tracked
		if kerrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	resourceOwners, found := leaderTracker.Data[lutils.ResourceOwnersKey]
	if !found {
		return fmt.Errorf("could not get LTPA leader tracker state (%s) for deletion", lutils.ResourceOwnersKey)
	}
	if i := tree.CommaSeparatedStringContains(resourceOwners, instance.Name); i != -1 {
		resourceNames, found := leaderTracker.Data[lutils.ResourcesKey]
		if !found {
			return fmt.Errorf("could not get LTPA leader tracker state (%s) for deletion", lutils.ResourcesKey)
		}
		ownerNameSuffix, err := tree.GetCommaSeparatedString(resourceNames, i)
		if err != nil {
			return err
		}

		generateLTPAKeysJob := &v1.Job{}
		generateLTPAKeysJob.Name = OperatorShortName + "-managed-ltpa-keys-generation" + ownerNameSuffix
		generateLTPAKeysJob.Namespace = instance.GetNamespace()
		deletePropagationBackground := metav1.DeletePropagationBackground
		err = r.GetClient().Delete(context.TODO(), generateLTPAKeysJob, &client.DeleteOptions{PropagationPolicy: &deletePropagationBackground})
		if err != nil && !kerrors.IsNotFound(err) {
			return err
		}

		ltpaKeysCreationScriptConfigMap := &corev1.ConfigMap{}
		ltpaKeysCreationScriptConfigMap.Name = OperatorShortName + "-managed-ltpa-script" + ownerNameSuffix
		ltpaKeysCreationScriptConfigMap.Namespace = instance.GetNamespace()
		err = r.DeleteResource(ltpaKeysCreationScriptConfigMap)
		if err != nil {
			return err
		}

		ltpaJobRequest := &corev1.ConfigMap{}
		ltpaJobRequest.Name = OperatorShortName + "-managed-ltpa-job-request" + ownerNameSuffix
		ltpaJobRequest.Namespace = instance.GetNamespace()
		err = r.DeleteResource(ltpaJobRequest)
		if err != nil {
			return err
		}
	}

	ltpaServiceAccount := &corev1.ServiceAccount{}
	ltpaServiceAccount.Name = OperatorShortName + "-ltpa"
	ltpaServiceAccount.Namespace = instance.GetNamespace()
	hasNoOwners, err := r.DeleteResourceWithLeaderTrackingLabels(ltpaServiceAccount, instance)
	if err != nil {
		return err
	}

	if hasNoOwners {
		ltpaRoleBinding := &rbacv1.RoleBinding{}
		ltpaRoleBinding.Name = OperatorShortName + "-managed-ltpa-rolebinding"
		ltpaRoleBinding.Namespace = instance.GetNamespace()
		err = r.DeleteResource(ltpaRoleBinding)
		if err != nil {
			return err
		}

		ltpaRole := &rbacv1.Role{}
		ltpaRole.Name = OperatorShortName + "-managed-ltpa-role"
		ltpaRole.Namespace = instance.GetNamespace()
		err = r.DeleteResource(ltpaRole)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileOpenLiberty) CustomizeLTPALeaderTracker(leaderTracker *corev1.ConfigMap, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string) error {
	// create the ConfigMap
	leaderTracker.Labels[lutils.LTPAVersionLabel] = latestOperandVersion
	leaderTracker.Data = make(map[string]string)
	leaderTracker.Data[lutils.ResourcesKey] = ""
	leaderTracker.Data[lutils.ResourceOwnersKey] = ""
	leaderTracker.Data[lutils.ResourcePathsKey] = ""
	leaderTracker.Data[lutils.ResourcePathIndicesKey] = ""
	leaderTracker.ResourceVersion = ""

	// initialize the leader tracker from existing LTPA Secrets
	ltpaSecrets := &corev1.SecretList{}
	ltpaRootName := OperatorShortName + "-managed-ltpa"
	if err := r.GetClient().List(context.TODO(), ltpaSecrets, client.MatchingLabels{
		"app.kubernetes.io/name": ltpaRootName,
	}); err != nil {
		return err
	}
	// only track LTPA Secrets with path indices
	n := 0
	for _, secret := range ltpaSecrets.Items {
		if _, found := secret.Labels[lutils.LTPAPathIndexLabel]; found {
			n += 1
		}
	}
	if n > 0 {
		resources := make([]string, n)
		resourceOwners := make([]string, n)
		resourcePaths := make([]string, n)
		resourcePathIndices := make([]string, n)
		k := 0
		for i, secret := range ltpaSecrets.Items {
			if val, found := secret.Labels[lutils.LTPAPathIndexLabel]; found {
				resourcePathIndices[k] = val
				// TODO: assert val contains a "." or skip this element
				labelVersionArray := strings.Split(val, ".")
				// TODO: assert labelVersionArray is 2 elements, or skip this element
				intVal, _ := strconv.ParseInt(labelVersionArray[1], 10, 64)
				path, pathErr := tree.GetPathFromLeafIndex(treeMap, labelVersionArray[0], int(intVal))
				// If path comes from a different operand version, the path needs to be upgraded/downgraded to the latestOperandVersion
				if labelVersionArray[0] != latestOperandVersion {
					// If user error has occurred, based on whether or not a dev deleted the decision tree structure of an older version
					// we must allow this process to error so that a deleted (older) revision of the decision tree that may be missing
					// won't halt the operator when ReplacePath does a validation check
					if path, err := tree.ReplacePath(path, latestOperandVersion, treeMap, replaceMap); err == nil {
						newPathIndex := strings.Split(path, ".")[0] + "." + strconv.FormatInt(int64(tree.GetLeafIndex(treeMap, path)), 10)
						resourcePathIndices[k] = newPathIndex
						resourcePaths[k] = path
						// the path may have changed so the path index reference needs to be updated directly in the LTPA Secret
						if err := r.CreateOrUpdate(&ltpaSecrets.Items[i], nil, func() error {
							ltpaSecrets.Items[i].Labels[lutils.LTPAPathIndexLabel] = newPathIndex
							return nil
						}); err != nil {
							return err
						}
					}
				} else if pathErr == nil { // only update the path metadata if this operator's decision tree structure recognizes the LTPA Secret found
					resourcePaths[k] = path
				}
				resources[k] = secret.Name[len(ltpaRootName):]
				k += 1
			}

		}
		leaderTracker.Data[lutils.ResourcesKey] = strings.Join(resources, ",")
		leaderTracker.Data[lutils.ResourceOwnersKey] = strings.Join(resourceOwners, ",")
		leaderTracker.Data[lutils.ResourcePathsKey] = strings.Join(resourcePaths, ",")
		leaderTracker.Data[lutils.ResourcePathIndicesKey] = strings.Join(resourcePathIndices, ",")
	}
	return nil
}

// Initializes a ConfigMap used to track LTPA Secrets' state
func (r *ReconcileOpenLiberty) initializeLTPALeaderTracker(instance *olv1.OpenLibertyApplication, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string) error {
	if leaderTracker, err := r.getLTPALeaderTracker(instance); err != nil && kerrors.IsNotFound(err) {
		if err := r.CreateOrUpdate(leaderTracker, nil, func() error {
			return r.CustomizeLTPALeaderTracker(leaderTracker, treeMap, replaceMap, latestOperandVersion)
		}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// if the ConfigMap is outdated, delete it so that it gets recreated in another reconcile
		if leaderTracker.Labels[lutils.LTPAVersionLabel] != latestOperandVersion {
			if err := r.DeleteResource(leaderTracker); err != nil {
				return err
			}
		}
	}
	return nil
}

// Gets the LTPA Leader Tracker ConfigMap or errors if it doesn't exist
func (r *ReconcileOpenLiberty) getLTPALeaderTracker(instance *olv1.OpenLibertyApplication) (*corev1.ConfigMap, error) {
	leaderTracker := &corev1.ConfigMap{}
	leaderTracker.Name = OperatorShortName + "-managed-leader-tracking-ltpa"
	leaderTracker.Namespace = instance.GetNamespace()
	leaderTracker.Labels = lutils.GetRequiredLabels(leaderTracker.Name, "")
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: leaderTracker.Name, Namespace: leaderTracker.Namespace}, leaderTracker)
	return leaderTracker, err
}

// Create or update the LTPA service account and track the LTPA state
func (r *ReconcileOpenLiberty) CreateOrUpdateWithLeaderTrackingLabels(sa *corev1.ServiceAccount, instance *olv1.OpenLibertyApplication, ltpaMetadata *lutils.LTPAMetadata, createOrUpdateObject bool) (string, bool, string, error) {
	// Create the ServiceAccount
	if createOrUpdateObject {
		r.CreateOrUpdate(sa, instance, func() error {
			return nil
		})
	}

	leaderTracker, err := r.getLTPALeaderTracker(instance)
	if err != nil {
		return "", false, "", err
	}

	initialLeaderIndex := -1
	resourcesLabel, resourcesLabelFound := leaderTracker.Data[lutils.ResourcesKey]
	if resourcesLabelFound && resourcesLabel != "" {
		initialLeaderIndex = tree.CommaSeparatedStringContains(string(resourcesLabel), ltpaMetadata.NameSuffix)
	}
	resourceOwnersLabel, resourceOwnersLabelFound := leaderTracker.Data[lutils.ResourceOwnersKey]
	// if ltpaNameSuffix does not exist in resources labels or resource labels don't exist so this instance is leader
	if initialLeaderIndex == -1 {
		if !createOrUpdateObject {
			return "", false, "", nil
		}
		if err := r.CreateOrUpdate(leaderTracker, nil, func() error {
			if resourcesLabelFound && resourcesLabel != "" {
				leaderTracker.Data[lutils.ResourcesKey] += "," + ltpaMetadata.NameSuffix
				leaderTracker.Data[lutils.ResourcePathsKey] += "," + ltpaMetadata.Path
				leaderTracker.Data[lutils.ResourcePathIndicesKey] += "," + ltpaMetadata.PathIndex
				if resourceOwnersLabelFound {
					resourceOwners := strings.Split(resourceOwnersLabel, ",")
					n := len(resourceOwners)
					newResourceOwners := make([]string, n+1)
					for i, owner := range resourceOwners {
						if owner == instance.Name { // if this instance was a previous leader, remove the reference
							newResourceOwners[i] = ""
						} else {
							newResourceOwners[i] = owner
						}
					}
					newResourceOwners[n] = instance.Name // Track this instance as leader
					leaderTracker.Data[lutils.ResourceOwnersKey] = strings.Join(newResourceOwners, ",")
				} else {
					// something went wrong
					return fmt.Errorf("something went wrong, elem_len(LTPAResourceOwnersLabel) != elem_len(LTPAResourcesLabel)")
				}
			} else {
				leaderTracker.Data[lutils.ResourcesKey] = ltpaMetadata.NameSuffix
				leaderTracker.Data[lutils.ResourceOwnersKey] = instance.Name
				leaderTracker.Data[lutils.ResourcePathsKey] = ltpaMetadata.Path
				leaderTracker.Data[lutils.ResourcePathIndicesKey] = ltpaMetadata.PathIndex
			}
			return nil
		}); err != nil {
			if deleteErr := r.DeleteResource(leaderTracker); deleteErr != nil {
				return "", false, "", deleteErr
			}
			return "", false, "", err
		}
		return instance.Name, true, ltpaMetadata.PathIndex, nil
	}
	// otherwise, ltpaNameSuffix is being tracked
	// if the leader of ltpaNameSuffix is non empty return that leader
	if resourceOwnersLabelFound {
		resourceOwners := strings.Split(resourceOwnersLabel, ",")
		pathIndices := strings.Split(leaderTracker.Data[lutils.ResourcePathIndicesKey], ",")
		if initialLeaderIndex < len(resourceOwners) {
			candidateLeader := resourceOwners[initialLeaderIndex]
			if len(candidateLeader) > 0 {
				// Return this other instance as the leader (the "other" instance could also be this instance)
				// Before returning, if the candidate instance is not this instance, this instance must clean up its old owner references to avoid an resource owner cycle.
				// A resource owner cycle can occur when instance A points to resource A and instance B points to resource B but then both instance A and B swap pointing to each other's resource.
				if candidateLeader != instance.Name {
					// create a new array of resource owners without any references to instance.Name
					newResourceOwners := make([]string, len(resourceOwners))
					for i, owner := range resourceOwners {
						if owner == instance.Name { // if this instance was a previous leader, remove the reference
							newResourceOwners[i] = ""
						} else {
							newResourceOwners[i] = owner
						}
					}
					// save this new owner list
					r.CreateOrUpdate(leaderTracker, nil, func() error {
						leaderTracker.Data[lutils.ResourceOwnersKey] = strings.Join(newResourceOwners, ",")
						return nil
					})
				}
				return candidateLeader, candidateLeader == instance.Name, pathIndices[initialLeaderIndex], nil
			} else {
				if !createOrUpdateObject {
					return "", false, "", nil
				}
				// there is either no leader (empty string) or the leader was deleted so now this instance is leader
				pathIndex := ""
				newResourceOwners := make([]string, len(resourceOwners))
				for i, owner := range resourceOwners {
					if owner == instance.Name { // if this instance was a previous leader, remove the reference
						newResourceOwners[i] = ""
					} else if i == initialLeaderIndex {
						newResourceOwners[i] = instance.Name // Track this instance as leader
						pathIndex = pathIndices[i]
					} else {
						newResourceOwners[i] = owner
					}
				}
				// save this new owner list
				r.CreateOrUpdate(leaderTracker, nil, func() error {
					leaderTracker.Data[lutils.ResourceOwnersKey] = strings.Join(newResourceOwners, ",")
					return nil
				})
				return instance.Name, true, pathIndex, nil
			}
		}
		// else { // something went wrong, elem_len(LTPAResourceOwnersLabel) != elem_len(LTPAResourcesLabel) }
	}
	// else { // something went wrong, elem_len(LTPAResourcesLabel) != 0 }
	// the leader tracking labels are out of sync, delete the leader tracker to allow the operator to recreate it in the correct format
	if err := r.DeleteResource(leaderTracker); err != nil {
		return "", false, "", err
	}
	return "", false, "", fmt.Errorf("leader tracking labels are out of sync")
}

// Removes the instance owner reference and references in leader tracking labels
// Precondition: instance must be the resource leader
func (r *ReconcileOpenLiberty) DeleteResourceWithLeaderTrackingLabels(sa *corev1.ServiceAccount, instance *olv1.OpenLibertyApplication) (bool, error) {
	leaderTracker, err := r.getLTPALeaderTracker(instance)
	if err != nil {
		return false, err
	}
	hasNoOwners := false
	r.CreateOrUpdate(leaderTracker, nil, func() error {
		// If the instance is being tracked, remove it
		resourceOwnersLabel, resourceOwnersLabelFound := leaderTracker.Data[lutils.ResourceOwnersKey]
		if resourceOwnersLabelFound {
			if strings.Contains(resourceOwnersLabel, ",") {
				owners := strings.Split(resourceOwnersLabel, ",")
				newOwners := make([]string, len(owners))
				for i, owner := range owners {
					if owner != instance.Name {
						newOwners[i] = owner
					} else {
						newOwners[i] = ""
					}
				}
				leaderTracker.Data[lutils.ResourceOwnersKey] = strings.Join(newOwners, ",")
				hasNoOwners = len(leaderTracker.Data[lutils.ResourceOwnersKey]) == len(newOwners)-1
			} else if resourceOwnersLabel == instance.Name || resourceOwnersLabel == "" {
				leaderTracker.Data[lutils.ResourceOwnersKey] = ""
				hasNoOwners = true
			}
		} else {
			hasNoOwners = true
		}
		return nil
	})
	if hasNoOwners {
		err = r.DeleteResource(sa)
	}
	return hasNoOwners, err
}

// Returns true if the OpenLibertyApplication instance initiated the LTPA keys sharing process or sets the instance as the leader if the LTPA keys are not yet shared
func (r *ReconcileOpenLiberty) getLTPAKeysSharingLeader(instance *olv1.OpenLibertyApplication, ltpaMetadata *lutils.LTPAMetadata, createOrUpdateServiceAccount bool) (string, bool, string, string, error) {
	ltpaServiceAccount := &corev1.ServiceAccount{}
	ltpaServiceAccount.Name = OperatorShortName + "-ltpa"
	ltpaServiceAccount.Namespace = instance.GetNamespace()
	ltpaServiceAccount.Labels = lutils.GetRequiredLabels(ltpaServiceAccount.Name, "")
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaServiceAccount.Name, Namespace: ltpaServiceAccount.Namespace}, ltpaServiceAccount)
	if err != nil && kerrors.IsNotFound(err) {
		if !createOrUpdateServiceAccount {
			return "", false, "", "", nil
		}
	} else if err != nil {
		return "", false, ltpaServiceAccount.Name, "", err
	}
	// Service account is found, but a new owner could be added or one already exists
	leaderName, thisInstanceIsLeader, secretRef, err := r.CreateOrUpdateWithLeaderTrackingLabels(ltpaServiceAccount, instance, ltpaMetadata, createOrUpdateServiceAccount)
	return leaderName, thisInstanceIsLeader, ltpaServiceAccount.Name, secretRef, err
}
