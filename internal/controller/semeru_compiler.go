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

package controller

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	openlibertyv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	"github.com/application-stacks/runtime-component-operator/common"
	utils "github.com/application-stacks/runtime-component-operator/utils"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	SemeruLabelNameSuffix                   = "-semeru-compiler"
	SemeruLabelName                         = "semeru-compiler"
	SemeruGenerationLabelNameSuffix         = "/semeru-compiler-generation"
	StatusReferenceSemeruGeneration         = "semeruGeneration"
	StatusReferenceSemeruInstancesCompleted = "semeruInstancesCompleted"
)

// Create the Deployment and Service objects for a Semeru Compiler used by an Open Liberty Application
func (r *ReconcileOpenLiberty) reconcileSemeruCompiler(ola *openlibertyv1.OpenLibertyApplication) (error, string, bool) {
	compilerMeta := metav1.ObjectMeta{
		Name:      getSemeruCompilerNameWithGeneration(ola),
		Namespace: ola.GetNamespace(),
	}

	currentGeneration := getGeneration(ola)

	if r.isSemeruEnabled(ola) {
		cmPresent, _ := r.IsGroupVersionSupported(certmanagerv1.SchemeGroupVersion.String(), "Certificate")

		// Create the Semeru Service object
		semsvc := &corev1.Service{ObjectMeta: compilerMeta}
		tlsSecretName := ""
		err := r.CreateOrUpdate(semsvc, ola, func() error {
			reconcileSemeruService(semsvc, ola)
			if r.IsOpenShift() {
				if _, ok := semsvc.Annotations["service.beta.openshift.io/serving-cert-secret-name"]; !ok {
					if _, ok = semsvc.Annotations["service.alpha.openshift.io/serving-cert-secret-name"]; !ok {
						if semsvc.Annotations == nil {
							semsvc.Annotations = map[string]string{}
						}
						tlsSecretName = semsvc.GetName() + "-tls-ocp"
						semsvc.Annotations["service.beta.openshift.io/serving-cert-secret-name"] = tlsSecretName
						if ola.Status.SemeruCompiler == nil {
							ola.Status.SemeruCompiler = &openlibertyv1.SemeruCompilerStatus{}
						}
						ola.Status.SemeruCompiler.TLSSecretName = tlsSecretName
					}
				}
			}
			return nil
		})
		if err != nil {
			return err, "Failed to reconcile the Semeru Compiler Service", false
		}

		//create certmanager issuer and certificate if necessary
		if cmPresent {
			err = r.GenerateCMIssuer(ola.Namespace, OperatorShortName, "Open Liberty Operator", OperatorName)
			if err != nil {
				return err, "Failed to reconcile Certificate Issuer", false
			}
			err = r.reconcileSemeruCMCertificate(ola)
			if err != nil {
				return err, "Failed to reconcile Semeru Compiler Certificate", false
			}
		} else if !r.IsOpenShift() {
			return fmt.Errorf("Could not detect a cert-manager installation. Ensure cert-manager is installed and running"), "", false
		}

		//TLS Secret
		semeruTLSSecret := &corev1.Secret{}
		err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: ola.Status.SemeruCompiler.TLSSecretName, Namespace: ola.Namespace}, semeruTLSSecret)
		if err != nil {
			return err, "Failed to reconcile Semeru Compiler TLS Secret", false
		}

		//Deployment
		semeruDeployment := &appsv1.Deployment{ObjectMeta: compilerMeta}
		err = r.CreateOrUpdate(semeruDeployment, ola, func() error {
			r.reconcileSemeruDeployment(ola, semeruDeployment)
			return nil
		})
		if err != nil {
			return err, "Failed to reconcile Deployment : " + semeruDeployment.Name, false
		}

		// Add the new generation number to .status.reference.semeruInstancesCompleted as a comma-separated string
		areCompletedSemeruInstancesMarkedToBeDeleted := false
		if ola.Status.References != nil {
			if completedInstances, ok := ola.Status.References[StatusReferenceSemeruInstancesCompleted]; ok {
				if completedInstances != currentGeneration {
					// Mark old Semeru Cloud Compiler instances for deletion
					areCompletedSemeruInstancesMarkedToBeDeleted = true
					if !strings.Contains(completedInstances, currentGeneration) {
						ola.Status.References[StatusReferenceSemeruInstancesCompleted] += "," + currentGeneration
					}
				}
			} else {
				ola.Status.References[StatusReferenceSemeruInstancesCompleted] = currentGeneration
			}
		}

		return nil, "", areCompletedSemeruInstancesMarkedToBeDeleted
	} else {
		semsvc := &corev1.Service{ObjectMeta: compilerMeta}
		semeruDeployment := &appsv1.Deployment{ObjectMeta: compilerMeta}
		if err := r.DeleteResources([]client.Object{semsvc, semeruDeployment}); err != nil {
			return err, "Failed to delete Semeru Compiler resources", false
		}
		ola.Status.SemeruCompiler = nil
		return nil, "", false
	}
}

// Returns the one-based index generation indicated by .status.references.semeruGeneration if it exists, otherwise defaults to 1
func getGeneration(ola *openlibertyv1.OpenLibertyApplication) string {
	if ola.Status.References != nil {
		if semeruGeneration, ok := ola.Status.References[StatusReferenceSemeruGeneration]; ok {
			return semeruGeneration
		}
		ola.Status.References[StatusReferenceSemeruGeneration] = fmt.Sprint(1)
	}
	return "1"
}

// Increments the generation number at .status.references.semeruGeneration if it exists, otherwise if possible, initializes the generation to 1
func createNewSemeruGeneration(ola *openlibertyv1.OpenLibertyApplication) {
	if ola.Status.References != nil {
		if semeruGeneration, ok := ola.Status.References[StatusReferenceSemeruGeneration]; ok {
			if generation, err := strconv.Atoi(semeruGeneration); err == nil {
				ola.Status.References[StatusReferenceSemeruGeneration] = fmt.Sprint(generation + 1)
			} else {
				ola.Status.References[StatusReferenceSemeruGeneration] = fmt.Sprint(1)
			}
		} else {
			ola.Status.References[StatusReferenceSemeruGeneration] = fmt.Sprint(1)
		}
	}
}

func getSemeruGenerationLabelName(ola *openlibertyv1.OpenLibertyApplication) string {
	return ola.GetGroupName() + SemeruGenerationLabelNameSuffix
}

// Deletes old Semeru Cloud Compiler instances that have been marked as completed (instances that underwent at least one reconcile)
func (r *ReconcileOpenLiberty) deleteCompletedSemeruInstances(ola *openlibertyv1.OpenLibertyApplication) error {
	if semeruInstancesCompleted, ok := ola.Status.References[StatusReferenceSemeruInstancesCompleted]; ok {
		generationsMarkedForDeletion := make([]string, 0)
		cmPresent, _ := r.IsGroupVersionSupported(certmanagerv1.SchemeGroupVersion.String(), "Certificate")
		useCertManager := !r.IsOpenShift() || cmPresent

		// For each completed Semeru Cloud Compiler generation
		for _, completedGenerationStr := range strings.Split(semeruInstancesCompleted, ",") {
			completedGeneration, _ := strconv.Atoi(completedGenerationStr)
			currentGeneration, _ := strconv.Atoi(ola.Status.References[StatusReferenceSemeruGeneration])

			// Delete the older generation's resources and mark the status reference field for deletion
			if completedGeneration < currentGeneration {
				resourceName := getSemeruCompilerName(ola) + "-" + completedGenerationStr
				resourceNamespace := ola.GetNamespace()
				resourceLabels := map[string]string{
					getSemeruGenerationLabelName(ola): completedGenerationStr,
					"app.kubernetes.io/name":          getSemeruCompilerName(ola),
				}

				// Delete Deployment
				deployment := &appsv1.Deployment{}
				deployment.Name = resourceName
				deployment.Namespace = resourceNamespace
				deployment.Labels = resourceLabels
				err := r.DeleteResource(deployment)
				if err != nil {
					return err
				}

				// Delete Service
				service := &corev1.Service{}
				service.Name = resourceName
				service.Namespace = resourceNamespace
				service.Labels = resourceLabels
				err = r.DeleteResource(service)
				if err != nil {
					return err
				}

				// Remove CertManager Certificate and Secret if necessary
				if useCertManager {
					cmCertificate := &certmanagerv1.Certificate{}
					cmCertificate.Name = resourceName
					cmCertificate.Namespace = resourceNamespace
					cmCertificate.Labels = resourceLabels
					err = r.DeleteResource(cmCertificate)
					if err != nil {
						return err
					}

					cmSecret := &corev1.Secret{}
					cmSecret.Name = resourceName + "-tls-cm"
					cmSecret.Namespace = resourceNamespace
					err = r.DeleteResource(cmSecret)
					if err != nil {
						return err
					}
				}

				// On successful cleanup, mark the generation for deletion from the status reference field
				generationsMarkedForDeletion = append(generationsMarkedForDeletion, completedGenerationStr)
			}
		}

		// Remove deleted generations from the status reference field
		for _, deletedGeneration := range generationsMarkedForDeletion {
			oldInstancesCompleted := ola.Status.References[StatusReferenceSemeruInstancesCompleted]
			ola.Status.References[StatusReferenceSemeruInstancesCompleted] = strings.Replace(oldInstancesCompleted, deletedGeneration+",", "", 1)
			// Corner case: The new generation completed before the old generation completed
			if oldInstancesCompleted == ola.Status.References[StatusReferenceSemeruInstancesCompleted] {
				ola.Status.References[StatusReferenceSemeruInstancesCompleted] = strings.Replace(oldInstancesCompleted, ","+deletedGeneration, "", 1)
			}
		}
	}
	return nil
}

func (r *ReconcileOpenLiberty) reconcileSemeruDeployment(ola *openlibertyv1.OpenLibertyApplication, deploy *appsv1.Deployment) {
	deploy.Labels = getLabels(ola)
	deploy.Spec.Strategy.Type = appsv1.RecreateDeploymentStrategyType

	if deploy.Spec.Selector == nil {
		deploy.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: getSelectors(ola),
		}
	}

	semeruCloudCompiler := ola.GetSemeruCloudCompiler()

	deploy.Spec.Replicas = semeruCloudCompiler.GetReplicas()

	// Get Semeru resources config
	instanceResources := semeruCloudCompiler.Resources

	requestsMemory := getQuantityFromRequestsOrDefault(instanceResources, corev1.ResourceMemory, "800Mi")
	requestsCPU := getQuantityFromRequestsOrDefault(instanceResources, corev1.ResourceCPU, "100m")
	limitsMemory := getQuantityFromLimitsOrDefault(instanceResources, corev1.ResourceMemory, "1200Mi")
	limitsCPU := getQuantityFromLimitsOrDefault(instanceResources, corev1.ResourceCPU, "2000m")

	// Liveness probe
	livenessProbe := corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(38400),
			},
		},
		InitialDelaySeconds: 10,
		PeriodSeconds:       10,
	}

	// Readiness probe
	readinessProbe := corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(38400),
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       5,
	}

	semeruPodMatchLabels := map[string]string{
		"app.kubernetes.io/instance": getSemeruCompilerNameWithGeneration(ola),
	}
	deploy.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: getLabels(ola),
		},
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
						{
							Weight: 50,
							PodAffinityTerm: corev1.PodAffinityTerm{
								TopologyKey: "topology.kubernetes.io/zone",
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: semeruPodMatchLabels,
								},
							},
						},
						{
							Weight: 50,
							PodAffinityTerm: corev1.PodAffinityTerm{
								TopologyKey: "kubernetes.io/hostname",
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: semeruPodMatchLabels,
								},
							},
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:            "compiler",
					Image:           ola.Status.GetImageReference(),
					ImagePullPolicy: *ola.GetPullPolicy(),
					Command:         []string{"jitserver"},
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 38400,
							Protocol:      corev1.ProtocolTCP,
						},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: requestsMemory,
							corev1.ResourceCPU:    requestsCPU,
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: limitsMemory,
							corev1.ResourceCPU:    limitsCPU,
						},
					},
					Env: []corev1.EnvVar{
						{Name: "OPENJ9_JAVA_OPTIONS", Value: "-XX:+JITServerLogConnections" +
							" -XX:+JITServerShareROMClasses" +
							" -XX:JITServerSSLKey=/etc/x509/certs/tls.key" +
							" -XX:JITServerSSLCert=/etc/x509/certs/tls.crt"},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "certs",
							ReadOnly:  true,
							MountPath: "/etc/x509/certs",
						},
					},
					LivenessProbe:  &livenessProbe,
					ReadinessProbe: &readinessProbe,
				},
			},
			Volumes: []corev1.Volume{{
				Name: "certs",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: ola.Status.SemeruCompiler.TLSSecretName,
					},
				},
			}},
		},
	}

	// Configure TopologySpreadConstraints from the OpenLibertyApplication CR
	deploy.Spec.Template.Spec.TopologySpreadConstraints = make([]corev1.TopologySpreadConstraint, 0)
	topologySpreadConstraintsConfig := ola.GetTopologySpreadConstraints()
	if topologySpreadConstraintsConfig == nil || topologySpreadConstraintsConfig.GetDisableOperatorDefaults() == nil || !*topologySpreadConstraintsConfig.GetDisableOperatorDefaults() {
		utils.CustomizeTopologySpreadConstraints(&deploy.Spec.Template, semeruPodMatchLabels)
	}
	if topologySpreadConstraintsConfig != nil && topologySpreadConstraintsConfig.GetConstraints() != nil {
		deploy.Spec.Template.Spec.TopologySpreadConstraints = utils.MergeTopologySpreadConstraints(deploy.Spec.Template.Spec.TopologySpreadConstraints,
			*topologySpreadConstraintsConfig.GetConstraints())
	}

	// Copy the service account from the OpenLibertyApplcation CR
	if saName := utils.GetServiceAccountName(ola); saName != "" {
		deploy.Spec.Template.Spec.ServiceAccountName = saName
	} else {
		deploy.Spec.Template.Spec.ServiceAccountName = ola.GetName()
	}

	// This ensures that the semeru pod(s) are updated if the service account is updated
	saRV := ola.GetStatus().GetReferences()[common.StatusReferenceSAResourceVersion]
	if saRV != "" {
		deploy.Spec.Template.Spec.Containers[0].Env = append(deploy.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: "SA_RESOURCE_VERSION", Value: saRV})
	}

	// Copy the securityContext from the OpenLibertyApplcation CR
	deploy.Spec.Template.Spec.Containers[0].SecurityContext = utils.GetSecurityContext(ola)

	lutils.AddSecretResourceVersionAsEnvVar(&deploy.Spec.Template, ola, r.GetClient(), ola.Status.SemeruCompiler.TLSSecretName, "TLS")
}

func reconcileSemeruService(svc *corev1.Service, ola *openlibertyv1.OpenLibertyApplication) {
	var port int32 = 38400
	var timeout int32 = 86400
	svc.Labels = getLabels(ola)
	svc.Spec.Selector = getSelectors(ola)
	utils.CustomizeServiceAnnotations(svc, ola.GetDisableTopology())
	if len(svc.Spec.Ports) == 0 {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{})
	}
	svc.Spec.Ports[0].Protocol = corev1.ProtocolTCP
	svc.Spec.Ports[0].Port = port
	svc.Spec.Ports[0].TargetPort = intstr.FromInt(int(port))
	svc.Spec.SessionAffinity = corev1.ServiceAffinityClientIP
	svc.Spec.SessionAffinityConfig = &corev1.SessionAffinityConfig{
		ClientIP: &corev1.ClientIPConfig{
			TimeoutSeconds: &timeout,
		},
	}

	if ola.Status.SemeruCompiler == nil {
		ola.Status.SemeruCompiler = &openlibertyv1.SemeruCompilerStatus{}
	}
	ola.Status.SemeruCompiler.ServiceHostname = svc.GetName() + "." + svc.GetNamespace() + ".svc"

}

func (r *ReconcileOpenLiberty) reconcileSemeruCMCertificate(ola *openlibertyv1.OpenLibertyApplication) error {
	svcCert := &certmanagerv1.Certificate{}
	svcCert.Name = getSemeruCompilerNameWithGeneration(ola)
	svcCert.Namespace = ola.GetNamespace()
	customIssuer := &certmanagerv1.Issuer{ObjectMeta: metav1.ObjectMeta{
		Name:      OperatorShortName + "-custom-issuer",
		Namespace: svcCert.Namespace,
	}}

	customIssuerFound := false
	err := r.GetClient().Get(context.Background(), types.NamespacedName{Name: customIssuer.Name,
		Namespace: customIssuer.Namespace}, customIssuer)
	if err == nil {
		customIssuerFound = true
	}

	shouldRefreshCertSecret := false

	err = r.CreateOrUpdate(svcCert, ola, func() error {
		svcCert.Labels = ola.GetLabels()
		svcCert.Labels[getSemeruGenerationLabelName(ola)] = getGeneration(ola)
		svcCert.Spec.IssuerRef = certmanagermetav1.ObjectReference{
			Name: OperatorShortName + "-ca-issuer",
		}
		if customIssuerFound {
			svcCert.Spec.IssuerRef.Name = customIssuer.Name
		}

		rVersion, _ := utils.GetIssuerResourceVersion(r.GetClient(), svcCert)
		if svcCert.Spec.SecretTemplate == nil {
			svcCert.Spec.SecretTemplate = &certmanagerv1.CertificateSecretTemplate{
				Annotations: map[string]string{},
			}
		}

		if svcCert.Spec.SecretTemplate.Annotations[ola.GetGroupName()+"/cm-issuer-version"] != rVersion {
			if svcCert.Spec.SecretTemplate.Annotations == nil {
				svcCert.Spec.SecretTemplate.Annotations = map[string]string{}
			}
			svcCert.Spec.SecretTemplate.Annotations[ola.GetGroupName()+"/cm-issuer-version"] = rVersion
			shouldRefreshCertSecret = true
		}

		svcCert.Spec.SecretName = svcCert.Name + "-tls-cm"
		svcCert.Spec.DNSNames = make([]string, 2)
		svcCert.Spec.DNSNames[0] = svcCert.Name + "." + ola.Namespace + ".svc"
		svcCert.Spec.DNSNames[1] = svcCert.Name + "." + ola.Namespace + ".svc.cluster.local"
		svcCert.Spec.CommonName = svcCert.Name
		duration, err := time.ParseDuration(common.LoadFromConfig(common.Config, common.OpConfigCMCertDuration))
		if err != nil {
			return err
		}
		svcCert.Spec.Duration = &metav1.Duration{Duration: duration}
		return nil
	})
	if err != nil {
		return err
	}

	if shouldRefreshCertSecret {
		r.DeleteResource(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: svcCert.Spec.SecretName, Namespace: svcCert.Namespace}})
	}

	if ola.Status.SemeruCompiler == nil {
		ola.Status.SemeruCompiler = &openlibertyv1.SemeruCompilerStatus{}
	}
	ola.Status.SemeruCompiler.TLSSecretName = svcCert.Spec.SecretName
	return nil
}

func getSemeruCompilerNameWithGeneration(ola *openlibertyv1.OpenLibertyApplication) string {
	return getSemeruCompilerName(ola) + "-" + getGeneration(ola)
}

func getSemeruCompilerName(ola *openlibertyv1.OpenLibertyApplication) string {
	return ola.GetName() + SemeruLabelNameSuffix
}

// Create the Selector map for a Semeru Compiler
func getSelectors(ola *openlibertyv1.OpenLibertyApplication) map[string]string {
	requiredSelector := make(map[string]string)
	requiredSelector["app.kubernetes.io/component"] = SemeruLabelName
	requiredSelector["app.kubernetes.io/instance"] = getSemeruCompilerNameWithGeneration(ola)
	requiredSelector["app.kubernetes.io/part-of"] = ola.GetName()
	return requiredSelector
}

// Create the Labels map for a Semeru Compiler
func getLabels(ola *openlibertyv1.OpenLibertyApplication) map[string]string {
	requiredLabels := make(map[string]string)
	requiredLabels["app.kubernetes.io/name"] = getSemeruCompilerName(ola)
	requiredLabels["app.kubernetes.io/instance"] = getSemeruCompilerNameWithGeneration(ola)
	requiredLabels["app.kubernetes.io/managed-by"] = OperatorName
	requiredLabels["app.kubernetes.io/component"] = SemeruLabelName
	requiredLabels["app.kubernetes.io/part-of"] = ola.GetName()
	requiredLabels[getSemeruGenerationLabelName(ola)] = getGeneration(ola)
	return requiredLabels
}

// Returns quantity at resourceRequirements.Requests[resourceName] if it exists, otherwise return the parsed defaultQuantity
func getQuantityFromRequestsOrDefault(resourceRequirements *corev1.ResourceRequirements, resourceName corev1.ResourceName, defaultQuantity string) resource.Quantity {
	if resourceRequirements != nil && resourceRequirements.Requests != nil {
		if mapValue, ok := resourceRequirements.Requests[resourceName]; ok {
			return mapValue
		}
	}
	return resource.MustParse(defaultQuantity)
}

// Returns quantity at resourceRequirements.Limits[resourceName] if it exists, otherwise return the parsed defaultQuantity
func getQuantityFromLimitsOrDefault(resourceRequirements *corev1.ResourceRequirements, resourceName corev1.ResourceName, defaultQuantity string) resource.Quantity {
	if resourceRequirements != nil && resourceRequirements.Limits != nil {
		if mapValue, ok := resourceRequirements.Limits[resourceName]; ok {
			return mapValue
		}
	}
	return resource.MustParse(defaultQuantity)
}

func getSemeruCertVolumeMount(ola *openlibertyv1.OpenLibertyApplication) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "semeru-certs",
		MountPath: "/etc/x509/semeru-certs",
		ReadOnly:  true,
	}
}
func getSemeruCertVolume(ola *openlibertyv1.OpenLibertyApplication) *corev1.Volume {
	if ola.Status.SemeruCompiler == nil || ola.Status.SemeruCompiler.TLSSecretName == "" ||
		strings.HasSuffix(ola.Status.SemeruCompiler.TLSSecretName, "-ocp") {
		return nil
	}
	return &corev1.Volume{
		Name: "semeru-certs",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: ola.Status.SemeruCompiler.TLSSecretName,
			},
		},
	}
}

func (r *ReconcileOpenLiberty) getSemeruJavaOptions(instance *openlibertyv1.OpenLibertyApplication) []string {
	if r.isSemeruEnabled(instance) {
		certificateLocation := "/etc/x509/semeru-certs/ca.crt"
		if instance.Status.SemeruCompiler != nil && strings.HasSuffix(instance.Status.SemeruCompiler.TLSSecretName, "-ocp") {
			certificateLocation = "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt"
		}
		jitServerAddress := instance.Status.SemeruCompiler.ServiceHostname
		jitSeverOptions := fmt.Sprintf("-XX:+UseJITServer -XX:+JITServerLogConnections -XX:JITServerAddress=%v -XX:JITServerSSLRootCerts=%v",
			jitServerAddress, certificateLocation)

		args := []string{
			"/bin/bash",
			"-c",
			"export OPENJ9_JAVA_OPTIONS=\"$OPENJ9_JAVA_OPTIONS " + jitSeverOptions +
				"\" && export OPENJ9_RESTORE_JAVA_OPTIONS=\"$OPENJ9_RESTORE_JAVA_OPTIONS " + jitSeverOptions +
				"\" && server run",
		}
		return args
	}
	return nil
}
func (r *ReconcileOpenLiberty) areSemeruCompilerResourcesReady(ola *openlibertyv1.OpenLibertyApplication) error {
	var replicas, readyReplicas, updatedReplicas int32
	namespacedName := types.NamespacedName{Name: getSemeruCompilerNameWithGeneration(ola), Namespace: ola.GetNamespace()}

	// Check if deployment exists
	deployment := &appsv1.Deployment{}
	err := r.GetClient().Get(context.TODO(), namespacedName, deployment)
	if err != nil {
		return errors.New("Semeru Cloud Compiler is not ready: Deployment is not created.")
	}

	// Get replicas
	expectedReplicas := ola.GetSemeruCloudCompiler().GetReplicas()
	ds := deployment.Status
	replicas, readyReplicas, updatedReplicas = ds.Replicas, ds.ReadyReplicas, ds.UpdatedReplicas

	// Check if all replicas are equal to the expected replicas
	if replicas == *expectedReplicas && readyReplicas == *expectedReplicas && updatedReplicas == *expectedReplicas {
		return nil // Semeru ready
	} else if replicas > *expectedReplicas {
		return errors.New("Semeru Cloud Compiler is not ready: Replica set is progressing.")
	}
	return errors.New("Semeru Cloud Compiler is not ready: Deployment is not ready.")
}

func (r *ReconcileOpenLiberty) isSemeruEnabled(ola *openlibertyv1.OpenLibertyApplication) bool {
	if ola.GetSemeruCloudCompiler() != nil && ola.GetSemeruCloudCompiler().Enable {
		return true
	} else {
		return false
	}
}
