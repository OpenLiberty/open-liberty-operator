/*
  Copyright contributors to the WASdev project.

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"io/ioutil"

	v1 "k8s.io/api/batch/v1"

	lutils "github.com/OpenLiberty/open-liberty-operator/utils"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Create the Deployment and Service objects for a Semeru Compiler used by a Websphere Liberty Application
func (r *ReconcileOpenLiberty) reconcileLTPAKeysSharing(instance *olv1.OpenLibertyApplication) (error, string, string) {
	var ltpaSecretName string
	var err error
	if r.isLTPAKeySharingEnabled(instance) {
		err, ltpaSecretName = r.generateLTPAKeys(instance)
		if err != nil {
			return err, "Failed to generate the shared LTPA Keys file", ltpaSecretName
		}
	} else {
		err := r.deleteLTPAKeysResources(instance)
		if err != nil {
			return err, "Failed to delete LTPA Keys Resource", ltpaSecretName
		}
	}
	return nil, "", ltpaSecretName
}

// Returns true if the OpenLibertyApplication instance initiated the LTPA keys sharing process or sets the instance as the leader if the LTPA keys are not yet shared
func (r *ReconcileOpenLiberty) getLTPAKeysSharingLeader(instance *olv1.OpenLibertyApplication, createServiceAccount bool) (error, string, bool, string) {
	ltpaServiceAccount := &corev1.ServiceAccount{}
	ltpaServiceAccount.Name = OperatorShortName + "-ltpa"
	ltpaServiceAccount.Namespace = instance.GetNamespace()
	ltpaServiceAccount.Labels = lutils.GetRequiredLabels(ltpaServiceAccount.Name, "")
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaServiceAccount.Name, Namespace: ltpaServiceAccount.Namespace}, ltpaServiceAccount)
	if err != nil {
		if kerrors.IsNotFound(err) {
			if createServiceAccount {
				r.CreateOrUpdate(ltpaServiceAccount, instance, func() error {
					return nil
				})
				return nil, instance.Name, true, ltpaServiceAccount.Name
			}
			return nil, "", false, ""
		}
		return err, "", false, ltpaServiceAccount.Name
	}
	ltpaKeySharingLeaderName := ""
	for _, ownerReference := range ltpaServiceAccount.OwnerReferences {
		if ownerReference.Name == instance.Name {
			return nil, instance.Name, true, ltpaServiceAccount.Name
		}
		ltpaKeySharingLeaderName = ownerReference.Name
	}
	return nil, ltpaKeySharingLeaderName, false, ltpaServiceAccount.Name
}

// If the LTPA Secret is being created but does not exist yet, the LTPA instance leader will halt the process and restart creation of LTPA keys
func (r *ReconcileOpenLiberty) restartLTPAKeysGeneration(instance *olv1.OpenLibertyApplication) error {
	err, _, isLTPAKeySharingLeader, _ := r.getLTPAKeysSharingLeader(instance, false)
	if err != nil {
		return err
	}
	if isLTPAKeySharingLeader {
		ltpaSecret := &corev1.Secret{}
		ltpaSecret.Name = OperatorShortName + "-managed-ltpa"
		ltpaSecret.Namespace = instance.GetNamespace()
		err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaSecret.Name, Namespace: ltpaSecret.Namespace}, ltpaSecret)
		if err != nil && kerrors.IsNotFound(err) {
			// Deleting the job request removes existing LTPA resourxes and restarts the LTPA generation process
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
func (r *ReconcileOpenLiberty) generateLTPAKeys(instance *olv1.OpenLibertyApplication) (error, string) {
	// Initialize LTPA resources
	ltpaXMLSecret := &corev1.Secret{}
	ltpaXMLSecret.Name = OperatorShortName + lutils.LTPAServerXMLSuffix
	ltpaXMLSecret.Namespace = instance.GetNamespace()
	ltpaXMLSecret.Labels = lutils.GetRequiredLabels(ltpaXMLSecret.Name, "")

	generateLTPAKeysJob := &v1.Job{}
	generateLTPAKeysJob.Name = OperatorShortName + "-managed-ltpa-keys-generation"
	generateLTPAKeysJob.Namespace = instance.GetNamespace()
	generateLTPAKeysJob.Labels = lutils.GetRequiredLabels(generateLTPAKeysJob.Name, "")

	deletePropagationBackground := metav1.DeletePropagationBackground

	ltpaJobRequest := &corev1.ConfigMap{}
	ltpaJobRequest.Name = OperatorShortName + "-managed-ltpa-job-request"
	ltpaJobRequest.Namespace = instance.GetNamespace()
	ltpaJobRequest.Labels = lutils.GetRequiredLabels(ltpaJobRequest.Name, "")

	ltpaKeysCreationScriptConfigMap := &corev1.ConfigMap{}
	ltpaKeysCreationScriptConfigMap.Name = OperatorShortName + "-managed-ltpa-script"
	ltpaKeysCreationScriptConfigMap.Namespace = instance.GetNamespace()
	ltpaKeysCreationScriptConfigMap.Labels = lutils.GetRequiredLabels(ltpaKeysCreationScriptConfigMap.Name, "")

	ltpaSecret := &corev1.Secret{}
	ltpaSecret.Name = OperatorShortName + "-managed-ltpa"
	ltpaSecret.Namespace = instance.GetNamespace()
	ltpaSecret.Labels = lutils.GetRequiredLabels(ltpaSecret.Name, "")
	// If the LTPA Secret does not exist, run the Kubernetes Job to generate the shared ltpa.keys file and Secret
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaSecret.Name, Namespace: ltpaSecret.Namespace}, ltpaSecret)
	if err != nil && kerrors.IsNotFound(err) {
		err, ltpaKeySharingLeaderName, isLTPAKeySharingLeader, ltpaServiceAccountName := r.getLTPAKeysSharingLeader(instance, true)
		if err != nil {
			return err, ""
		}
		// If this instance is not the leader, exit the reconcile loop
		if !isLTPAKeySharingLeader {
			return fmt.Errorf("Waiting for OpenLibertyApplication instance '" + ltpaKeySharingLeaderName + "' to generate the shared LTPA keys file for the namespace '" + instance.Namespace + "'."), ""
		}

		err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaJobRequest.Name, Namespace: ltpaJobRequest.Namespace}, ltpaJobRequest)
		if err != nil {
			// Create the Job Request if it doesn't exist
			if kerrors.IsNotFound(err) {
				// Clear all LTPA-related resources from a prior reconcile
				err = r.DeleteResource(ltpaXMLSecret)
				if err != nil {
					return err, ""
				}
				err = r.DeleteResource(ltpaKeysCreationScriptConfigMap)
				if err != nil {
					return err, ""
				}
				err = r.GetClient().Delete(context.TODO(), generateLTPAKeysJob, &client.DeleteOptions{PropagationPolicy: &deletePropagationBackground})
				if err != nil && !kerrors.IsNotFound(err) {
					return err, ""
				}
				err := r.CreateOrUpdate(ltpaJobRequest, instance, func() error {
					return nil
				})
				if err != nil {
					return fmt.Errorf("Failed to create ConfigMap " + ltpaJobRequest.Name), ""
				}
			} else {
				return fmt.Errorf("Failed to get ConfigMap " + ltpaJobRequest.Name), ""
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
				script, err := ioutil.ReadFile("controllers/assets/create_ltpa_keys.sh")
				if err != nil {
					return err, ""
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
						lutils.CustomizeLTPAJob(generateLTPAKeysJob, instance, ltpaSecret.Name, ltpaServiceAccountName, ltpaKeysCreationScriptConfigMap.Name, ltpaJobRequest.Name)
						return nil
					})
					if err != nil {
						return fmt.Errorf("Failed to create Job " + generateLTPAKeysJob.Name), ""
					}
				} else if err == nil {
					// If the LTPA Secret is not yet created (LTPA Job has not successfully completed)
					// and the LTPA Job's configuration is outdated, retry LTPA generation with the new configuration
					if lutils.IsLTPAJobConfigurationOutdated(generateLTPAKeysJob, instance) {
						// Delete the Job request to restart the entire LTPA generation process (i.e. reloading the script, ltpa.xml, and Job)
						err = r.DeleteResource(ltpaJobRequest)
						if err != nil {
							return err, ltpaSecret.Name
						}
					}
				} else {
					return fmt.Errorf("Failed to get Job " + generateLTPAKeysJob.Name), ""
				}
			}
		}

		// Reconcile the Job
		err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: generateLTPAKeysJob.Name, Namespace: generateLTPAKeysJob.Namespace}, generateLTPAKeysJob)
		if err != nil && kerrors.IsNotFound(err) {
			return fmt.Errorf("Waiting for the LTPA key to be generated by Job '" + generateLTPAKeysJob.Name + "'."), ""
		} else if err != nil {
			return fmt.Errorf("Failed to get Job " + generateLTPAKeysJob.Name), ""
		}
		if len(generateLTPAKeysJob.Status.Conditions) > 0 && generateLTPAKeysJob.Status.Conditions[0].Type == v1.JobFailed {
			return fmt.Errorf("Job " + generateLTPAKeysJob.Name + " has failed. Manually clean up hung resources by setting .spec.manageLTPA to false in the " + ltpaKeySharingLeaderName + " instance."), ""
		}
		return fmt.Errorf("Waiting for the LTPA key to be generated by Job '" + generateLTPAKeysJob.Name + "'."), ""
	} else if err != nil {
		return err, ""
	} else {
		err, _, isLTPAKeySharingLeader, _ := r.getLTPAKeysSharingLeader(instance, false)
		if err != nil {
			return err, ""
		}
		if !isLTPAKeySharingLeader {
			return nil, ltpaSecret.Name
		}
	}

	// The LTPA Secret is created (in other words, the LTPA Job has completed), so delete the Job request
	err = r.DeleteResource(ltpaJobRequest)
	if err != nil {
		return err, ltpaSecret.Name
	}

	// Create the Liberty Server XML Secret if it doesn't exist
	serverXMLSecretErr := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ltpaXMLSecret.Name, Namespace: ltpaXMLSecret.Namespace}, ltpaXMLSecret)
	if serverXMLSecretErr != nil && kerrors.IsNotFound(serverXMLSecretErr) {
		r.CreateOrUpdate(ltpaXMLSecret, nil, func() error {
			lutils.CustomizeLTPAServerXML(ltpaXMLSecret, instance, string(ltpaSecret.Data["password"]))
			return nil
		})
	}
	return nil, ltpaSecret.Name
}

func (r *ReconcileOpenLiberty) isLTPAKeySharingEnabled(instance *olv1.OpenLibertyApplication) bool {
	if instance.GetManageLTPA() != nil && *instance.GetManageLTPA() {
		return true
	}
	return false
}

// Deletes resources used to create the LTPA keys file
func (r *ReconcileOpenLiberty) deleteLTPAKeysResources(instance *olv1.OpenLibertyApplication) error {
	// Don't delete LTPA keys resources if this instance is not the leader
	err, _, isLTPAKeySharingLeader, ltpaServiceAccountName := r.getLTPAKeysSharingLeader(instance, false)
	if err != nil {
		return err
	}
	if !isLTPAKeySharingLeader {
		return nil
	}

	generateLTPAKeysJob := &v1.Job{}
	generateLTPAKeysJob.Name = OperatorShortName + "-managed-ltpa-keys-generation"
	generateLTPAKeysJob.Namespace = instance.GetNamespace()
	deletePropagationBackground := metav1.DeletePropagationBackground
	err = r.GetClient().Delete(context.TODO(), generateLTPAKeysJob, &client.DeleteOptions{PropagationPolicy: &deletePropagationBackground})
	if err != nil && !kerrors.IsNotFound(err) {
		return err
	}

	ltpaKeysCreationScriptConfigMap := &corev1.ConfigMap{}
	ltpaKeysCreationScriptConfigMap.Name = OperatorShortName + "-managed-ltpa-script"
	ltpaKeysCreationScriptConfigMap.Namespace = instance.GetNamespace()
	err = r.DeleteResource(ltpaKeysCreationScriptConfigMap)
	if err != nil {
		return err
	}

	ltpaJobRequest := &corev1.ConfigMap{}
	ltpaJobRequest.Name = OperatorShortName + "-managed-ltpa-job-request"
	ltpaJobRequest.Namespace = instance.GetNamespace()
	err = r.DeleteResource(ltpaJobRequest)
	if err != nil {
		return err
	}

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

	ltpaServiceAccount := &corev1.ServiceAccount{}
	ltpaServiceAccount.Name = ltpaServiceAccountName
	ltpaServiceAccount.Namespace = instance.GetNamespace()
	err = r.DeleteResource(ltpaServiceAccount)
	if err != nil {
		return err
	}

	return nil
}
