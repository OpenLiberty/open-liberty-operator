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

func (r *ReconcileOpenLiberty) getEphemeralPodWorkerPoolSize(instance *olv1.OpenLibertyApplication) int {
	if instance.GetExperimental() != nil && instance.GetExperimental().GetEphemeralPodWorkerPoolSize() != nil {
		return *instance.GetExperimental().GetEphemeralPodWorkerPoolSize()
	}
	return 3
}

func (r *ReconcileOpenLiberty) isCachingEnabled(instance *olv1.OpenLibertyApplication) bool {
	if instance.GetExperimental() != nil && instance.GetExperimental().GetManageCache() != nil && *instance.GetExperimental().GetManageCache() {
		return true
	}
	return false
}

func (r *ReconcileOpenLiberty) reconcileImageStream(instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
	instanceMutex.Lock()
	defer instanceMutex.Unlock()

	if r.IsOpenShift() {
		image, err := imageutil.ParseDockerImageReference(instance.Spec.ApplicationImage)
		if err == nil {
			isTag := &imagev1.ImageStreamTag{}
			isTagName := imageutil.JoinImageStreamTag(image.Name, image.Tag)
			isTagNamespace := image.Namespace
			if isTagNamespace == "" {
				isTagNamespace = instance.Namespace
			}
			key := types.NamespacedName{Name: isTagName, Namespace: isTagNamespace}
			err = r.GetAPIReader().Get(context.Background(), key, isTag)
			// Call ManageError only if the error type is not found or is not forbidden. Forbidden could happen
			// when the operator tries to call GET for ImageStreamTags on a namespace that doesn't exists (e.g.
			// cannot get imagestreamtags.image.openshift.io in the namespace "navidsh": no RBAC policy matched)
			if err == nil {
				image := isTag.Image
				if image.DockerImageReference != "" {
					instance.Status.ImageReference = image.DockerImageReference
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
	instanceMutex.Lock()
	defer instanceMutex.Unlock()

	// Reconciles the shared LTPA state for the instance namespace
	var ltpaMetadataList *lutils.LTPAMetadataList
	expectedMetadataLength := 2
	if r.isLTPAKeySharingEnabled(instance) {
		leaderMetadataList, err := r.reconcileResourceTrackingState(instance, LTPA_RESOURCE_SHARING_FILE_NAME, r.isCachingEnabled(instance))
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
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcilePasswordEncryptionKeySharingEnabled(instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult, passwordEncryptionMetadataChan chan<- *lutils.PasswordEncryptionMetadata) {
	instanceMutex.Lock()
	defer instanceMutex.Unlock()

	// Reconciles the shared password encryption key state for the instance namespace only if the shared key already exists
	var passwordEncryptionMetadataList *lutils.PasswordEncryptionMetadataList
	passwordEncryptionMetadata := &lutils.PasswordEncryptionMetadata{}
	expectedMetadataLength := 1
	if r.isUsingPasswordEncryptionKeySharing(instance, passwordEncryptionMetadata) {
		leaderMetadataList, err := r.reconcileResourceTrackingState(instance, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, r.isCachingEnabled(instance))
		if err != nil {
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled}
			for i := 0; i < expectedMetadataLength; i++ {
				passwordEncryptionMetadataChan <- nil
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
		// error if the password encryption key sharing is enabled but the Secret is not found
		passwordEncryptionSecretName := lutils.PasswordEncryptionKeyRootName + passwordEncryptionMetadata.Name
		err := errors.Wrapf(fmt.Errorf("Secret %q not found", passwordEncryptionSecretName), "Secret for Password Encryption was not found. Create a secret named %q in namespace %q with the encryption key specified in data field %q.", passwordEncryptionSecretName, instance.GetNamespace(), "passwordEncryptionKey")
		reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled}
		for i := 0; i < expectedMetadataLength; i++ {
			passwordEncryptionMetadataChan <- nil
		}
		return
	}
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
	for i := 0; i < expectedMetadataLength; i++ {
		passwordEncryptionMetadataChan <- nil
	}
}

// func (r *ReconcileOpenLiberty) reconcile(instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
// 	instanceMutex.Lock()
// 	defer instanceMutex.Unlock()
// }

// func (r *ReconcileOpenLiberty) reconcile(instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {

// }

func (r *ReconcileOpenLiberty) reconcileServiceAccount(defaultMeta metav1.ObjectMeta, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
	instanceMutex.Lock()
	defer instanceMutex.Unlock()

	serviceAccountName := oputils.GetServiceAccountName(instance)
	if serviceAccountName != defaultMeta.Name {
		if serviceAccountName == "" {
			serviceAccount := &corev1.ServiceAccount{ObjectMeta: defaultMeta}
			err := r.CreateOrUpdate(serviceAccount, instance, func() error {
				return oputils.CustomizeServiceAccount(serviceAccount, instance, r.GetClient())
			})
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
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) concurrentReconcile(ba common.BaseComponent, instance *olv1.OpenLibertyApplication, reqLogger logr.Logger, isKnativeSupported bool, ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	defaultMeta := metav1.ObjectMeta{
		Name:      instance.Name,
		Namespace: instance.Namespace,
	}

	imageReferenceOld := instance.Status.ImageReference
	instance.Status.ImageReference = instance.Spec.ApplicationImage

	reconcileResultChan := make(chan ReconcileResult, 3)
	instanceMutex := &sync.Mutex{}

	go r.reconcileImageStream(instance, instanceMutex, reconcileResultChan) // // STATE: {reconcileResultChan: 1}

	// obtain ltpa keys and config metadata
	ltpaMetadataChan := make(chan *lutils.LTPAMetadata, 2)
	go r.reconcileLTPAKeySharingEnabled(instance, instanceMutex, reconcileResultChan, ltpaMetadataChan) // STATE: {reconcileResultChan: 2, ltpaMetadataChan: 2}

	// The if statement below depends on instance.Status.ImageReference being possibly set in reconcileImageStream, so it must block for the first reconcile result
	reconcileResult := <-reconcileResultChan // STATE: {reconcileResultChan: 1, ltpaMetadataChan: 2}
	if reconcileResult.err != nil {
		return r.ManageError(reconcileResult.err, reconcileResult.condition, instance)
	}

	// Everything done from here on out will be with an invalid image so this should terminate the parent reconcile and pull all channel data
	if imageReferenceOld != instance.Status.ImageReference {
		ltpaKeysMetadata := <-ltpaMetadataChan // at the very least, block for the first ltpaKeys metadata
		// STATE: {reconcileResultChan: 1, ltpaMetadataChan: 1}

		// Trigger a new Semeru Cloud Compiler generation
		createNewSemeruGeneration(instance)

		// If the shared LTPA keys was not generated from the last application image, restart the key generation process
		if r.isLTPAKeySharingEnabled(instance) {
			if err := r.restartLTPAKeysGeneration(instance, ltpaKeysMetadata); err != nil {
				// clear channels before exiting
				<-reconcileResultChan
				<-ltpaMetadataChan
				reqLogger.Error(err, "Error restarting the LTPA keys generation process")
				return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
			}
		}

		// clear the channels before updating CR status
		<-reconcileResultChan
		<-ltpaMetadataChan

		reqLogger.Info("Updating status.imageReference", "status.imageReference", instance.Status.ImageReference)
		err := r.UpdateStatus(instance)
		if err != nil {
			reqLogger.Error(err, "Error updating Open Liberty application status")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	}
	// obtain password encryption metadata
	passwordEncryptionMetadataChan := make(chan *lutils.PasswordEncryptionMetadata, 1)
	go r.reconcilePasswordEncryptionKeySharingEnabled(instance, instanceMutex, reconcileResultChan, passwordEncryptionMetadataChan) // STATE: {reconcileResultChan: 2, ltpaMetadataChan: 2, passwordEncryptionMetadataChan: 1}

	go r.reconcileServiceAccount(defaultMeta, instance, instanceMutex, reconcileResultChan) // STATE: {reconcileResultChan: 3, ltpaMetadataChan: 2, passwordEncryptionMetadataChan: 1}

	ltpaKeysMetadata := <-ltpaMetadataChan
	ltpaConfigMetadata := <-ltpaMetadataChan
	passwordEncryptionMetadata := <-passwordEncryptionMetadataChan

	reconcileResults := 3
	for i := 0; i < reconcileResults; i++ {
		reconcileResult := <-reconcileResultChan
		if reconcileResult.err != nil {
			return r.ManageError(reconcileResult.err, reconcileResult.condition, instance)
		}
	}

	// Check if the ServiceAccount has a valid pull secret before creating the deployment/statefulset
	// or setting up knative. Otherwise the pods can go into an ImagePullBackOff loop
	saErr := oputils.ServiceAccountPullSecretExists(instance, r.GetClient())
	if saErr != nil {
		return r.ManageError(saErr, common.StatusConditionTypeReconciled, instance)
	}

	// Check if SemeruCloudCompiler is enabled before reconciling the Semeru Compiler deployment and service.
	// Otherwise, delete the Semeru Compiler deployment and service.
	message := "Start Semeru Compiler reconcile"
	reqLogger.Info(message)
	err, message, areCompletedSemeruInstancesMarkedToBeDeleted := r.reconcileSemeruCompiler(instance)
	if err != nil {
		reqLogger.Error(err, message)
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}
	// If semeru compiler is enabled, make sure its ready
	if r.isSemeruEnabled(instance) {
		message = "Check Semeru Compiler resources ready"
		reqLogger.Info(message)
		err = r.areSemeruCompilerResourcesReady(instance)
		if err != nil {
			reqLogger.Error(err, message)
			return r.ManageError(err, common.StatusConditionTypeResourcesReady, instance)
		}
	}

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
		err = r.DeleteResources(resources)
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
		err = r.DeleteResource(ksvc)
		if err != nil {
			reqLogger.Error(err, "Failed to delete Knative Service")
			r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	}

	useCertmanager, err := r.GenerateSvcCertSecret(ba, OperatorShortName, "Open Liberty Operator", OperatorName)
	if err != nil {
		reqLogger.Error(err, "Failed to reconcile CertManager Certificate")
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}
	if ba.GetService().GetCertificateSecretRef() != nil {
		ba.GetStatus().SetReference(common.StatusReferenceCertSecretName, *ba.GetService().GetCertificateSecretRef())
	}

	svc := &corev1.Service{ObjectMeta: defaultMeta}
	err = r.CreateOrUpdate(svc, instance, func() error {
		oputils.CustomizeService(svc, ba)
		svc.Annotations = oputils.MergeMaps(svc.Annotations, instance.Spec.Service.Annotations)
		if !useCertmanager && r.IsOpenShift() {
			oputils.AddOCPCertAnnotation(ba, svc)
		}
		monitoringEnabledLabelName := getMonitoringEnabledLabelName(ba)
		if instance.Spec.Monitoring != nil {
			svc.Labels[monitoringEnabledLabelName] = "true"
		} else {
			delete(svc.Labels, monitoringEnabledLabelName)
		}
		return nil
	})
	if err != nil {
		reqLogger.Error(err, "Failed to reconcile Service")
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	if (ba.GetManageTLS() == nil || *ba.GetManageTLS()) &&
		ba.GetStatus().GetReferences()[common.StatusReferenceCertSecretName] == "" {
		return r.ManageError(errors.New("Failed to generate TLS certificate. Ensure cert-manager is installed and running"),
			common.StatusConditionTypeReconciled, instance)
	}

	networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: defaultMeta}
	if np := instance.Spec.NetworkPolicy; np == nil || np != nil && !np.IsDisabled() {
		err = r.CreateOrUpdate(networkPolicy, instance, func() error {
			oputils.CustomizeNetworkPolicy(networkPolicy, r.IsOpenShift(), instance)
			return nil
		})
		if err != nil {
			reqLogger.Error(err, "Failed to reconcile network policy")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	} else {
		if err := r.DeleteResource(networkPolicy); err != nil {
			reqLogger.Error(err, "Failed to delete network policy")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	}

	if instance.Spec.Serviceability != nil {
		if instance.Spec.Serviceability.VolumeClaimName != "" {
			pvcName := instance.Spec.Serviceability.VolumeClaimName
			err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: pvcName, Namespace: instance.Namespace}, &corev1.PersistentVolumeClaim{})
			if err != nil && kerrors.IsNotFound(err) {
				reqLogger.Error(err, "Failed to find PersistentVolumeClaim "+pvcName+" in namespace "+instance.Namespace)
				return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
			}
		} else {
			err = r.CreateOrUpdate(lutils.CreateServiceabilityPVC(instance), nil, func() error {
				return nil
			})
			if err != nil {
				reqLogger.Error(err, "Failed to create PersistentVolumeClaim for Serviceability")
				return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
			}
		}
	} else {
		r.deletePVC(reqLogger, instance.Name+"-serviceability", instance.Namespace)
	}

	err = r.ReconcileBindings(instance)
	if err != nil {
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	// Manage the shared password encryption key Secret if it exists
	message, encryptionSecretName, passwordEncryptionKeyLastRotation, err := r.reconcilePasswordEncryptionKey(instance, passwordEncryptionMetadata)
	if err != nil {
		reqLogger.Error(err, message)
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	// Create and manage the shared LTPA keys Secret if the feature is enabled
	message, ltpaSecretName, ltpaKeysLastRotation, err := r.reconcileLTPAKeys(instance, ltpaKeysMetadata, ltpaConfigMetadata)
	if err != nil {
		reqLogger.Error(err, message)
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	// get the last key-related rotation time as a string to be used by reconcileLTPAConfig for non-leaders to yield (blocking) to the LTPA config leader
	lastKeyRelatedRotation, err := lutils.GetMaxTime(passwordEncryptionKeyLastRotation, ltpaKeysLastRotation)
	if err != nil {
		reqLogger.Error(err, message)
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	// Using the LTPA keys and config metadata, create and manage the shared LTPA Liberty server XML if the feature is enabled
	message, ltpaXMLSecretName, err := r.reconcileLTPAConfig(instance, ltpaKeysMetadata, ltpaConfigMetadata, passwordEncryptionMetadata, ltpaKeysLastRotation, lastKeyRelatedRotation)
	if err != nil {
		reqLogger.Error(err, message)
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	if instance.Spec.StatefulSet != nil {
		// Delete Deployment if exists
		deploy := &appsv1.Deployment{ObjectMeta: defaultMeta}
		err = r.DeleteResource(deploy)

		if err != nil {
			reqLogger.Error(err, "Failed to delete Deployment")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
		svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: instance.Name + "-headless", Namespace: instance.Namespace}}
		err = r.CreateOrUpdate(svc, instance, func() error {
			oputils.CustomizeService(svc, instance)
			svc.Spec.ClusterIP = corev1.ClusterIPNone
			svc.Spec.Type = corev1.ServiceTypeClusterIP
			return nil
		})
		if err != nil {
			reqLogger.Error(err, "Failed to reconcile headless Service")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}

		statefulSet := &appsv1.StatefulSet{ObjectMeta: defaultMeta}
		err = r.CreateOrUpdate(statefulSet, instance, func() error {
			oputils.CustomizeStatefulSet(statefulSet, instance)
			oputils.CustomizePodSpec(&statefulSet.Spec.Template, instance)
			oputils.CustomizePersistence(statefulSet, instance)
			if err := lutils.CustomizeLibertyEnv(&statefulSet.Spec.Template, instance, r.GetClient()); err != nil {
				reqLogger.Error(err, "Failed to reconcile Liberty env, error: "+err.Error())
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
					reqLogger.Error(err, "Failed to reconcile Single sign-on configuration")
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
		if err != nil {
			reqLogger.Error(err, "Failed to reconcile StatefulSet")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}

	} else {
		// Delete StatefulSet if exists
		statefulSet := &appsv1.StatefulSet{ObjectMeta: defaultMeta}
		err = r.DeleteResource(statefulSet)
		if err != nil {
			reqLogger.Error(err, "Failed to delete Statefulset")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}

		// Delete StatefulSet if exists
		headlesssvc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: instance.Name + "-headless", Namespace: instance.Namespace}}
		err = r.DeleteResource(headlesssvc)

		if err != nil {
			reqLogger.Error(err, "Failed to delete headless Service")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
		deploy := &appsv1.Deployment{ObjectMeta: defaultMeta}
		err = r.CreateOrUpdate(deploy, instance, func() error {
			oputils.CustomizeDeployment(deploy, instance)
			oputils.CustomizePodSpec(&deploy.Spec.Template, instance)
			if err := lutils.CustomizeLibertyEnv(&deploy.Spec.Template, instance, r.GetClient()); err != nil {
				reqLogger.Error(err, "Failed to reconcile Liberty env, error: "+err.Error())
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
					reqLogger.Error(err, "Failed to reconcile Single sign-on configuration")
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
		if err != nil {
			reqLogger.Error(err, "Failed to reconcile Deployment")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}

	}

	if instance.Spec.Autoscaling != nil {
		hpa := &autoscalingv1.HorizontalPodAutoscaler{ObjectMeta: defaultMeta}
		err = r.CreateOrUpdate(hpa, instance, func() error {
			oputils.CustomizeHPA(hpa, instance)
			return nil
		})

		if err != nil {
			reqLogger.Error(err, "Failed to reconcile HorizontalPodAutoscaler")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	} else {
		hpa := &autoscalingv1.HorizontalPodAutoscaler{ObjectMeta: defaultMeta}
		err = r.DeleteResource(hpa)
		if err != nil {
			reqLogger.Error(err, "Failed to delete HorizontalPodAutoscaler")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	}

	if ok, err := r.IsGroupVersionSupported(routev1.SchemeGroupVersion.String(), "Route"); err != nil {
		reqLogger.Error(err, fmt.Sprintf("Failed to check if %s is supported", routev1.SchemeGroupVersion.String()))
		r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	} else if ok {
		if instance.Spec.Expose != nil && *instance.Spec.Expose {
			if oputils.ShouldDeleteRoute(ba) {
				reqLogger.Info("Custom hostname has been removed from route, deleting and recreating the route")
				route := &routev1.Route{ObjectMeta: defaultMeta}
				err = r.DeleteResource(route)
				if err != nil {
					reqLogger.Error(err, "Failed to delete Route")
					return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
				}
			}
			route := &routev1.Route{ObjectMeta: defaultMeta}
			err = r.CreateOrUpdate(route, instance, func() error {
				key, cert, caCert, destCACert, err := r.GetRouteTLSValues(ba)
				if err != nil {
					return err
				}
				oputils.CustomizeRoute(route, instance, key, cert, caCert, destCACert)

				return nil
			})
			if err != nil {
				reqLogger.Error(err, "Failed to reconcile Route")
				return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
			}

		} else {
			route := &routev1.Route{ObjectMeta: defaultMeta}
			err = r.DeleteResource(route)
			if err != nil {
				reqLogger.Error(err, "Failed to delete Route")
				return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
			}
		}
	} else {

		if ok, err := r.IsGroupVersionSupported(networkingv1.SchemeGroupVersion.String(), "Ingress"); err != nil {
			reqLogger.Error(err, fmt.Sprintf("Failed to check if %s is supported", networkingv1.SchemeGroupVersion.String()))
			r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		} else if ok {
			if instance.Spec.Expose != nil && *instance.Spec.Expose {
				ing := &networkingv1.Ingress{ObjectMeta: defaultMeta}
				err = r.CreateOrUpdate(ing, instance, func() error {
					oputils.CustomizeIngress(ing, instance)
					return nil
				})
				if err != nil {
					reqLogger.Error(err, "Failed to reconcile Ingress")
					return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
				}
			} else {
				ing := &networkingv1.Ingress{ObjectMeta: defaultMeta}
				err = r.DeleteResource(ing)
				if err != nil {
					reqLogger.Error(err, "Failed to delete Ingress")
					return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
				}
			}
		}
	}

	if ok, err := r.IsGroupVersionSupported(prometheusv1.SchemeGroupVersion.String(), "ServiceMonitor"); err != nil {
		reqLogger.Error(err, fmt.Sprintf("Failed to check if %s is supported", prometheusv1.SchemeGroupVersion.String()))
		r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	} else if ok {
		if instance.Spec.Monitoring != nil && (instance.Spec.CreateKnativeService == nil || !*instance.Spec.CreateKnativeService) {
			// Validate the monitoring endpoints' configuration before creating/updating the ServiceMonitor
			if err := oputils.ValidatePrometheusMonitoringEndpoints(instance, r.GetClient(), instance.GetNamespace()); err != nil {
				return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
			}
			sm := &prometheusv1.ServiceMonitor{ObjectMeta: defaultMeta}
			err = r.CreateOrUpdate(sm, instance, func() error {
				oputils.CustomizeServiceMonitor(sm, instance)
				return nil
			})
			if err != nil {
				reqLogger.Error(err, "Failed to reconcile ServiceMonitor")
				return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
			}
		} else {
			sm := &prometheusv1.ServiceMonitor{ObjectMeta: defaultMeta}
			err = r.DeleteResource(sm)
			if err != nil {
				reqLogger.Error(err, "Failed to delete ServiceMonitor")
				return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
			}
		}

	} else {
		reqLogger.V(1).Info(fmt.Sprintf("%s is not supported", prometheusv1.SchemeGroupVersion.String()))
	}

	// Delete completed Semeru instances because all pods now point to the newest Semeru service
	if areCompletedSemeruInstancesMarkedToBeDeleted && r.isOpenLibertyApplicationReady(instance) {
		if err := r.deleteCompletedSemeruInstances(instance); err != nil {
			reqLogger.Error(err, "Failed to delete completed Semeru instance")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	}

	instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
	instance.Status.Versions.Reconciled = lutils.OperandVersion
	reqLogger.Info("Reconcile OpenLibertyApplication - completed")
	return r.ManageSuccess(common.StatusConditionTypeReconciled, instance)
}
