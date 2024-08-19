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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const LTPA_RESOURCE_SHARING_FILE_NAME = "ltpa"

func (r *ReconcileOpenLiberty) reconcileLTPAMetadata(instance *olv1.OpenLibertyApplication, treeMap map[string]interface{}, latestOperandVersion string, assetsFolder *string) (*lutils.LTPAMetadata, error) {
	metadata := &lutils.LTPAMetadata{}
	// During runtime, the OpenLibertyApplication instance will decide what LTPA Secret to track by populating array pathChoices
	pathOptions, pathChoices := r.getLTPAPathOptionsAndChoices(instance, latestOperandVersion)

	// convert the path options and choices into a labelString, for a path of length n, the labelString is
	// constructed as a weaved array in format "<pathOptions[0]>.<pathChoices[0]>.<pathOptions[1]>.<pathChoices[1]>...<pathOptions[n-1]>.<pathChoices[n-1]>"
	labelString, err := tree.GetLabelFromDecisionPath(latestOperandVersion, pathOptions, pathChoices)
	if err != nil {
		return metadata, err
	}
	// validate that the decision path such as "v1_4_0.managePasswordEncryption.<pathChoices[n-1]>" is a valid subpath in treeMap
	// an error here indicates a build time error created by the operator developer or pollution of the ltpa-decision-tree.yaml
	// Note: validSubPath is a substring of labelString and a valid path within treeMap; it will always hold that len(validSubPath) <= len(labelString)
	validSubPath, err := tree.CanTraverseTree(treeMap, labelString, true)
	if err != nil {
		return metadata, err
	}
	// retrieve the LTPA leader tracker to re-use an existing name or to create a new metadata.Name
	leaderTracker, _, err := lutils.GetLeaderTracker(instance, OperatorShortName, LTPA_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	if err != nil {
		return metadata, err
	}
	// if the leaderTracker is on a mismatched version, wait for a subsequent reconcile loop to re-create the leader tracker
	if leaderTracker.Labels[lutils.LeaderVersionLabel] != latestOperandVersion {
		return metadata, fmt.Errorf("waiting for the Leader Tracker to be updated")
	}

	// to avoid limitation with Kubernetes label values having a max length of 63, translate validSubPath into a path index
	pathIndex := tree.GetLeafIndex(treeMap, validSubPath)
	versionedPathIndex := fmt.Sprintf("%s.%d", latestOperandVersion, pathIndex)

	metadata.Path = validSubPath
	metadata.PathIndex = versionedPathIndex
	metadata.Name = r.getLTPAMetadataName(instance, leaderTracker, validSubPath, assetsFolder)
	return metadata, nil
}

func (r *ReconcileOpenLiberty) getLTPAPathOptionsAndChoices(instance *olv1.OpenLibertyApplication, latestOperandVersion string) ([]string, []string) {
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
	return pathOptions, pathChoices
}

func (r *ReconcileOpenLiberty) getLTPAMetadataName(instance *olv1.OpenLibertyApplication, leaderTracker *corev1.Secret, validSubPath string, assetsFolder *string) string {
	// if an existing resource name (suffix) for this key combination already exists, use it
	loc := lutils.CommaSeparatedStringContains(string(leaderTracker.Data[lutils.ResourcePathsKey]), validSubPath)
	if loc != -1 {
		suffix, _ := lutils.GetCommaSeparatedString(string(leaderTracker.Data[lutils.ResourcesKey]), loc)
		return suffix
	}

	// For example, if the env variable LTPA_RESOURCE_SUFFIXES is set,
	// it can provide a comma separated string of length lutils.ResourceSuffixLength suffixes to exhaust
	//
	// spec:
	//   env:
	//     - name: LTPA_RESOURCE_SUFFIXES
	//       value: "aaaaa,bbbbb,ccccc,zzzzz,a1b2c"
	if predeterminedSuffixes, hasEnv := hasLTPAResourceSuffixesEnv(instance); hasEnv {
		predeterminedSuffixesArray := lutils.GetCommaSeparatedArray(predeterminedSuffixes)
		for _, suffix := range predeterminedSuffixesArray {
			if len(suffix) == lutils.ResourceSuffixLength && lutils.IsLowerAlphanumericSuffix(suffix) && !strings.Contains(string(leaderTracker.Data[lutils.ResourcesKey]), suffix) {
				return "-" + suffix
			}
		}
	}

	// otherwise, generate a random suffix of length lutils.ResourceSuffixLength
	randomSuffix := lutils.GetRandomLowerAlphanumericSuffix(lutils.ResourceSuffixLength)
	suffixFoundInCluster := true // MUST check that the operator is not overriding another instance's untracked shared resource
	for strings.Contains(string(leaderTracker.Data[lutils.ResourcesKey]), randomSuffix) || suffixFoundInCluster {
		randomSuffix = lutils.GetRandomLowerAlphanumericSuffix(lutils.ResourceSuffixLength)
		// create the unstructured object; parse and obtain the sharedResourceName via the controllers/assets/ltpa-signature.yaml
		if sharedResource, sharedResourceName, err := lutils.CreateUnstructuredResourceFromSignature(LTPA_RESOURCE_SHARING_FILE_NAME, assetsFolder, OperatorShortName, randomSuffix); err == nil {
			err := r.GetClient().Get(context.TODO(), types.NamespacedName{Namespace: instance.GetNamespace(), Name: sharedResourceName}, sharedResource)
			if err != nil && kerrors.IsNotFound(err) {
				suffixFoundInCluster = false
			}
		}
	}
	return randomSuffix
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
func (r *ReconcileOpenLiberty) reconcileLTPAKeys(instance *olv1.OpenLibertyApplication, ltpaMetadata *lutils.LTPAMetadata) (string, string, error) {
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
	_, thisInstanceIsLeader, _, err := r.reconcileLeader(instance, ltpaMetadata, LTPA_RESOURCE_SHARING_FILE_NAME, false)
	if err != nil {
		return err
	}
	if thisInstanceIsLeader {
		ltpaSecret := &corev1.Secret{}
		ltpaSecret.Name = OperatorShortName + "-managed-ltpa" + ltpaMetadata.Name
		ltpaSecret.Namespace = instance.GetNamespace()
		err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaSecret.Name, Namespace: ltpaSecret.Namespace}, ltpaSecret)
		if err != nil && kerrors.IsNotFound(err) {
			// Deleting the job request removes existing LTPA resources and restarts the LTPA generation process
			ltpaJobRequest := &corev1.ConfigMap{}
			ltpaJobRequest.Name = OperatorShortName + "-managed-ltpa-job-request" + ltpaMetadata.Name
			ltpaJobRequest.Namespace = instance.GetNamespace()
			err = r.DeleteResource(ltpaJobRequest)
			if err != nil {
				return err
			}

			ltpaServiceAccount := &corev1.ServiceAccount{}
			ltpaServiceAccountRootName := OperatorShortName + "-ltpa"
			ltpaServiceAccount.Name = ltpaServiceAccountRootName + ltpaMetadata.Name
			err = r.DeleteResource(ltpaServiceAccount)
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
	ltpaXMLSecret.Name = ltpaXMLSecretRootName + ltpaMetadata.Name
	ltpaXMLSecret.Namespace = instance.GetNamespace()
	ltpaXMLSecret.Labels = lutils.GetRequiredLabels(ltpaXMLSecretRootName, ltpaXMLSecret.Name)

	ltpaXMLMountSecret := &corev1.Secret{}
	ltpaXMLMountSecretRootName := OperatorShortName + lutils.LTPAServerXMLMountSuffix
	ltpaXMLMountSecret.Name = ltpaXMLMountSecretRootName + ltpaMetadata.Name
	ltpaXMLMountSecret.Namespace = instance.GetNamespace()
	ltpaXMLMountSecret.Labels = lutils.GetRequiredLabels(ltpaXMLMountSecretRootName, ltpaXMLSecret.Name)

	generateLTPAKeysJob := &v1.Job{}
	generateLTPAKeysJobRootName := OperatorShortName + "-managed-ltpa-keys-generation"
	generateLTPAKeysJob.Name = generateLTPAKeysJobRootName + ltpaMetadata.Name
	generateLTPAKeysJob.Namespace = instance.GetNamespace()
	generateLTPAKeysJob.Labels = lutils.GetRequiredLabels(generateLTPAKeysJobRootName, generateLTPAKeysJob.Name)

	deletePropagationBackground := metav1.DeletePropagationBackground

	ltpaJobRequest := &corev1.ConfigMap{}
	ltpaJobRequestRootName := OperatorShortName + "-managed-ltpa-job-request"
	ltpaJobRequest.Name = ltpaJobRequestRootName + ltpaMetadata.Name
	ltpaJobRequest.Namespace = instance.GetNamespace()
	ltpaJobRequest.Labels = lutils.GetRequiredLabels(ltpaJobRequestRootName, ltpaJobRequest.Name)

	ltpaKeysCreationScriptConfigMap := &corev1.ConfigMap{}
	ltpaKeysCreationScriptConfigMapRootName := OperatorShortName + "-managed-ltpa-script"
	ltpaKeysCreationScriptConfigMap.Name = ltpaKeysCreationScriptConfigMapRootName + ltpaMetadata.Name
	ltpaKeysCreationScriptConfigMap.Namespace = instance.GetNamespace()
	ltpaKeysCreationScriptConfigMap.Labels = lutils.GetRequiredLabels(ltpaKeysCreationScriptConfigMapRootName, ltpaKeysCreationScriptConfigMap.Name)

	ltpaServiceAccount := &corev1.ServiceAccount{}
	ltpaServiceAccountRootName := OperatorShortName + "-ltpa"
	ltpaServiceAccount.Name = ltpaServiceAccountRootName + ltpaMetadata.Name
	ltpaServiceAccount.Namespace = instance.GetNamespace()
	ltpaServiceAccount.Labels = lutils.GetRequiredLabels(ltpaServiceAccountRootName, ltpaServiceAccount.Name)

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
			return "", err
		}
		// If this instance is not the leader, exit the reconcile loop
		if !thisInstanceIsLeader {
			return "", fmt.Errorf("Waiting for OpenLibertyApplication instance '" + leaderName + "' to generate the shared LTPA keys file for the namespace '" + instance.Namespace + "'.")
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
				err := r.CreateOrUpdate(ltpaJobRequest, nil, func() error {
					return nil
				})
				if err != nil {
					return "", fmt.Errorf("Failed to create ConfigMap " + ltpaJobRequest.Name)
				}
			} else {
				return "", fmt.Errorf("Failed to get ConfigMap " + ltpaJobRequest.Name)
			}
		} else {
			// Create the ServiceAccount
			r.CreateOrUpdate(ltpaServiceAccount, nil, func() error {
				return nil
			})

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
			r.CreateOrUpdate(ltpaRole, ltpaServiceAccount, func() error {
				return nil
			})

			ltpaRoleBinding := &rbacv1.RoleBinding{}
			ltpaRoleBinding.Name = OperatorShortName + "-managed-ltpa-rolebinding"
			ltpaRoleBinding.Namespace = instance.GetNamespace()
			ltpaRoleBinding.Subjects = []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      ltpaServiceAccount.Name,
					Namespace: instance.GetNamespace(),
				},
			}
			ltpaRoleBinding.RoleRef = rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     ltpaRole.Name,
			}
			ltpaRoleBinding.Labels = lutils.GetRequiredLabels(ltpaRoleBinding.Name, "")
			r.CreateOrUpdate(ltpaRoleBinding, ltpaServiceAccount, func() error {
				return nil
			})

			// Create a ConfigMap to store the controllers/assets/create_ltpa_keys.sh script
			err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaKeysCreationScriptConfigMap.Name, Namespace: ltpaKeysCreationScriptConfigMap.Namespace}, ltpaKeysCreationScriptConfigMap)
			if err != nil && kerrors.IsNotFound(err) {
				ltpaKeysCreationScriptConfigMap.Data = make(map[string]string)
				script, err := os.ReadFile("controllers/assets/" + lutils.LTPAKeysCreationScriptFileName)
				if err != nil {
					return "", err
				}
				ltpaKeysCreationScriptConfigMap.Data[lutils.LTPAKeysCreationScriptFileName] = string(script)
				// prevent script from being modified
				trueBool := true
				ltpaKeysCreationScriptConfigMap.Immutable = &trueBool
				r.CreateOrUpdate(ltpaKeysCreationScriptConfigMap, nil, func() error {
					return nil
				})
			}

			// Verify the controllers/assets/create_ltpa_keys.sh script has been loaded before starting the LTPA Job
			err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaKeysCreationScriptConfigMap.Name, Namespace: ltpaKeysCreationScriptConfigMap.Namespace}, ltpaKeysCreationScriptConfigMap)
			if err == nil {
				// Compare the bundle script against the ltpaKeysCreationScriptConfigMap's saved script
				script, err := os.ReadFile("controllers/assets/" + lutils.LTPAKeysCreationScriptFileName)
				if err != nil {
					return "", err
				}
				savedScript, found := ltpaKeysCreationScriptConfigMap.Data[lutils.LTPAKeysCreationScriptFileName]
				// Delete ltpaKeysCreationScriptConfigMap if it is missing the data key
				if !found {
					if err := r.DeleteResource(ltpaKeysCreationScriptConfigMap); err != nil {
						return "", err
					}
					return "", fmt.Errorf("the LTPA Keys Creation ConfigMap is missing key " + lutils.LTPAKeysCreationScriptFileName)
				}
				// Delete ltpaKeysCreationScriptConfigMap if the file contents do not match
				if string(script) != savedScript {
					if err := r.DeleteResource(ltpaKeysCreationScriptConfigMap); err != nil {
						return "", err
					}
					return "", fmt.Errorf("the LTPA Keys Creation ConfigMap key '" + lutils.LTPAKeysCreationScriptFileName + "' is out of sync")
				}
				// Run the Kubernetes Job to generate the shared ltpa.keys file and LTPA Secret
				err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: generateLTPAKeysJob.Name, Namespace: generateLTPAKeysJob.Namespace}, generateLTPAKeysJob)
				if err != nil && kerrors.IsNotFound(err) {
					err = r.CreateOrUpdate(generateLTPAKeysJob, nil, func() error {
						ltpaConfig := &lutils.LTPAConfig{
							Metadata:                    ltpaMetadata,
							SecretName:                  ltpaSecretRootName,
							SecretInstanceName:          ltpaSecret.Name,
							ServiceAccountName:          ltpaServiceAccount.Name,
							ConfigMapName:               ltpaKeysCreationScriptConfigMap.Name,
							JobRequestConfigMapName:     ltpaJobRequest.Name,
							FileName:                    lutils.LTPAKeysFileName,
							EncryptionKeySecretName:     OperatorShortName + lutils.PasswordEncryptionKeySuffix,
							EncryptionKeySharingEnabled: r.isUsingPasswordEncryptionKeySharing(instance),
						}
						lutils.CustomizeLTPAJob(generateLTPAKeysJob, instance, ltpaConfig, r.GetClient())
						return nil
					})
					if err != nil {
						return "", fmt.Errorf("Failed to create Job %s: %s"+generateLTPAKeysJob.Name, err)
					}
				} else if err == nil {
					// If the LTPA Secret is not yet created (LTPA Job has not successfully completed)
					// and the LTPA Job's configuration is outdated, retry LTPA generation with the new configuration
					if lutils.IsLTPAJobConfigurationOutdated(generateLTPAKeysJob, instance, r.GetClient()) {
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
			return "", fmt.Errorf("Job " + generateLTPAKeysJob.Name + " has failed. Manually clean up hung resources by setting .spec.manageLTPA to false in the " + leaderName + " instance.")
		}
		return "", fmt.Errorf("Waiting for the LTPA key to be generated by Job '" + generateLTPAKeysJob.Name + "'.")
	} else if err != nil {
		return "", err
	} else {
		_, thisInstanceIsLeader, _, err := r.reconcileLeader(instance, ltpaMetadata, LTPA_RESOURCE_SHARING_FILE_NAME, true)
		if err != nil {
			return "", err
		}
		if !thisInstanceIsLeader {
			return ltpaSecret.Name, nil
		}
	}

	// The LTPA Secret is created (in other words, the LTPA Job has completed) so delete the Job request
	err = r.DeleteResource(ltpaJobRequest)
	if err != nil {
		return ltpaSecret.Name, err
	}
	// The LTPA Secret is created so delete the ServiceAccount
	err = r.DeleteResource(ltpaServiceAccount)
	if err != nil {
		return ltpaSecret.Name, err
	}

	// Set LTPA Secret as owner of LTPA Job
	err = r.CreateOrUpdate(generateLTPAKeysJob, nil, func() error {
		controllerutil.SetControllerReference(ltpaSecret, generateLTPAKeysJob, r.GetClient().Scheme())
		return nil
	})
	if err != nil {
		return ltpaSecret.Name, err
	}
	// Set LTPA Secret as owner of LTPA script
	err = r.CreateOrUpdate(ltpaKeysCreationScriptConfigMap, nil, func() error {
		controllerutil.SetControllerReference(ltpaSecret, ltpaKeysCreationScriptConfigMap, r.GetClient().Scheme())
		return nil
	})
	if err != nil {
		return ltpaSecret.Name, err
	}

	// The Secret to hold the server.xml that will import the LTPA keys into the Liberty server
	// This server.xml will be mounted in /config/configDropins/overrides/ltpaKeysMount.xml
	serverXMLMountSecretErr := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaXMLMountSecret.Name, Namespace: ltpaXMLMountSecret.Namespace}, ltpaXMLMountSecret)
	if serverXMLMountSecretErr != nil {
		if kerrors.IsNotFound(serverXMLMountSecretErr) {
			if err := r.CreateOrUpdate(ltpaXMLMountSecret, ltpaSecret, func() error {
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
			r.CreateOrUpdate(ltpaXMLSecret, ltpaSecret, func() error {
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
	leaderTracker, leaderTrackers, err := lutils.GetLeaderTracker(instance, OperatorShortName, LTPA_RESOURCE_SHARING_FILE_NAME, r.GetClient())
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return r.RemoveLeader(instance, leaderTracker, leaderTrackers)
}

// Search the cluster namespace for existing LTPA Secrets
func (r *ReconcileOpenLiberty) GetLTPAResources(instance *olv1.OpenLibertyApplication, treeMap map[string]interface{}, replaceMap map[string]map[string]string, latestOperandVersion string, assetsFolder *string) (*unstructured.UnstructuredList, string, error) {
	ltpaResourceList, err := lutils.CreateUnstructuredResourceListFromSignature(LTPA_RESOURCE_SHARING_FILE_NAME, assetsFolder, OperatorShortName)
	if err != nil {
		return nil, "", err
	}
	ltpaRootName := OperatorShortName + "-managed-ltpa"
	if err := r.GetClient().List(context.TODO(), ltpaResourceList, client.MatchingLabels{
		"app.kubernetes.io/name": ltpaRootName,
	}, client.InNamespace(instance.GetNamespace())); err != nil {
		return nil, "", err
	}
	return ltpaResourceList, ltpaRootName, nil
}
