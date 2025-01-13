package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	"github.com/application-stacks/runtime-component-operator/common"
	oputils "github.com/application-stacks/runtime-component-operator/utils"
	"github.com/go-logr/logr"
	imagev1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	imageutil "github.com/openshift/library-go/pkg/image/imageutil"
	"github.com/pkg/errors"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ReconcileResult struct {
	err       error
	condition common.StatusConditionType
	message   string
}

func (r *ReconcileOpenLiberty) isConcurrencyEnabled(instance *olv1.OpenLibertyApplication) bool {
	if instance.GetExperimental() != nil && instance.GetExperimental().GetManageConcurrency() != nil && *instance.GetExperimental().GetManageConcurrency() {
		return true
	}
	return false
}

func (r *ReconcileOpenLiberty) isCachingEnabled(instance *olv1.OpenLibertyApplication) bool {
	if instance.GetExperimental() != nil && instance.GetExperimental().GetManageCache() != nil && *instance.GetExperimental().GetManageCache() {
		return true
	}
	return false
}

func (r *ReconcileOpenLiberty) reconcileLibertyProxyConcurrent(operatorNamespace string, reconcileResultChan chan<- ReconcileResult) {
	message, err := r.reconcileLibertyProxy(operatorNamespace)
	reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: message}
}

func (r *ReconcileOpenLiberty) reconcileImageStream(instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
	if r.IsOpenShift() {
		instanceMutex.Lock()
		image, err := imageutil.ParseDockerImageReference(instance.Spec.ApplicationImage)
		instanceMutex.Unlock()
		if err == nil {
			isTag := &imagev1.ImageStreamTag{}
			isTagName := imageutil.JoinImageStreamTag(image.Name, image.Tag)
			isTagNamespace := image.Namespace
			if isTagNamespace == "" {
				instanceMutex.Lock()
				isTagNamespace = instance.Namespace
				instanceMutex.Unlock()
			}
			key := types.NamespacedName{Name: isTagName, Namespace: isTagNamespace}
			err = r.GetAPIReader().Get(context.Background(), key, isTag)
			// Call ManageError only if the error type is not found or is not forbidden. Forbidden could happen
			// when the operator tries to call GET for ImageStreamTags on a namespace that doesn't exists (e.g.
			// cannot get imagestreamtags.image.openshift.io in the namespace "navidsh": no RBAC policy matched)
			if err == nil {
				image := isTag.Image
				if image.DockerImageReference != "" {
					instanceMutex.Lock()
					instance.Status.ImageReference = image.DockerImageReference
					instanceMutex.Unlock()
				}
			} else if err != nil && !kerrors.IsNotFound(err) && !kerrors.IsForbidden(err) && !strings.Contains(isTagName, "/") {
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled}
				return
			}
		}
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileLTPAKeySharingEnabled(instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult, ltpaMetadataChan chan<- *lutils.LTPAMetadata) {
	// Reconciles the shared LTPA state for the instance namespace
	var ltpaMetadataList *lutils.LTPAMetadataList
	expectedMetadataLength := 2
	instanceMutex.Lock()
	if r.isLTPAKeySharingEnabled(instance) {
		leaderMetadataList, err := r.reconcileResourceTrackingState(instance, LTPA_RESOURCE_SHARING_FILE_NAME, r.isCachingEnabled(instance))
		instanceMutex.Unlock()
		if err != nil {
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled}
			// send dummy data back to the channel
			for i := 0; i < expectedMetadataLength; i++ {
				ltpaMetadataChan <- nil
			}
			return
		}
		ltpaMetadataList = leaderMetadataList.(*lutils.LTPAMetadataList)
		if ltpaMetadataList != nil && len(ltpaMetadataList.Items) == expectedMetadataLength {
			for i := 0; i < expectedMetadataLength; i++ {
				ltpaMetadataChan <- ltpaMetadataList.Items[i].(*lutils.LTPAMetadata)
			}
		}
	} else {
		instanceMutex.Unlock()
		for i := 0; i < expectedMetadataLength; i++ {
			ltpaMetadataChan <- nil
		}
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcilePasswordEncryptionKeySharingEnabled(instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult, passwordEncryptionMetadataChan chan<- *lutils.PasswordEncryptionMetadata) {
	// Reconciles the shared password encryption key state for the instance namespace only if the shared key already exists
	var passwordEncryptionMetadataList *lutils.PasswordEncryptionMetadataList
	passwordEncryptionMetadata := &lutils.PasswordEncryptionMetadata{Name: ""}
	expectedMetadataLength := 1
	instanceMutex.Lock()
	if r.isUsingPasswordEncryptionKeySharing(instance, passwordEncryptionMetadata) {
		leaderMetadataList, err := r.reconcileResourceTrackingState(instance, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, r.isCachingEnabled(instance))
		instanceMutex.Unlock()
		if err != nil {
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled}
			for i := 0; i < expectedMetadataLength; i++ {
				passwordEncryptionMetadataChan <- passwordEncryptionMetadata
			}
			return
		}
		passwordEncryptionMetadataList = leaderMetadataList.(*lutils.PasswordEncryptionMetadataList)
		if passwordEncryptionMetadataList != nil && len(passwordEncryptionMetadataList.Items) == expectedMetadataLength {
			for i := 0; i < expectedMetadataLength; i++ {
				passwordEncryptionMetadataChan <- passwordEncryptionMetadataList.Items[i].(*lutils.PasswordEncryptionMetadata)
			}
		}
	} else if r.isPasswordEncryptionKeySharingEnabled(instance) {
		instanceMutex.Unlock()
		// error if the password encryption key sharing is enabled but the Secret is not found
		passwordEncryptionSecretName := lutils.PasswordEncryptionKeyRootName + passwordEncryptionMetadata.Name
		err := errors.Wrapf(fmt.Errorf("Secret %q not found", passwordEncryptionSecretName), "Secret for Password Encryption was not found. Create a secret named %q in namespace %q with the encryption key specified in data field %q.", passwordEncryptionSecretName, instance.GetNamespace(), "passwordEncryptionKey")
		reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled}
		for i := 0; i < expectedMetadataLength; i++ {
			passwordEncryptionMetadataChan <- passwordEncryptionMetadata
		}
		return
	} else {
		instanceMutex.Unlock()
		for i := 0; i < expectedMetadataLength; i++ {
			passwordEncryptionMetadataChan <- passwordEncryptionMetadata
		}
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}

}

func (r *ReconcileOpenLiberty) reconcileServiceAccount(defaultMeta metav1.ObjectMeta, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
	instanceMutex.Lock()
	serviceAccountName := oputils.GetServiceAccountName(instance)
	instanceMutex.Unlock()
	if serviceAccountName != defaultMeta.Name {
		if serviceAccountName == "" {
			serviceAccount := &corev1.ServiceAccount{ObjectMeta: defaultMeta}
			instanceMutex.Lock()
			err := r.CreateOrUpdate(serviceAccount, instance, func() error {
				return oputils.CustomizeServiceAccount(serviceAccount, instance, r.GetClient())
			})
			instanceMutex.Unlock()
			if err != nil {
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile ServiceAccount"}
				return
			}
		} else {
			serviceAccount := &corev1.ServiceAccount{ObjectMeta: defaultMeta}
			err := r.DeleteResource(serviceAccount)
			if err != nil {
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete ServiceAccount"}
				return
			}
		}
	}

	// Check if the ServiceAccount has a valid pull secret before creating the deployment/statefulset
	// or setting up knative. Otherwise the pods can go into an ImagePullBackOff loop
	instanceMutex.Lock()
	saErr := oputils.ServiceAccountPullSecretExists(instance, r.GetClient())
	instanceMutex.Unlock()
	if saErr != nil {
		reconcileResultChan <- ReconcileResult{err: saErr, condition: common.StatusConditionTypeReconciled}
		return
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileSemeruCloudCompilerInit(instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult, semeruMarkedForDeletionChan chan<- bool) {
	// Check if SemeruCloudCompiler is enabled before reconciling the Semeru Compiler deployment and service.
	// Otherwise, delete the Semeru Compiler deployment and service.
	message := "Start Semeru Compiler reconcile"
	instanceMutex.Lock()
	err, message, areCompletedSemeruInstancesMarkedToBeDeleted := r.reconcileSemeruCompiler(instance)
	instanceMutex.Unlock()
	semeruMarkedForDeletionChan <- areCompletedSemeruInstancesMarkedToBeDeleted
	if err != nil {
		reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: message}
		return
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileSemeruCloudCompilerReady(instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
	// If semeru compiler is enabled, make sure its ready
	instanceMutex.Lock()
	if r.isSemeruEnabled(instance) {
		err := r.areSemeruCompilerResourcesReady(instance)
		message := "Check Semeru Compiler resources ready"
		instanceMutex.Unlock()
		if err != nil {
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeResourcesReady, message: message}
			return
		}
	} else {
		instanceMutex.Unlock()
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeResourcesReady}
}

func (r *ReconcileOpenLiberty) reconcileKnativeServiceSequential(defaultMeta metav1.ObjectMeta, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reqLogger logr.Logger, isKnativeSupported bool) (ctrl.Result, error) {
	instanceMutex.Lock()
	defer instanceMutex.Unlock()
	if instance.Spec.CreateKnativeService != nil && *instance.Spec.CreateKnativeService {
		// Clean up non-Knative resources
		resources := []client.Object{
			&corev1.Service{ObjectMeta: defaultMeta},
			&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: instance.Name + "-headless", Namespace: instance.Namespace}},
			&appsv1.Deployment{ObjectMeta: defaultMeta},
			&appsv1.StatefulSet{ObjectMeta: defaultMeta},
			&autoscalingv1.HorizontalPodAutoscaler{ObjectMeta: defaultMeta},
			&networkingv1.NetworkPolicy{ObjectMeta: defaultMeta},
		}
		err := r.DeleteResources(resources)
		if err != nil {
			reqLogger.Error(err, "Failed to clean up non-Knative resources")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}

		if ok, _ := r.IsGroupVersionSupported(networkingv1.SchemeGroupVersion.String(), "Ingress"); ok {
			r.DeleteResource(&networkingv1.Ingress{ObjectMeta: defaultMeta})
		}

		if r.IsOpenShift() {
			route := &routev1.Route{ObjectMeta: defaultMeta}
			err = r.DeleteResource(route)
			if err != nil {
				reqLogger.Error(err, "Failed to clean up non-Knative resource Route")
				return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
			}
		}
		if isKnativeSupported {
			reqLogger.Info("Knative is supported and Knative Service is enabled")
			ksvc := &servingv1.Service{ObjectMeta: defaultMeta}
			err = r.CreateOrUpdate(ksvc, instance, func() error {
				oputils.CustomizeKnativeService(ksvc, instance)
				return nil
			})

			if err != nil {
				reqLogger.Error(err, "Failed to reconcile Knative Service")
				return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
			}
			instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
			instance.Status.Versions.Reconciled = lutils.OperandVersion
			reqLogger.Info("Reconcile OpenLibertyApplication - completed")
			return r.ManageSuccess(common.StatusConditionTypeReconciled, instance)
		}
		return r.ManageError(errors.New("failed to reconcile Knative service as operator could not find Knative CRDs"), common.StatusConditionTypeReconciled, instance)
	}

	if isKnativeSupported {
		ksvc := &servingv1.Service{ObjectMeta: defaultMeta}
		err := r.DeleteResource(ksvc)
		if err != nil {
			reqLogger.Error(err, "Failed to delete Knative Service")
			r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	}
	return ctrl.Result{}, nil
}

func (r *ReconcileOpenLiberty) reconcileServiceCertificate(ba common.BaseComponent, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, serviceCertificateReconcileResultChan chan<- ReconcileResult, useCertManagerChan chan<- bool) {
	instanceMutex.Lock()
	useCertmanager, err := r.GenerateSvcCertSecret(ba, OperatorShortName, "Open Liberty Operator", OperatorName)
	instanceMutex.Unlock()
	useCertManagerChan <- useCertmanager
	if err != nil {
		serviceCertificateReconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile CertManager Certificate"}
		return
	}

	instanceMutex.Lock()
	if ba.GetService().GetCertificateSecretRef() != nil {
		ba.GetStatus().SetReference(common.StatusReferenceCertSecretName, *ba.GetService().GetCertificateSecretRef())
	}
	instanceMutex.Unlock()
	serviceCertificateReconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileService(defaultMeta metav1.ObjectMeta, ba common.BaseComponent, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult, useCertManagerChan <-chan bool) {
	svc := &corev1.Service{ObjectMeta: defaultMeta}
	instanceMutex.Lock()
	err := r.CreateOrUpdate(svc, instance, func() error {
		oputils.CustomizeService(svc, ba)
		svc.Annotations = oputils.MergeMaps(svc.Annotations, instance.Spec.Service.Annotations)
		instanceMutex.Unlock()
		useCertmanager := <-useCertManagerChan
		if !useCertmanager && r.IsOpenShift() {
			instanceMutex.Lock()
			oputils.AddOCPCertAnnotation(ba, svc)
			instanceMutex.Unlock()
		}
		instanceMutex.Lock()
		monitoringEnabledLabelName := getMonitoringEnabledLabelName(ba)
		if instance.Spec.Monitoring != nil {
			instanceMutex.Unlock()
			svc.Labels[monitoringEnabledLabelName] = "true"
		} else {
			instanceMutex.Unlock()
			delete(svc.Labels, monitoringEnabledLabelName)
		}
		return nil
	})
	if err != nil {
		reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile Service"}
		return
	}

	instanceMutex.Lock()
	if (ba.GetManageTLS() == nil || *ba.GetManageTLS()) &&
		ba.GetStatus().GetReferences()[common.StatusReferenceCertSecretName] == "" {
		instanceMutex.Unlock()
		reconcileResultChan <- ReconcileResult{err: errors.New("Failed to generate TLS certificate. Ensure cert-manager is installed and running"), condition: common.StatusConditionTypeReconciled}
		return
	} else {
		instanceMutex.Unlock()
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileNetworkPolicy(defaultMeta metav1.ObjectMeta, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
	networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: defaultMeta}
	instanceMutex.Lock()
	if np := instance.Spec.NetworkPolicy; np == nil || np != nil && !np.IsDisabled() {
		err := r.CreateOrUpdate(networkPolicy, instance, func() error {
			oputils.CustomizeNetworkPolicy(networkPolicy, r.IsOpenShift(), instance)
			// add liberty proxy to ingress
			// if len(networkPolicy.Spec.Ingress) > 0 {
			// 	networkPolicy.Spec.Ingress[0].From = append(networkPolicy.Spec.Ingress[0].From, networkingv1.NetworkPolicyPeer{
			// 		NamespaceSelector: &metav1.LabelSelector{
			// 			MatchLabels: map[string]string{
			// 				"kubernetes.io/metadata.name": "openshift-operators",
			// 			},
			// 		},
			// 		PodSelector: &metav1.LabelSelector{
			// 			MatchLabels: map[string]string{
			// 				"app.kubernetes.io/managed-by": "open-liberty-operator",
			// 				"app.kubernetes.io/name":       "liberty-proxy",
			// 			},
			// 		},
			// 	})

			// }
			return nil
		})
		instanceMutex.Unlock()
		if err != nil {
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile network policy"}
			return
		}
	} else {
		instanceMutex.Unlock()
		if err := r.DeleteResource(networkPolicy); err != nil {
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete network policy"}
			return
		}
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileServiceability(instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reqLogger logr.Logger, reconcileResultChan chan<- ReconcileResult) {
	instanceMutex.Lock()
	if instance.Spec.Serviceability != nil {
		if instance.Spec.Serviceability.VolumeClaimName != "" {
			pvcName := instance.Spec.Serviceability.VolumeClaimName
			err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: pvcName, Namespace: instance.Namespace}, &corev1.PersistentVolumeClaim{})
			instanceMutex.Unlock()
			if err != nil && kerrors.IsNotFound(err) {
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to find PersistentVolumeClaim " + pvcName + " in namespace " + instance.Namespace}
				return
			}
		} else {
			err := r.CreateOrUpdate(lutils.CreateServiceabilityPVC(instance), nil, func() error {
				return nil
			})
			instanceMutex.Unlock()
			if err != nil {
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to create PersistentVolumeClaim for Serviceability"}
				return
			}
		}
	} else {
		r.deletePVC(reqLogger, instance.Name+"-serviceability", instance.Namespace)
		instanceMutex.Unlock()
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileBindings(instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
	instanceMutex.Lock()
	err := r.ReconcileBindings(instance)
	instanceMutex.Unlock()
	if err != nil {
		reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled}
		return
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcilePasswordEncryptionKeyConcurrent(instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata, sharedResourceReconcileResultChan chan<- ReconcileResult, lastRotationChan chan<- string, encryptionSecretNameChan chan<- string) {
	// Manage the shared password encryption key Secret if it exists
	instanceMutex.Lock()
	message, encryptionSecretName, passwordEncryptionKeyLastRotation, err := r.reconcilePasswordEncryptionKey(instance, passwordEncryptionMetadata)
	instanceMutex.Unlock()
	lastRotationChan <- passwordEncryptionKeyLastRotation
	encryptionSecretNameChan <- encryptionSecretName
	if err != nil {
		sharedResourceReconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: message}
		return
	}
	sharedResourceReconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled, message: message}
}

func (r *ReconcileOpenLiberty) reconcileLTPAKeysConcurrent(operatorNamespace string, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, ltpaKeysMetadata *lutils.LTPAMetadata, ltpaConfigMetadata *lutils.LTPAMetadata, reconcileResultChan chan<- ReconcileResult, lastRotationChan chan<- string, ltpaSecretNameChan chan<- string, ltpaKeysLastRotationChan chan<- string) {
	// Create and manage the shared LTPA keys Secret if the feature is enabled
	var err error
	var message string
	if ltpaKeysMetadata != nil {
		instanceMutex.Lock()
		ltpaMessage, ltpaSecretName, ltpaKeysLastRotation, ltpaErr := r.reconcileLTPAKeys(operatorNamespace, instance, ltpaKeysMetadata)
		instanceMutex.Unlock()
		err = ltpaErr
		message = ltpaMessage
		ltpaSecretNameChan <- ltpaSecretName
		lastRotationChan <- ltpaKeysLastRotation
		ltpaKeysLastRotationChan <- ltpaKeysLastRotation
	} else {
		ltpaSecretNameChan <- ""
		lastRotationChan <- ""
		ltpaKeysLastRotationChan <- ""
		err = nil
		message = ""
	}
	reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: message}
}

func (r *ReconcileOpenLiberty) reconcileLTPAConfigConcurrent(operatorNamespace string, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, ltpaKeysMetadata *lutils.LTPAMetadata, ltpaConfigMetadata *lutils.LTPAMetadata, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata, reconcileResultChan chan<- ReconcileResult, sharedResourceReconcileResultChan <-chan ReconcileResult, lastRotationChan <-chan string, ltpaKeysLastRotationChan <-chan string, ltpaXMLSecretNameChan chan<- string) {
	// there are two shared resources this function depends on: LTPA and PasswordEncryption
	for i := 0; i < 2; i++ {
		sharedResourceReconcileResult := <-sharedResourceReconcileResultChan
		if sharedResourceReconcileResult.err != nil {
			<-lastRotationChan
			<-lastRotationChan
			<-ltpaKeysLastRotationChan
			ltpaXMLSecretNameChan <- "" // flush with dummy data
			reconcileResultChan <- sharedResourceReconcileResult
			return
		}
	}

	// Block for the shared resources's lastRotation times
	lastRotationVal1 := <-lastRotationChan
	lastRotationVal2 := <-lastRotationChan
	ltpaKeysLastRotation := <-ltpaKeysLastRotationChan

	// get the last key-related rotation time as a string to be used by reconcileLTPAConfig for non-leaders to yield (blocking) to the LTPA config leader
	lastKeyRelatedRotation, err := lutils.GetMaxTime(lastRotationVal1, lastRotationVal2)
	if err != nil {
		ltpaXMLSecretNameChan <- "" // flush with dummy data
		reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled}
		return
	}

	// Using the LTPA keys and config metadata, create and manage the shared LTPA Liberty server XML if the feature is enabled
	if ltpaConfigMetadata != nil {
		instanceMutex.Lock()
		message, ltpaXMLSecretName, err := r.reconcileLTPAConfig(operatorNamespace, instance, ltpaKeysMetadata, ltpaConfigMetadata, passwordEncryptionMetadata, ltpaKeysLastRotation, lastKeyRelatedRotation)
		instanceMutex.Unlock()
		ltpaXMLSecretNameChan <- ltpaXMLSecretName
		if err != nil {
			reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled, message: message}
			return
		}
	} else {
		ltpaXMLSecretNameChan <- ""
	}

	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileStatefulSetDeployment(defaultMeta metav1.ObjectMeta, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, ltpaConfigMetadata *lutils.LTPAMetadata, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata, reconcileResultChan chan<- ReconcileResult, sharedResourceHandoffReconcileResultChan <-chan ReconcileResult, encryptionSecretNameChan <-chan string, ltpaSecretNameChan <-chan string, ltpaXMLSecretNameChan <-chan string) {
	sharedResourceHandoffResult := <-sharedResourceHandoffReconcileResultChan
	encryptionSecretName := <-encryptionSecretNameChan
	ltpaSecretName := <-ltpaSecretNameChan
	ltpaXMLSecretName := <-ltpaXMLSecretNameChan
	if sharedResourceHandoffResult.err != nil {
		reconcileResultChan <- sharedResourceHandoffResult
		return
	}

	instanceMutex.Lock()
	if instance.Spec.StatefulSet != nil {
		instanceMutex.Unlock()
		// Delete Deployment if exists
		deploy := &appsv1.Deployment{ObjectMeta: defaultMeta}
		err := r.DeleteResource(deploy)

		if err != nil {
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete Deployment"}
			return
		}
		instanceMutex.Lock()
		svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: instance.Name + "-headless", Namespace: instance.Namespace}}
		err = r.CreateOrUpdate(svc, instance, func() error {
			oputils.CustomizeService(svc, instance)
			svc.Spec.ClusterIP = corev1.ClusterIPNone
			svc.Spec.Type = corev1.ServiceTypeClusterIP
			return nil
		})
		instanceMutex.Unlock()
		if err != nil {
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile headless Service"}
			return
		}

		statefulSet := &appsv1.StatefulSet{ObjectMeta: defaultMeta}
		capturedSubError := false
		instanceMutex.Lock()
		err = r.CreateOrUpdate(statefulSet, instance, func() error {
			oputils.CustomizeStatefulSet(statefulSet, instance)
			oputils.CustomizePodSpec(&statefulSet.Spec.Template, instance)
			oputils.CustomizePersistence(statefulSet, instance)
			if err := lutils.CustomizeLibertyEnv(&statefulSet.Spec.Template, instance, r.GetClient()); err != nil {
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile StatefulSet; Failed to reconcile Liberty env, error: " + err.Error()}
				capturedSubError = true
				return err
			}

			statefulSet.Spec.Template.Spec.Containers[0].Args = r.getSemeruJavaOptions(instance)

			if err := oputils.CustomizePodWithSVCCertificate(&statefulSet.Spec.Template, instance, r.GetClient()); err != nil {
				return err
			}

			lutils.CustomizeLibertyAnnotations(&statefulSet.Spec.Template, instance)
			if instance.Spec.SSO != nil {
				err = lutils.CustomizeEnvSSO(&statefulSet.Spec.Template, instance, r.GetClient(), r.IsOpenShift())
				if err != nil {
					reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile StatefulSet; Failed to reconcile Single sign-on configuration"}
					capturedSubError = true
					return err
				}
			}
			lutils.ConfigureServiceability(&statefulSet.Spec.Template, instance)
			semeruCertVolume := getSemeruCertVolume(instance)
			if r.isSemeruEnabled(instance) && semeruCertVolume != nil {
				statefulSet.Spec.Template.Spec.Volumes = append(statefulSet.Spec.Template.Spec.Volumes, *semeruCertVolume)
				statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts = append(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts,
					getSemeruCertVolumeMount(instance))
				semeruTLSSecretName := instance.Status.SemeruCompiler.TLSSecretName
				err := lutils.AddSecretResourceVersionAsEnvVar(&statefulSet.Spec.Template, instance, r.GetClient(),
					semeruTLSSecretName, "SEMERU_TLS")
				if err != nil {
					return err
				}
			}

			if r.isPasswordEncryptionKeySharingEnabled(instance) && len(encryptionSecretName) > 0 {
				lutils.ConfigurePasswordEncryption(&statefulSet.Spec.Template, instance, OperatorShortName, passwordEncryptionMetadata)
				lastRotationAnnotation, err := lutils.GetSecretLastRotationAsLabelMap(instance, r.GetClient(), encryptionSecretName, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME)
				if err != nil {
					return err
				}
				lutils.AddPodTemplateSpecAnnotation(&statefulSet.Spec.Template, lastRotationAnnotation)
				if instance.Status.GetReferences()[lutils.GetTrackedResourceName(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME)] != encryptionSecretName {
					instance.Status.SetReference(lutils.GetTrackedResourceName(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME), encryptionSecretName)
				}
			} else {
				lutils.RemovePodTemplateSpecAnnotationByKey(&statefulSet.Spec.Template, lutils.GetLastRotationLabelKey(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME))
				lutils.RemoveMapElementByKey(instance.Status.GetReferences(), lutils.GetTrackedResourceName(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME))
			}

			if r.isLTPAKeySharingEnabled(instance) && len(ltpaSecretName) > 0 {
				lutils.ConfigureLTPAConfig(&statefulSet.Spec.Template, instance, OperatorShortName, ltpaSecretName, ltpaConfigMetadata.Name)
				// add LTPA key last rotation annotation
				lastRotationAnnotation, err := lutils.GetSecretLastRotationAsLabelMap(instance, r.GetClient(), ltpaSecretName, LTPA_RESOURCE_SHARING_FILE_NAME)
				if err != nil {
					return err
				}
				lutils.AddPodTemplateSpecAnnotation(&statefulSet.Spec.Template, lastRotationAnnotation)
				// add LTPA config last rotation annotation
				configLastRotationAnnotation, err := lutils.GetSecretLastRotationLabel(instance, r.GetClient(), ltpaXMLSecretName, LTPA_CONFIG_RESOURCE_SHARING_FILE_NAME)
				if err != nil {
					return err
				}
				lutils.AddPodTemplateSpecAnnotation(&statefulSet.Spec.Template, configLastRotationAnnotation)
				if instance.Status.GetReferences()[lutils.GetTrackedResourceName(LTPA_RESOURCE_SHARING_FILE_NAME)] != ltpaSecretName {
					instance.Status.SetReference(lutils.GetTrackedResourceName(LTPA_RESOURCE_SHARING_FILE_NAME), ltpaSecretName)
				}
			} else {
				lutils.RemovePodTemplateSpecAnnotationByKey(&statefulSet.Spec.Template, lutils.GetLastRotationLabelKey(LTPA_RESOURCE_SHARING_FILE_NAME))
				lutils.RemoveMapElementByKey(instance.Status.GetReferences(), lutils.GetTrackedResourceName(LTPA_RESOURCE_SHARING_FILE_NAME))
			}
			return nil
		})
		instanceMutex.Unlock()
		if err != nil {
			if !capturedSubError {
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile StatefulSet"}
			}
			return
		}

	} else {
		instanceMutex.Unlock()
		// Delete StatefulSet if exists
		statefulSet := &appsv1.StatefulSet{ObjectMeta: defaultMeta}
		err := r.DeleteResource(statefulSet)
		if err != nil {
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete StatefulSet"}
			return
		}

		// Delete StatefulSet if exists
		headlesssvc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: defaultMeta.Name + "-headless", Namespace: defaultMeta.Namespace}}
		err = r.DeleteResource(headlesssvc)

		if err != nil {
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete headless Service"}
			return
		}
		capturedSubError := false
		deploy := &appsv1.Deployment{ObjectMeta: defaultMeta}
		instanceMutex.Lock()
		err = r.CreateOrUpdate(deploy, instance, func() error {
			oputils.CustomizeDeployment(deploy, instance)
			oputils.CustomizePodSpec(&deploy.Spec.Template, instance)
			if err := lutils.CustomizeLibertyEnv(&deploy.Spec.Template, instance, r.GetClient()); err != nil {
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile Deployment; Failed to reconcile Liberty env, error: " + err.Error()}
				capturedSubError = true
				return err
			}
			deploy.Spec.Template.Spec.Containers[0].Args = r.getSemeruJavaOptions(instance)

			if err := oputils.CustomizePodWithSVCCertificate(&deploy.Spec.Template, instance, r.GetClient()); err != nil {
				return err
			}
			lutils.CustomizeLibertyAnnotations(&deploy.Spec.Template, instance)
			if instance.Spec.SSO != nil {
				err = lutils.CustomizeEnvSSO(&deploy.Spec.Template, instance, r.GetClient(), r.IsOpenShift())
				if err != nil {
					reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile Deployment; Failed to reconcile Single sign-on configuration"}
					capturedSubError = true
					return err
				}
			}

			lutils.ConfigureServiceability(&deploy.Spec.Template, instance)
			semeruCertVolume := getSemeruCertVolume(instance)
			if r.isSemeruEnabled(instance) && semeruCertVolume != nil {
				deploy.Spec.Template.Spec.Volumes = append(deploy.Spec.Template.Spec.Volumes, *semeruCertVolume)
				deploy.Spec.Template.Spec.Containers[0].VolumeMounts = append(deploy.Spec.Template.Spec.Containers[0].VolumeMounts,
					getSemeruCertVolumeMount(instance))
				semeruTLSSecretName := instance.Status.SemeruCompiler.TLSSecretName
				err := lutils.AddSecretResourceVersionAsEnvVar(&deploy.Spec.Template, instance, r.GetClient(),
					semeruTLSSecretName, "SEMERU_TLS")
				if err != nil {
					return err
				}
			}

			if r.isPasswordEncryptionKeySharingEnabled(instance) && len(encryptionSecretName) > 0 {
				lutils.ConfigurePasswordEncryption(&deploy.Spec.Template, instance, OperatorShortName, passwordEncryptionMetadata)
				lastRotationAnnotation, err := lutils.GetSecretLastRotationAsLabelMap(instance, r.GetClient(), encryptionSecretName, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME)
				if err != nil {
					return err
				}
				lutils.AddPodTemplateSpecAnnotation(&deploy.Spec.Template, lastRotationAnnotation)
				if instance.Status.GetReferences()[lutils.GetTrackedResourceName(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME)] != encryptionSecretName {
					instance.Status.SetReference(lutils.GetTrackedResourceName(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME), encryptionSecretName)
				}
			} else {
				lutils.RemovePodTemplateSpecAnnotationByKey(&deploy.Spec.Template, lutils.GetLastRotationLabelKey(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME))
				lutils.RemoveMapElementByKey(instance.Status.GetReferences(), lutils.GetTrackedResourceName(PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME))
			}

			if r.isLTPAKeySharingEnabled(instance) && len(ltpaSecretName) > 0 {
				lutils.ConfigureLTPAConfig(&deploy.Spec.Template, instance, OperatorShortName, ltpaSecretName, ltpaConfigMetadata.Name)
				// add LTPA key last rotation annotation
				lastRotationAnnotation, err := lutils.GetSecretLastRotationAsLabelMap(instance, r.GetClient(), ltpaSecretName, LTPA_RESOURCE_SHARING_FILE_NAME)
				if err != nil {
					return err
				}
				lutils.AddPodTemplateSpecAnnotation(&deploy.Spec.Template, lastRotationAnnotation)
				// add LTPA config last rotation annotation
				configLastRotationAnnotation, err := lutils.GetSecretLastRotationLabel(instance, r.GetClient(), ltpaXMLSecretName, LTPA_CONFIG_RESOURCE_SHARING_FILE_NAME)
				if err != nil {
					return err
				}
				lutils.AddPodTemplateSpecAnnotation(&deploy.Spec.Template, configLastRotationAnnotation)
				if instance.Status.GetReferences()[lutils.GetTrackedResourceName(LTPA_RESOURCE_SHARING_FILE_NAME)] != ltpaSecretName {
					instance.Status.SetReference(lutils.GetTrackedResourceName(LTPA_RESOURCE_SHARING_FILE_NAME), ltpaSecretName)
				}
			} else {
				lutils.RemovePodTemplateSpecAnnotationByKey(&deploy.Spec.Template, lutils.GetLastRotationLabelKey(LTPA_RESOURCE_SHARING_FILE_NAME))
				lutils.RemoveMapElementByKey(instance.Status.GetReferences(), lutils.GetTrackedResourceName(LTPA_RESOURCE_SHARING_FILE_NAME))
			}
			return nil
		})
		instanceMutex.Unlock()
		if err != nil {
			if !capturedSubError {
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile Deployment"}
			}
			// otherwise return since the error is already captured in inside r.CreateOrUpdate()
			return
		}

	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileAutoscaling(defaultMeta metav1.ObjectMeta, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
	instanceMutex.Lock()
	if instance.Spec.Autoscaling != nil {
		instanceMutex.Unlock()
		hpa := &autoscalingv1.HorizontalPodAutoscaler{ObjectMeta: defaultMeta}
		instanceMutex.Lock()
		err := r.CreateOrUpdate(hpa, instance, func() error {
			oputils.CustomizeHPA(hpa, instance)
			return nil
		})
		instanceMutex.Unlock()

		if err != nil {
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile HorizontalPodAutoscaler"}
			return
		}
	} else {
		instanceMutex.Unlock()
		hpa := &autoscalingv1.HorizontalPodAutoscaler{ObjectMeta: defaultMeta}
		err := r.DeleteResource(hpa)
		if err != nil {
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete HorizontalPodAutoscaler"}
			return
		}
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileRouteIngress(defaultMeta metav1.ObjectMeta, ba common.BaseComponent, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reqLogger logr.Logger, reconcileResultChan chan<- ReconcileResult) {
	if ok, err := r.IsGroupVersionSupported(routev1.SchemeGroupVersion.String(), "Route"); err != nil {
		reqLogger.Error(err, fmt.Sprintf("Failed to check if %s is supported", routev1.SchemeGroupVersion.String()))
		// r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	} else if ok {
		instanceMutex.Lock()
		if instance.Spec.Expose != nil && *instance.Spec.Expose {
			if oputils.ShouldDeleteRoute(ba) {
				instanceMutex.Unlock()
				// reqLogger.Info("Custom hostname has been removed from route, deleting and recreating the route")
				route := &routev1.Route{ObjectMeta: defaultMeta}
				err = r.DeleteResource(route)
				if err != nil {
					reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete Route when the Custom hostname has been removed"}
					return
				}
			} else {
				instanceMutex.Unlock()
			}
			route := &routev1.Route{ObjectMeta: defaultMeta}
			instanceMutex.Lock()
			err = r.CreateOrUpdate(route, instance, func() error {
				key, cert, caCert, destCACert, err := r.GetRouteTLSValues(ba)
				if err != nil {
					return err
				}
				oputils.CustomizeRoute(route, instance, key, cert, caCert, destCACert)

				return nil
			})
			instanceMutex.Unlock()
			if err != nil {
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile Route"}
				return
			}

		} else {
			instanceMutex.Unlock()
			route := &routev1.Route{ObjectMeta: defaultMeta}
			err = r.DeleteResource(route)
			if err != nil {
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete Route"}
				return
			}
		}
	} else {
		if ok, err := r.IsGroupVersionSupported(networkingv1.SchemeGroupVersion.String(), "Ingress"); err != nil {
			reqLogger.Error(err, fmt.Sprintf("Failed to check if %s is supported", networkingv1.SchemeGroupVersion.String()))
			// r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		} else if ok {
			instanceMutex.Lock()
			if instance.Spec.Expose != nil && *instance.Spec.Expose {
				ing := &networkingv1.Ingress{ObjectMeta: defaultMeta}
				err = r.CreateOrUpdate(ing, instance, func() error {
					oputils.CustomizeIngress(ing, instance)
					return nil
				})
				instanceMutex.Unlock()
				if err != nil {
					reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile Ingress"}
					return
				}
			} else {
				instanceMutex.Unlock()
				ing := &networkingv1.Ingress{ObjectMeta: defaultMeta}
				err = r.DeleteResource(ing)
				if err != nil {
					reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete Ingress"}
					return
				}
			}
		}
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileServiceMonitor(defaultMeta metav1.ObjectMeta, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reqLogger logr.Logger, reconcileResultChan chan<- ReconcileResult) {
	if ok, err := r.IsGroupVersionSupported(prometheusv1.SchemeGroupVersion.String(), "ServiceMonitor"); err != nil {
		reqLogger.Error(err, fmt.Sprintf("Failed to check if %s is supported", prometheusv1.SchemeGroupVersion.String()))
		// r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	} else if ok {
		instanceMutex.Lock()
		if instance.Spec.Monitoring != nil && (instance.Spec.CreateKnativeService == nil || !*instance.Spec.CreateKnativeService) {
			// Validate the monitoring endpoints' configuration before creating/updating the ServiceMonitor
			if err := oputils.ValidatePrometheusMonitoringEndpoints(instance, r.GetClient(), instance.GetNamespace()); err != nil {
				instanceMutex.Unlock()
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled}
				return
			} else {
				instanceMutex.Unlock()
			}
			sm := &prometheusv1.ServiceMonitor{ObjectMeta: defaultMeta}
			instanceMutex.Lock()
			err = r.CreateOrUpdate(sm, instance, func() error {
				oputils.CustomizeServiceMonitor(sm, instance)
				return nil
			})
			instanceMutex.Unlock()
			if err != nil {
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile ServiceMonitor"}
				return
			}
		} else {
			instanceMutex.Unlock()
			sm := &prometheusv1.ServiceMonitor{ObjectMeta: defaultMeta}
			err = r.DeleteResource(sm)
			if err != nil {
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete ServiceMonitor"}
				return
			}
		}
	} else {
		reqLogger.V(1).Info(fmt.Sprintf("%s is not supported", prometheusv1.SchemeGroupVersion.String()))
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileSemeruCloudCompilerCleanup(instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult, semeruMarkedForDeletionChan <-chan bool) {
	// Delete completed Semeru instances because all pods now point to the newest Semeru service
	areCompletedSemeruInstancesMarkedToBeDeleted := <-semeruMarkedForDeletionChan

	if areCompletedSemeruInstancesMarkedToBeDeleted {
		instanceMutex.Lock()
		if r.isOpenLibertyApplicationReady(instance) {
			if err := r.deleteCompletedSemeruInstances(instance); err != nil {
				instanceMutex.Unlock()
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete completed Semeru instance"}
				return
			} else {
				instanceMutex.Unlock()
			}
		} else {
			instanceMutex.Unlock()
		}
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) concurrentReconcile(operatorNamespace string, ba common.BaseComponent, instance *olv1.OpenLibertyApplication, reqLogger logr.Logger, isKnativeSupported bool, ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	defaultMeta := metav1.ObjectMeta{
		Name:      instance.Name,
		Namespace: instance.Namespace,
	}

	imageReferenceOld := instance.Status.ImageReference
	instance.Status.ImageReference = instance.Spec.ApplicationImage

	reconcileResultChan := make(chan ReconcileResult, 9)
	instanceMutex := &sync.Mutex{}

	go r.reconcileImageStream(instance, instanceMutex, reconcileResultChan) // STATE: {reconcileResultChan: 1}

	// The if statement below depends on instance.Status.ImageReference being possibly set in reconcileImageStream, so it must block for the first reconcile result
	reconcileResult := <-reconcileResultChan // STATE: {}
	if reconcileResult.err != nil {
		return r.ManageError(reconcileResult.err, reconcileResult.condition, instance)
	}

	instanceMutex.Lock()
	if imageReferenceOld != instance.Status.ImageReference {
		// Trigger a new Semeru Cloud Compiler generation
		createNewSemeruGeneration(instance)

		reqLogger.Info("Updating status.imageReference", "status.imageReference", instance.Status.ImageReference)
		err := r.UpdateStatus(instance)
		if err != nil {
			reqLogger.Error(err, "Error updating Open Liberty application status")
			instanceMutex.Unlock()
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	}
	instanceMutex.Unlock()

	go r.reconcileLibertyProxyConcurrent(operatorNamespace, reconcileResultChan) // STATE: {reconcileResultChan: 1}
	// obtain ltpa keys and config metadata
	ltpaMetadataChan := make(chan *lutils.LTPAMetadata, 2)
	go r.reconcileLTPAKeySharingEnabled(instance, instanceMutex, reconcileResultChan, ltpaMetadataChan) // STATE: {reconcileResultChan: 2, ltpaMetadataChan: 2}

	// obtain password encryption metadata
	passwordEncryptionMetadataChan := make(chan *lutils.PasswordEncryptionMetadata, 1)
	go r.reconcilePasswordEncryptionKeySharingEnabled(instance, instanceMutex, reconcileResultChan, passwordEncryptionMetadataChan) // STATE: {reconcileResultChan: 3, ltpaMetadataChan: 2, passwordEncryptionMetadataChan: 1}
	go r.reconcileServiceAccount(defaultMeta, instance, instanceMutex, reconcileResultChan)                                         // STATE: {reconcileResultChan: 4, ltpaMetadataChan: 2, passwordEncryptionMetadataChan: 1}

	semeruMarkedForDeletionChan := make(chan bool, 1)
	go r.reconcileSemeruCloudCompilerInit(instance, instanceMutex, reconcileResultChan, semeruMarkedForDeletionChan) // STATE: {reconcileResultChan: 5, ltpaMetadataChan: 2, passwordEncryptionMetadataChan: 1, semeruMarkedForDeletionChan: 1}

	// FRONTIER: knative service should have option to exit the reconcile loop
	res, err := r.reconcileKnativeServiceSequential(defaultMeta, instance, instanceMutex, reqLogger, isKnativeSupported)
	if err != nil {
		// block to pull from all go routines before exiting reconcile
		<-ltpaMetadataChan               // STATE: {reconcileResultChan: 5, ltpaMetadataChan: 1, passwordEncryptionMetadataChan: 1, semeruMarkedForDeletionChan: 1}
		<-ltpaMetadataChan               // STATE: {reconcileResultChan: 5, passwordEncryptionMetadataChan: 1, semeruMarkedForDeletionChan: 1}
		<-passwordEncryptionMetadataChan // STATE: {reconcileResultChan: 5, semeruMarkedForDeletionChan: 1}
		<-semeruMarkedForDeletionChan    // STATE: {reconcileResultChan: 5}

		reconcileResults := 5
		foundFirstError := false
		var firstErroringReconcileResult ReconcileResult
		for i := 0; i < reconcileResults; i++ {
			reconcileResult := <-reconcileResultChan
			if !foundFirstError && reconcileResult.err != nil {
				foundFirstError = true
				firstErroringReconcileResult = reconcileResult
			}
		}
		// STATE: {}
		if foundFirstError {
			return r.ManageError(firstErroringReconcileResult.err, firstErroringReconcileResult.condition, instance)
		}
		return res, err
	}

	ltpaKeysLastRotationChan := make(chan string, 1)
	lastRotationChan := make(chan string, 2) // order doesn't matter, just need the latest rotation time

	encryptionSecretNameChan := make(chan string, 1)
	ltpaSecretNameChan := make(chan string, 1)
	ltpaXMLSecretNameChan := make(chan string, 1)

	ltpaKeysMetadata := <-ltpaMetadataChan
	ltpaConfigMetadata := <-ltpaMetadataChan
	passwordEncryptionMetadata := <-passwordEncryptionMetadataChan

	sharedResourceReconcileResultChan := make(chan ReconcileResult,
		2) // reconcilePasswordEncryptionKeyConcurrent() and reconcileLTPAKeysConcurrent() write to this chan
	sharedResourceHandoffReconcileResultChan := make(chan ReconcileResult,
		1) // reconcileLTPAConfigConcurrent() reads from sharedResourceReconcileResultChan and writes to this chan

	useCertManagerChan := make(chan bool, 1)
	go r.reconcileServiceCertificate(ba, instance, instanceMutex, reconcileResultChan, useCertManagerChan)                                                                                                                // STATE: {reconcileResultChan: 6, semeruMarkedForDeletionChan: 1, useCertManagerChan: 1}
	go r.reconcilePasswordEncryptionKeyConcurrent(instance, instanceMutex, passwordEncryptionMetadata, sharedResourceReconcileResultChan, lastRotationChan, encryptionSecretNameChan)                                     // STATE: {reconcileResultChan: 6, semeruMarkedForDeletionChan: 1, useCertManagerChan: 1, sharedResourceReconcileResultChan: 1, lastRotationChan: 1, encryptionSecretNameChan: 1}
	go r.reconcileLTPAKeysConcurrent(operatorNamespace, instance, instanceMutex, ltpaKeysMetadata, ltpaConfigMetadata, sharedResourceReconcileResultChan, lastRotationChan, ltpaSecretNameChan, ltpaKeysLastRotationChan) // STATE: {reconcileResultChan: 6, semeruMarkedForDeletionChan: 1, useCertManagerChan: 1, sharedResourceReconcileResultChan: 2, lastRotationChan: 2, ltpaKeysLastRotationChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1}
	go r.reconcileLTPAConfigConcurrent(operatorNamespace, instance, instanceMutex, ltpaKeysMetadata, ltpaConfigMetadata, passwordEncryptionMetadata, sharedResourceHandoffReconcileResultChan, sharedResourceReconcileResultChan,
		lastRotationChan, ltpaKeysLastRotationChan, ltpaXMLSecretNameChan) // STATE: {reconcileResultChan: 5, semeruMarkedForDeletionChan: 1, useCertManagerChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	go r.reconcileSemeruCloudCompilerReady(instance, instanceMutex, reconcileResultChan)                     // STATE: {reconcileResultChan: 7, useCertManagerChan: 1, semeruMarkedForDeletionChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	go r.reconcileService(defaultMeta, ba, instance, instanceMutex, reconcileResultChan, useCertManagerChan) // STATE: {reconcileResultChan: 8, useCertManagerChan: 1,semeruMarkedForDeletionChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	go r.reconcileNetworkPolicy(defaultMeta, instance, instanceMutex, reconcileResultChan)                   // STATE: {reconcileResultChan: 9, semeruMarkedForDeletionChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	go r.reconcileServiceability(instance, instanceMutex, reqLogger, reconcileResultChan)                    // STATE: {reconcileResultChan: 10, semeruMarkedForDeletionChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	go r.reconcileBindings(instance, instanceMutex, reconcileResultChan)                                     // STATE: {reconcileResultChan: 11, semeruMarkedForDeletionChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}

	// FRONTIER: instances shouldn't proceed past if they are waiting for certificate creation
	reconcileResults := 11
	foundFirstError := false
	var firstErroringReconcileResult ReconcileResult
	for i := 0; i < reconcileResults; i++ {
		reconcileResult := <-reconcileResultChan
		if !foundFirstError && reconcileResult.err != nil {
			foundFirstError = true
			firstErroringReconcileResult = reconcileResult
		}
	}

	// STATE: {semeruMarkedForDeletionChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	if foundFirstError {
		<-semeruMarkedForDeletionChan              // STATE:  {sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
		<-sharedResourceHandoffReconcileResultChan // STATE:  {encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
		<-encryptionSecretNameChan                 // STATE:  {ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
		<-ltpaSecretNameChan                       // STATE:  {ltpaXMLSecretNameChan: 1}
		<-ltpaXMLSecretNameChan                    // STATE: {}
		return r.ManageError(firstErroringReconcileResult.err, firstErroringReconcileResult.condition, instance)
	}

	go r.reconcileStatefulSetDeployment(defaultMeta, instance, instanceMutex, ltpaConfigMetadata, passwordEncryptionMetadata, reconcileResultChan, sharedResourceHandoffReconcileResultChan, encryptionSecretNameChan, ltpaSecretNameChan, ltpaXMLSecretNameChan) // STATE: {reconcileResultChan: 1, semeruMarkedForDeletionChan: 1}

	// FRONTIER: past this point, it doesn't make sense to manage the route when the statefulset/deployment might possibly not exist, so block until completion
	// STATE: {reconcileResultChan: 1, semeruMarkedForDeletionChan: 1}
	reconcileResults = 1
	foundFirstError = false
	for i := 0; i < reconcileResults; i++ {
		reconcileResult := <-reconcileResultChan
		if !foundFirstError && reconcileResult.err != nil {
			foundFirstError = true
			firstErroringReconcileResult = reconcileResult
		}
	}

	// STATE: {semeruMarkedForDeletionChan: 1}
	if foundFirstError {
		<-semeruMarkedForDeletionChan // STATE: {}
		return r.ManageError(firstErroringReconcileResult.err, firstErroringReconcileResult.condition, instance)
	}
	// STATE: {semeruMarkedForDeletionChan: 1}
	go r.reconcileAutoscaling(defaultMeta, instance, instanceMutex, reconcileResultChan)                                // STATE: {semeruMarkedForDeletionChan: 1, reconcileResultChan: 1}
	go r.reconcileRouteIngress(defaultMeta, ba, instance, instanceMutex, reqLogger, reconcileResultChan)                // STATE: {semeruMarkedForDeletionChan: 1, reconcileResultChan: 2}
	go r.reconcileServiceMonitor(defaultMeta, instance, instanceMutex, reqLogger, reconcileResultChan)                  // STATE: {semeruMarkedForDeletionChan: 1, reconcileResultChan: 3}
	go r.reconcileSemeruCloudCompilerCleanup(instance, instanceMutex, reconcileResultChan, semeruMarkedForDeletionChan) // STATE: {reconcileResultChan: 4}
	// FRONTIER: pull from all remaining channels to manage success
	reconcileResults = 4
	foundFirstError = false
	for i := 0; i < reconcileResults; i++ {
		reconcileResult := <-reconcileResultChan
		if !foundFirstError && reconcileResult.err != nil {
			foundFirstError = true
			firstErroringReconcileResult = reconcileResult
		}
	}

	// STATE: {}
	if foundFirstError {
		return r.ManageError(firstErroringReconcileResult.err, firstErroringReconcileResult.condition, instance)
	}

	instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
	instance.Status.Versions.Reconciled = lutils.OperandVersion
	reqLogger.Info("Reconcile OpenLibertyApplication - concurrent completed")
	return r.ManageSuccess(common.StatusConditionTypeReconciled, instance)
}
