package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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

var instances *sync.Map
var skippingForPendingInstanceErr, isSuccessInstanceErr error

func init() {
	isSuccessInstanceErr = fmt.Errorf("is success instance")
	skippingForPendingInstanceErr = fmt.Errorf("skipping for pending instance")
	instances = &sync.Map{}
}

type Instance struct {
	state                    InstanceState
	pendingOnInstanceName    string
	pendingOnInstanceMessage string
}

type InstanceState int

const (
	SUCCESSFUL_INSTANCE InstanceState = iota
	PENDING_INSTANCE    InstanceState = iota
	ERRORING_INSTANCE   InstanceState = iota
)

func SetPendingLTPAInstance(instance *olv1.OpenLibertyApplication, leaderName string, err error) {
	if strings.HasPrefix(err.Error(), "Waiting for OpenLibertyApplication instance") {
		uniqueKey := getInstanceUniqueKey(instance)
		leaderUniqueKey := getInstanceUniqueKeyByValues(leaderName, instance.GetNamespace())
		instances.Store(uniqueKey, &Instance{
			state:                    PENDING_INSTANCE,
			pendingOnInstanceName:    leaderUniqueKey,
			pendingOnInstanceMessage: err.Error(),
		})
	}
}

func getCurrentInstanceState(instance *olv1.OpenLibertyApplication) (string, InstanceState) {
	message := ""
	trueCount := 0
	conditions := instance.Status.GetConditions()
	n := len(conditions)
	for _, cond := range conditions {
		if cond.GetStatus() == corev1.ConditionTrue {
			trueCount += 1
		} else if cond.GetType() == common.StatusConditionTypeReconciled && strings.HasPrefix(cond.GetMessage(), "Waiting for OpenLibertyApplication instance") {
			return message, PENDING_INSTANCE
		}
	}
	if n > 0 && trueCount == n {
		return message, SUCCESSFUL_INSTANCE
	}
	return message, ERRORING_INSTANCE
}

func getInstanceUniqueKey(instance *olv1.OpenLibertyApplication) string {
	return fmt.Sprintf("%s|%s", instance.GetNamespace(), instance.GetName())
}

func getInstanceUniqueKeyByValues(instanceName, instanceNamespace string) string {
	return fmt.Sprintf("%s|%s", instanceNamespace, instanceName)
}

func isBlockingForErroringInstances() bool {
	isBlockingForErroringInstances := false
	instances.Range(func(key, value any) bool {
		if value.(*Instance).state == ERRORING_INSTANCE {
			isBlockingForErroringInstances = true
			return false
		}
		return true
	})
	return isBlockingForErroringInstances
}

func (r *ReconcileOpenLiberty) CleanupInstance(instance *olv1.OpenLibertyApplication) {
	if r.isManagingErroringInstances(instance) {
		fmt.Printf("Finalizing cleanup for instance %s in namespace %s.\n", instance.GetName(), instance.GetNamespace())
		_, instanceState := getCurrentInstanceState(instance)
		uniqueKey := getInstanceUniqueKey(instance)
		currentInstanceObj, found := instances.Load(uniqueKey)
		// if the instance has changed between instanceState and the stored currentInstanceObj.state then update the instance, avoiding pending instances
		if instanceState != PENDING_INSTANCE && (!found || instanceState != currentInstanceObj.(*Instance).state) {
			instances.Store(uniqueKey, &Instance{state: instanceState})
		}
	}
}

func (r *ReconcileOpenLiberty) reconcileManageErroringInstances(instance *olv1.OpenLibertyApplication) (string, error) {
	if r.isManagingErroringInstances(instance) {
		uniqueKey := getInstanceUniqueKey(instance)
		currentInstanceObj, found := instances.Load(uniqueKey)
		if found {
			if currentInstanceObj.(*Instance).state == PENDING_INSTANCE {
				pendingInstanceName := currentInstanceObj.(*Instance).pendingOnInstanceName
				pendingOnInstanceMessage := currentInstanceObj.(*Instance).pendingOnInstanceMessage
				pendingUniqueKey := getInstanceUniqueKeyByValues(pendingInstanceName, instance.GetNamespace())
				pendingInstanceObj, found2 := instances.Load(pendingUniqueKey)
				if found2 {
					if pendingInstanceObj.(*Instance).state == SUCCESSFUL_INSTANCE {
						return pendingOnInstanceMessage, nil // pending instance can proceed because the instance it is pendingOn is sucessful
					}
				}
				return pendingOnInstanceMessage, skippingForPendingInstanceErr // this instance is still waiting on pendingOn instance to be registered in the instances sync.Map or to recover
			} else if currentInstanceObj.(*Instance).state == SUCCESSFUL_INSTANCE {
				// TODO: if this successful instance's generation has drifted from the observedGeneration, then move it to erroring state
				if isBlockingForErroringInstances() {
					return "", isSuccessInstanceErr
				}
			} else if currentInstanceObj.(*Instance).state == ERRORING_INSTANCE {
				return "", nil // no errors because this instance should be reconciled
			}
		}
	}
	return "", nil
}

func (r *ReconcileOpenLiberty) isConcurrencyEnabled(instance *olv1.OpenLibertyApplication) bool {
	if instance.GetExperimental() != nil && instance.GetExperimental().GetManageConcurrency() != nil && *instance.GetExperimental().GetManageConcurrency() {
		return true
	}
	return false
}

func (r *ReconcileOpenLiberty) isManagingErroringInstances(instance *olv1.OpenLibertyApplication) bool {
	if instance.GetExperimental() != nil && instance.GetExperimental().GetManageErroringInstances() != nil {
		return *instance.GetExperimental().GetManageErroringInstances()
	}
	return false
}

func (r *ReconcileOpenLiberty) isCachingEnabled(instance *olv1.OpenLibertyApplication) bool {
	if instance.GetExperimental() != nil && instance.GetExperimental().GetManageCache() != nil && *instance.GetExperimental().GetManageCache() {
		return true
	}
	return false
}

func (r *ReconcileOpenLiberty) reconcileImageStream(reqDebugLogger logr.Logger, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
	start := time.Now()
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
				elapsed := time.Since(start)
				fmt.Printf("-- reconcileImageStream failed with %s\n", elapsed)
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled}
				return
			}
		}
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileImageStream took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileLTPAKeySharingEnabled(reqDebugLogger logr.Logger, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult, ltpaMetadataChan chan<- *lutils.LTPAMetadata) {
	start := time.Now()
	// Reconciles the shared LTPA state for the instance namespace
	var ltpaMetadataList *lutils.LTPAMetadataList
	expectedMetadataLength := 2
	instanceMutex.Lock()
	if r.isLTPAKeySharingEnabled(instance) {
		leaderMetadataList, err := r.reconcileResourceTrackingState(instance, LTPA_RESOURCE_SHARING_FILE_NAME, r.isCachingEnabled(instance))
		instanceMutex.Unlock()
		if err != nil {
			elapsed := time.Since(start)
			fmt.Printf("-- reconcileLTPAKeySharingEnabled failed with %s\n", elapsed)
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
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileLTPAKeySharingEnabled took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcilePasswordEncryptionKeySharingEnabled(reqDebugLogger logr.Logger, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult, passwordEncryptionMetadataChan chan<- *lutils.PasswordEncryptionMetadata) {
	start := time.Now()
	// Reconciles the shared password encryption key state for the instance namespace only if the shared key already exists
	var passwordEncryptionMetadataList *lutils.PasswordEncryptionMetadataList
	passwordEncryptionMetadata := &lutils.PasswordEncryptionMetadata{Name: ""}
	expectedMetadataLength := 1
	instanceMutex.Lock()
	if r.isUsingPasswordEncryptionKeySharing(instance, passwordEncryptionMetadata) {
		leaderMetadataList, err := r.reconcileResourceTrackingState(instance, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, r.isCachingEnabled(instance))
		instanceMutex.Unlock()
		if err != nil {
			elapsed := time.Since(start)
			fmt.Printf("-- reconcilePasswordEncryptionKeySharingEnabled failed with %s\n", elapsed)
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
		elapsed := time.Since(start)
		fmt.Printf("-- reconcilePasswordEncryptionKeySharingEnabled failed with %s\n", elapsed)
		reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled}
		for i := 0; i < expectedMetadataLength; i++ {
			passwordEncryptionMetadataChan <- passwordEncryptionMetadata
		}
		return
	} else {
		instanceMutex.Unlock()
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcilePasswordEncryptionKeySharingEnabled took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
	for i := 0; i < expectedMetadataLength; i++ {
		passwordEncryptionMetadataChan <- passwordEncryptionMetadata
	}
}

func (r *ReconcileOpenLiberty) reconcileServiceAccount(reqDebugLogger logr.Logger, defaultMeta metav1.ObjectMeta, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
	start := time.Now()
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
				elapsed := time.Since(start)
				fmt.Printf("-- reconcileServiceAccount failed with %s\n", elapsed)
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile ServiceAccount"}
				return
			}
		} else {
			serviceAccount := &corev1.ServiceAccount{ObjectMeta: defaultMeta}
			err := r.DeleteResource(serviceAccount)
			if err != nil {
				elapsed := time.Since(start)
				fmt.Printf("-- reconcileServiceAccount failed with %s\n", elapsed)
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
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileServiceAccount took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileSemeruCloudCompilerInit(reqDebugLogger logr.Logger, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult, semeruMarkedForDeletionChan chan<- bool) {
	start := time.Now()
	// Check if SemeruCloudCompiler is enabled before reconciling the Semeru Compiler deployment and service.
	// Otherwise, delete the Semeru Compiler deployment and service.
	message := "Start Semeru Compiler reconcile"
	instanceMutex.Lock()
	err, message, areCompletedSemeruInstancesMarkedToBeDeleted := r.reconcileSemeruCompiler(instance)
	instanceMutex.Unlock()
	semeruMarkedForDeletionChan <- areCompletedSemeruInstancesMarkedToBeDeleted
	if err != nil {
		elapsed := time.Since(start)
		fmt.Printf("-- reconcileSemeruCloudCompilerInit failed with %s\n", elapsed)
		reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: message}
		return
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileSemeruCloudCompilerInit took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileSemeruCloudCompilerReady(reqDebugLogger logr.Logger, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
	start := time.Now()
	// If semeru compiler is enabled, make sure its ready
	instanceMutex.Lock()
	if r.isSemeruEnabled(instance) {
		err := r.areSemeruCompilerResourcesReady(instance)
		message := "Check Semeru Compiler resources ready"
		instanceMutex.Unlock()
		if err != nil {
			elapsed := time.Since(start)
			fmt.Printf("-- reconcileSemeruCloudCompilerReady failed with %s\n", elapsed)
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeResourcesReady, message: message}
			return
		}
	} else {
		instanceMutex.Unlock()
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileSemeruCloudCompilerReady took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeResourcesReady}
}

func (r *ReconcileOpenLiberty) reconcileKnativeServiceSequential(reqDebugLogger logr.Logger, defaultMeta metav1.ObjectMeta, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reqLogger logr.Logger, isKnativeSupported bool) (ctrl.Result, error) {
	start := time.Now()
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
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileKnativeServiceSequential took %s\n", elapsed)
	return ctrl.Result{}, nil
}

func (r *ReconcileOpenLiberty) reconcileServiceCertificate(reqDebugLogger logr.Logger, ba common.BaseComponent, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, serviceCertificateReconcileResultChan chan<- ReconcileResult, useCertManagerChan chan<- bool) {
	start := time.Now()
	fmt.Printf("-- reconcileServiceCertificate (1) queued for lock at t=%s\n", start)
	instanceMutex.Lock()
	useCertmanager, err := r.GenerateSvcCertSecret(ba, OperatorShortName, "Open Liberty Operator", OperatorName)
	instanceMutex.Unlock()
	fmt.Printf("-- reconcileServiceCertificate (1) queued for unlock at t=%s\n", time.Now())
	useCertManagerChan <- useCertmanager
	if err != nil {
		elapsed := time.Since(start)
		fmt.Printf("-- reconcileServiceCertificate failed with %s\n", elapsed)
		serviceCertificateReconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile CertManager Certificate"}
		return
	}

	fmt.Printf("-- reconcileServiceCertificate (2) queued for lock at t=%s\n", time.Now())
	instanceMutex.Lock()
	if ba.GetService().GetCertificateSecretRef() != nil {
		ba.GetStatus().SetReference(common.StatusReferenceCertSecretName, *ba.GetService().GetCertificateSecretRef())
	}
	instanceMutex.Unlock()
	fmt.Printf("-- reconcileServiceCertificate (2) queued for unlock at t=%s\n", time.Now())
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileServiceCertificate took %s\n", elapsed)
	serviceCertificateReconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileService(reqDebugLogger logr.Logger, defaultMeta metav1.ObjectMeta, ba common.BaseComponent, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult, useCertManagerChan <-chan bool) {
	start := time.Now()
	fmt.Printf("-- reconcileService (1) queued for lock at t=%s\n", start)
	svc := &corev1.Service{ObjectMeta: defaultMeta}
	instanceMutex.Lock()
	err := r.CreateOrUpdate(svc, instance, func() error {
		oputils.CustomizeService(svc, ba)
		svc.Annotations = oputils.MergeMaps(svc.Annotations, instance.Spec.Service.Annotations)
		fmt.Printf("-- reconcileService (1) queued for unlock at t=%s\n", time.Now())
		instanceMutex.Unlock()
		useCertmanager := <-useCertManagerChan
		if !useCertmanager && r.IsOpenShift() {
			fmt.Printf("-- reconcileService (2) queued for lock at t=%s\n", time.Now())
			instanceMutex.Lock()
			oputils.AddOCPCertAnnotation(ba, svc)
			fmt.Printf("-- reconcileService (2) queued for unlock at t=%s\n", time.Now())
			instanceMutex.Unlock()
		}
		fmt.Printf("-- reconcileService (3) queued for lock at t=%s\n", time.Now())
		instanceMutex.Lock()
		monitoringEnabledLabelName := getMonitoringEnabledLabelName(ba)
		if instance.Spec.Monitoring != nil {
			fmt.Printf("-- reconcileService (3a) queued for unlock at t=%s\n", time.Now())
			instanceMutex.Unlock()
			svc.Labels[monitoringEnabledLabelName] = "true"
		} else {
			fmt.Printf("-- reconcileService (3b) queued for unlock at t=%s\n", time.Now())
			instanceMutex.Unlock()
			delete(svc.Labels, monitoringEnabledLabelName)
		}
		return nil
	})
	if err != nil {
		reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile Service"}
		return
	}

	fmt.Printf("-- reconcileService (4) queued for lock at t=%s\n", time.Now())
	instanceMutex.Lock()
	if (ba.GetManageTLS() == nil || *ba.GetManageTLS()) &&
		ba.GetStatus().GetReferences()[common.StatusReferenceCertSecretName] == "" {
		fmt.Printf("-- reconcileService (4a) queued for unlock at t=%s\n", time.Now())
		instanceMutex.Unlock()
		elapsed := time.Since(start)
		fmt.Printf("-- reconcileService failed with %s\n", elapsed)
		reconcileResultChan <- ReconcileResult{err: errors.New("Failed to generate TLS certificate. Ensure cert-manager is installed and running"), condition: common.StatusConditionTypeReconciled}
		return
	} else {
		fmt.Printf("-- reconcileService (4b) queued for unlock at t=%s\n", time.Now())
		instanceMutex.Unlock()
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileService took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileNetworkPolicy(reqDebugLogger logr.Logger, defaultMeta metav1.ObjectMeta, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
	start := time.Now()
	networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: defaultMeta}
	instanceMutex.Lock()
	if np := instance.Spec.NetworkPolicy; np == nil || np != nil && !np.IsDisabled() {
		err := r.CreateOrUpdate(networkPolicy, instance, func() error {
			oputils.CustomizeNetworkPolicy(networkPolicy, r.IsOpenShift(), instance)
			return nil
		})
		instanceMutex.Unlock()
		if err != nil {
			elapsed := time.Since(start)
			fmt.Printf("-- reconcileNetworkPolicy failed with %s\n", elapsed)
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile network policy"}
			return
		}
	} else {
		instanceMutex.Unlock()
		if err := r.DeleteResource(networkPolicy); err != nil {
			elapsed := time.Since(start)
			fmt.Printf("-- reconcileNetworkPolicy failed with %s\n", elapsed)
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete network policy"}
			return
		}
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileNetworkPolicy took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileServiceability(reqDebugLogger logr.Logger, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reqLogger logr.Logger, reconcileResultChan chan<- ReconcileResult) {
	start := time.Now()
	instanceMutex.Lock()
	if instance.Spec.Serviceability != nil {
		if instance.Spec.Serviceability.VolumeClaimName != "" {
			pvcName := instance.Spec.Serviceability.VolumeClaimName
			err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: pvcName, Namespace: instance.Namespace}, &corev1.PersistentVolumeClaim{})
			instanceMutex.Unlock()
			if err != nil && kerrors.IsNotFound(err) {
				elapsed := time.Since(start)
				fmt.Printf("-- reconcileServiceability failed with %s\n", elapsed)
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to find PersistentVolumeClaim " + pvcName + " in namespace " + instance.Namespace}
				return
			}
		} else {
			err := r.CreateOrUpdate(lutils.CreateServiceabilityPVC(instance), nil, func() error {
				return nil
			})
			instanceMutex.Unlock()
			if err != nil {
				elapsed := time.Since(start)
				fmt.Printf("-- reconcileServiceability failed with %s\n", elapsed)
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to create PersistentVolumeClaim for Serviceability"}
				return
			}
		}
	} else {
		r.deletePVC(reqLogger, instance.Name+"-serviceability", instance.Namespace)
		instanceMutex.Unlock()
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileServiceability took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileBindings(reqDebugLogger logr.Logger, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
	start := time.Now()
	instanceMutex.Lock()
	err := r.ReconcileBindings(instance)
	instanceMutex.Unlock()
	if err != nil {
		elapsed := time.Since(start)
		fmt.Printf("-- reconcileBindings failed with %s\n", elapsed)
		reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled}
		return
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileBindings took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcilePasswordEncryptionKeyConcurrent(reqDebugLogger logr.Logger, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata, sharedResourceReconcileResultChan chan<- ReconcileResult, lastRotationChan chan<- string, encryptionSecretNameChan chan<- string) {
	start := time.Now()
	// Manage the shared password encryption key Secret if it exists
	instanceMutex.Lock()
	message, encryptionSecretName, passwordEncryptionKeyLastRotation, err := r.reconcilePasswordEncryptionKey(instance, passwordEncryptionMetadata)
	instanceMutex.Unlock()
	lastRotationChan <- passwordEncryptionKeyLastRotation
	encryptionSecretNameChan <- encryptionSecretName
	if err != nil {
		elapsed := time.Since(start)
		fmt.Printf("-- reconcilePasswordEncryptionKeyConcurrent failed with %s\n", elapsed)
		sharedResourceReconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: message}
		return
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcilePasswordEncryptionKeyConcurrent took %s\n", elapsed)
	sharedResourceReconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled, message: message}
}

func (r *ReconcileOpenLiberty) reconcileLTPAKeysConcurrent(reqDebugLogger logr.Logger, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, ltpaKeysMetadata *lutils.LTPAMetadata, ltpaConfigMetadata *lutils.LTPAMetadata, reconcileResultChan chan<- ReconcileResult, lastRotationChan chan<- string, ltpaSecretNameChan chan<- string, ltpaKeysLastRotationChan chan<- string, reqLogger logr.Logger) {
	start := time.Now()
	// Create and manage the shared LTPA keys Secret if the feature is enabled
	instanceMutex.Lock()
	message, ltpaSecretName, ltpaKeysLastRotation, err := r.reconcileLTPAKeys(instance, ltpaKeysMetadata, reqLogger)
	instanceMutex.Unlock()
	ltpaSecretNameChan <- ltpaSecretName
	lastRotationChan <- ltpaKeysLastRotation
	ltpaKeysLastRotationChan <- ltpaKeysLastRotation
	if err != nil {
		elapsed := time.Since(start)
		fmt.Printf("-- reconcileLTPAKeysConcurrent failed with %s\n", elapsed)
		reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: message}
		return
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileLTPAKeysConcurrent took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled, message: message}
}

func (r *ReconcileOpenLiberty) reconcileLTPAConfigConcurrent(reqDebugLogger logr.Logger, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, ltpaKeysMetadata *lutils.LTPAMetadata, ltpaConfigMetadata *lutils.LTPAMetadata, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata, reconcileResultChan chan<- ReconcileResult, sharedResourceReconcileResultChan <-chan ReconcileResult, lastRotationChan <-chan string, ltpaKeysLastRotationChan <-chan string, ltpaXMLSecretNameChan chan<- string) {
	start := time.Now()
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
		elapsed := time.Since(start)
		fmt.Printf("-- reconcileLTPAConfigConcurrent failed with %s\n", elapsed)
		reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled}
		return
	}

	// Using the LTPA keys and config metadata, create and manage the shared LTPA Liberty server XML if the feature is enabled
	instanceMutex.Lock()
	message, ltpaXMLSecretName, err := r.reconcileLTPAConfig(instance, ltpaKeysMetadata, ltpaConfigMetadata, passwordEncryptionMetadata, ltpaKeysLastRotation, lastKeyRelatedRotation)
	instanceMutex.Unlock()
	ltpaXMLSecretNameChan <- ltpaXMLSecretName
	if err != nil {
		elapsed := time.Since(start)
		fmt.Printf("-- reconcileLTPAConfigConcurrent failed with %s\n", elapsed)
		reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled, message: message}
		return
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileLTPAConfigConcurrent took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileStatefulSetDeployment(reqDebugLogger logr.Logger, defaultMeta metav1.ObjectMeta, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, ltpaConfigMetadata *lutils.LTPAMetadata, passwordEncryptionMetadata *lutils.PasswordEncryptionMetadata, reconcileResultChan chan<- ReconcileResult, sharedResourceHandoffReconcileResultChan <-chan ReconcileResult, encryptionSecretNameChan <-chan string, ltpaSecretNameChan <-chan string, ltpaXMLSecretNameChan <-chan string) {
	start := time.Now()
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
			elapsed := time.Since(start)
			fmt.Printf("-- reconcileStatefulSetDeployment failed with %s\n", elapsed)
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
			elapsed := time.Since(start)
			fmt.Printf("-- reconcileStatefulSetDeployment failed with %s\n", elapsed)
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
			elapsed := time.Since(start)
			fmt.Printf("-- reconcileStatefulSetDeployment failed with %s\n", elapsed)
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
			elapsed := time.Since(start)
			fmt.Printf("-- reconcileStatefulSetDeployment failed with %s\n", elapsed)
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete StatefulSet"}
			return
		}

		// Delete StatefulSet if exists
		headlesssvc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: defaultMeta.Name + "-headless", Namespace: defaultMeta.Namespace}}
		err = r.DeleteResource(headlesssvc)

		if err != nil {
			elapsed := time.Since(start)
			fmt.Printf("-- reconcileStatefulSetDeployment failed with %s\n", elapsed)
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
				elapsed := time.Since(start)
				fmt.Printf("-- reconcileStatefulSetDeployment sub failed with %s\n", elapsed)
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
					elapsed := time.Since(start)
					fmt.Printf("-- reconcileStatefulSetDeployment sub failed with %s\n", elapsed)
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
			elapsed := time.Since(start)
			fmt.Printf("-- reconcileStatefulSetDeployment failed with %s\n", elapsed)
			if !capturedSubError {
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile Deployment"}
			}
			// otherwise return since the error is already captured in inside r.CreateOrUpdate()
			return
		}

	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileStatefulSetDeployment took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileAutoscaling(reqDebugLogger logr.Logger, defaultMeta metav1.ObjectMeta, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult) {
	start := time.Now()
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
			elapsed := time.Since(start)
			fmt.Printf("-- reconcileAutoscaling failed with %s\n", elapsed)
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile HorizontalPodAutoscaler"}
			return
		}
	} else {
		instanceMutex.Unlock()
		hpa := &autoscalingv1.HorizontalPodAutoscaler{ObjectMeta: defaultMeta}
		err := r.DeleteResource(hpa)
		if err != nil {
			elapsed := time.Since(start)
			fmt.Printf("-- reconcileAutoscaling failed with %s\n", elapsed)
			reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete HorizontalPodAutoscaler"}
			return
		}
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileAutoscaling took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileRouteIngress(reqDebugLogger logr.Logger, defaultMeta metav1.ObjectMeta, ba common.BaseComponent, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reqLogger logr.Logger, reconcileResultChan chan<- ReconcileResult) {
	start := time.Now()
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
					elapsed := time.Since(start)
					fmt.Printf("-- reconcileRouteIngress failed with %s\n", elapsed)
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
				elapsed := time.Since(start)
				fmt.Printf("-- reconcileRouteIngress failed with %s\n", elapsed)
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile Route"}
				return
			}

		} else {
			instanceMutex.Unlock()
			route := &routev1.Route{ObjectMeta: defaultMeta}
			err = r.DeleteResource(route)
			if err != nil {
				elapsed := time.Since(start)
				fmt.Printf("-- reconcileRouteIngress failed with %s\n", elapsed)
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
					elapsed := time.Since(start)
					fmt.Printf("-- reconcileRouteIngress failed with %s\n", elapsed)
					reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile Ingress"}
					return
				}
			} else {
				instanceMutex.Unlock()
				ing := &networkingv1.Ingress{ObjectMeta: defaultMeta}
				err = r.DeleteResource(ing)
				if err != nil {
					elapsed := time.Since(start)
					fmt.Printf("-- reconcileRouteIngress failed with %s\n", elapsed)
					reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete Ingress"}
					return
				}
			}
		}
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileRouteIngress took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileServiceMonitor(reqDebugLogger logr.Logger, defaultMeta metav1.ObjectMeta, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reqLogger logr.Logger, reconcileResultChan chan<- ReconcileResult) {
	start := time.Now()
	if ok, err := r.IsGroupVersionSupported(prometheusv1.SchemeGroupVersion.String(), "ServiceMonitor"); err != nil {
		reqLogger.Error(err, fmt.Sprintf("Failed to check if %s is supported", prometheusv1.SchemeGroupVersion.String()))
		// r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	} else if ok {
		instanceMutex.Lock()
		if instance.Spec.Monitoring != nil && (instance.Spec.CreateKnativeService == nil || !*instance.Spec.CreateKnativeService) {
			// Validate the monitoring endpoints' configuration before creating/updating the ServiceMonitor
			if err := oputils.ValidatePrometheusMonitoringEndpoints(instance, r.GetClient(), instance.GetNamespace()); err != nil {
				instanceMutex.Unlock()
				elapsed := time.Since(start)
				fmt.Printf("-- reconcileServiceMonitor failed with %s\n", elapsed)
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
				elapsed := time.Since(start)
				fmt.Printf("-- reconcileServiceMonitor failed with %s\n", elapsed)
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to reconcile ServiceMonitor"}
				return
			}
		} else {
			instanceMutex.Unlock()
			sm := &prometheusv1.ServiceMonitor{ObjectMeta: defaultMeta}
			err = r.DeleteResource(sm)
			if err != nil {
				elapsed := time.Since(start)
				fmt.Printf("-- reconcileServiceMonitor failed with %s\n", elapsed)
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete ServiceMonitor"}
				return
			}
		}
	} else {
		reqLogger.V(1).Info(fmt.Sprintf("%s is not supported", prometheusv1.SchemeGroupVersion.String()))
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileServiceMonitor took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) reconcileSemeruCloudCompilerCleanup(reqDebugLogger logr.Logger, instance *olv1.OpenLibertyApplication, instanceMutex *sync.Mutex, reconcileResultChan chan<- ReconcileResult, semeruMarkedForDeletionChan <-chan bool) {
	start := time.Now()
	// Delete completed Semeru instances because all pods now point to the newest Semeru service
	areCompletedSemeruInstancesMarkedToBeDeleted := <-semeruMarkedForDeletionChan

	if areCompletedSemeruInstancesMarkedToBeDeleted {
		instanceMutex.Lock()
		if r.isOpenLibertyApplicationReady(instance) {
			if err := r.deleteCompletedSemeruInstances(instance); err != nil {
				instanceMutex.Unlock()
				elapsed := time.Since(start)
				fmt.Printf("-- reconcileSemeruCloudCompilerCleanup failed with %s\n", elapsed)
				reconcileResultChan <- ReconcileResult{err: err, condition: common.StatusConditionTypeReconciled, message: "Failed to delete completed Semeru instance"}
				return
			} else {
				instanceMutex.Unlock()
			}
		} else {
			instanceMutex.Unlock()
		}
	}
	elapsed := time.Since(start)
	fmt.Printf("-- reconcileSemeruCloudCompilerCleanup took %s\n", elapsed)
	reconcileResultChan <- ReconcileResult{err: nil, condition: common.StatusConditionTypeReconciled}
}

func (r *ReconcileOpenLiberty) concurrentReconcile(ba common.BaseComponent, instance *olv1.OpenLibertyApplication, reqLogger logr.Logger, reqDebugLogger logr.Logger, isKnativeSupported bool, ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	defaultMeta := metav1.ObjectMeta{
		Name:      instance.Name,
		Namespace: instance.Namespace,
	}

	imageReferenceOld := instance.Status.ImageReference
	instance.Status.ImageReference = instance.Spec.ApplicationImage

	reconcileResultChan := make(chan ReconcileResult, 9)
	instanceMutex := &sync.Mutex{}

	go r.reconcileImageStream(reqDebugLogger, instance, instanceMutex, reconcileResultChan) // STATE: {reconcileResultChan: 1}

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

	// obtain ltpa keys and config metadata
	ltpaMetadataChan := make(chan *lutils.LTPAMetadata, 2)
	go r.reconcileLTPAKeySharingEnabled(reqDebugLogger, instance, instanceMutex, reconcileResultChan, ltpaMetadataChan) // STATE: {reconcileResultChan: 1, ltpaMetadataChan: 2}

	// obtain password encryption metadata
	passwordEncryptionMetadataChan := make(chan *lutils.PasswordEncryptionMetadata, 1)
	go r.reconcilePasswordEncryptionKeySharingEnabled(reqDebugLogger, instance, instanceMutex, reconcileResultChan, passwordEncryptionMetadataChan) // STATE: {reconcileResultChan: 2, ltpaMetadataChan: 2, passwordEncryptionMetadataChan: 1}
	go r.reconcileServiceAccount(reqDebugLogger, defaultMeta, instance, instanceMutex, reconcileResultChan)                                         // STATE: {reconcileResultChan: 3, ltpaMetadataChan: 2, passwordEncryptionMetadataChan: 1}

	semeruMarkedForDeletionChan := make(chan bool, 1)
	go r.reconcileSemeruCloudCompilerInit(reqDebugLogger, instance, instanceMutex, reconcileResultChan, semeruMarkedForDeletionChan) // STATE: {reconcileResultChan: 4, ltpaMetadataChan: 2, passwordEncryptionMetadataChan: 1, semeruMarkedForDeletionChan: 1}

	// FRONTIER: knative service should have option to exit the reconcile loop
	res, err := r.reconcileKnativeServiceSequential(reqDebugLogger, defaultMeta, instance, instanceMutex, reqLogger, isKnativeSupported)
	if err != nil {
		// block to pull from all go routines before exiting reconcile
		<-ltpaMetadataChan               // STATE: {reconcileResultChan: 4, passwordEncryptionMetadataChan: 1, semeruMarkedForDeletionChan: 1}
		<-passwordEncryptionMetadataChan // STATE: {reconcileResultChan: 4, semeruMarkedForDeletionChan: 1}
		<-semeruMarkedForDeletionChan    // STATE: {reconcileResultChan: 4}

		reconcileResults := 4
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
		2) // reconcilePasswordEncryptionKeyConcurrent() + reconcileLTPAKeysConcurrent() write to this chan
	sharedResourceHandoffReconcileResultChan := make(chan ReconcileResult,
		1) // reconcileLTPAConfigConcurrent() reads from sharedResourceReconcileResultChan and writes to this chan

	useCertManagerChan := make(chan bool, 1)
	go r.reconcileServiceCertificate(reqDebugLogger, ba, instance, instanceMutex, reconcileResultChan, useCertManagerChan)                                                                                                        // STATE: {reconcileResultChan: 5, semeruMarkedForDeletionChan: 1, useCertManagerChan: 1}
	go r.reconcilePasswordEncryptionKeyConcurrent(reqDebugLogger, instance, instanceMutex, passwordEncryptionMetadata, sharedResourceReconcileResultChan, lastRotationChan, encryptionSecretNameChan)                             // STATE: {reconcileResultChan: 5, semeruMarkedForDeletionChan: 1, useCertManagerChan: 1, sharedResourceReconcileResultChan: 1, lastRotationChan: 1, encryptionSecretNameChan: 1}
	go r.reconcileLTPAKeysConcurrent(reqDebugLogger, instance, instanceMutex, ltpaKeysMetadata, ltpaConfigMetadata, sharedResourceReconcileResultChan, lastRotationChan, ltpaSecretNameChan, ltpaKeysLastRotationChan, reqLogger) // STATE: {reconcileResultChan: 5, semeruMarkedForDeletionChan: 1, useCertManagerChan: 1, sharedResourceReconcileResultChan: 2, lastRotationChan: 2, ltpaKeysLastRotationChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1}
	go r.reconcileLTPAConfigConcurrent(reqDebugLogger, instance, instanceMutex, ltpaKeysMetadata, ltpaConfigMetadata, passwordEncryptionMetadata, sharedResourceHandoffReconcileResultChan, sharedResourceReconcileResultChan,
		lastRotationChan, ltpaKeysLastRotationChan, ltpaXMLSecretNameChan) // STATE: {reconcileResultChan: 5, semeruMarkedForDeletionChan: 1, useCertManagerChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}

	// FRONTIER: instances shouldn't proceed past if they are waiting for LTPA creation
	// for i := 0; i < reconcileResults; i++ {
	// 	reconcileResult := <-reconcileResultChan
	// 	// fmt.Printf("reconcile result %d\n", i)
	// 	if !foundFirstError && reconcileResult.err != nil {
	// 		foundFirstError = true
	// 		firstErroringReconcileResult = reconcileResult
	// 	}
	// }
	// // STATE: {useCertManagerChan: 1, semeruMarkedForDeletionChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	// if foundFirstError {
	// 	<-useCertManagerChan                       // STATE:  {semeruMarkedForDeletionChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	// 	<-semeruMarkedForDeletionChan              // STATE:  {sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	// 	<-sharedResourceHandoffReconcileResultChan // STATE:  {encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	// 	<-encryptionSecretNameChan                 // STATE:  {ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	// 	<-ltpaSecretNameChan                       // STATE:  {ltpaXMLSecretNameChan: 1}
	// 	<-ltpaXMLSecretNameChan                    // STATE: {}
	// 	return r.ManageError(firstErroringReconcileResult.err, firstErroringReconcileResult.condition, instance)
	// }

	go r.reconcileSemeruCloudCompilerReady(reqDebugLogger, instance, instanceMutex, reconcileResultChan)                     // STATE: {reconcileResultChan: 6, useCertManagerChan: 1, semeruMarkedForDeletionChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	go r.reconcileService(reqDebugLogger, defaultMeta, ba, instance, instanceMutex, reconcileResultChan, useCertManagerChan) // STATE: {reconcileResultChan: 7, useCertManagerChan: 1,semeruMarkedForDeletionChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	go r.reconcileNetworkPolicy(reqDebugLogger, defaultMeta, instance, instanceMutex, reconcileResultChan)                   // STATE: {reconcileResultChan: 8, semeruMarkedForDeletionChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	go r.reconcileServiceability(reqDebugLogger, instance, instanceMutex, reqLogger, reconcileResultChan)                    // STATE: {reconcileResultChan: 9, semeruMarkedForDeletionChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	go r.reconcileBindings(reqDebugLogger, instance, instanceMutex, reconcileResultChan)                                     // STATE: {reconcileResultChan: 10, semeruMarkedForDeletionChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}

	// FRONTIER: instances shouldn't proceed past if they are waiting for certificate creation
	reconcileResults := 10
	foundFirstError := false
	var firstErroringReconcileResult ReconcileResult
	for i := 0; i < reconcileResults; i++ {
		reconcileResult := <-reconcileResultChan
		if !foundFirstError && reconcileResult.err != nil {
			foundFirstError = true
			firstErroringReconcileResult = reconcileResult
		}
	}
	fmt.Println("--== cleared 10 reconcile results") // STATE: {semeruMarkedForDeletionChan: 1, sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
	if foundFirstError {
		fmt.Println("--== 2")
		<-semeruMarkedForDeletionChan // STATE:  {sharedResourceHandoffReconcileResultChan: 1, encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
		fmt.Println("--== 3")
		<-sharedResourceHandoffReconcileResultChan // STATE:  {encryptionSecretNameChan: 1, ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
		fmt.Println("--== 4")
		<-encryptionSecretNameChan // STATE:  {ltpaSecretNameChan: 1, ltpaXMLSecretNameChan: 1}
		fmt.Println("--== 5")
		<-ltpaSecretNameChan // STATE:  {ltpaXMLSecretNameChan: 1}
		fmt.Println("--== 6")
		<-ltpaXMLSecretNameChan // STATE: {}
		fmt.Println("--== end")
		return r.ManageError(firstErroringReconcileResult.err, firstErroringReconcileResult.condition, instance)
	}

	go r.reconcileStatefulSetDeployment(reqDebugLogger, defaultMeta, instance, instanceMutex, ltpaConfigMetadata, passwordEncryptionMetadata, reconcileResultChan, sharedResourceHandoffReconcileResultChan, encryptionSecretNameChan, ltpaSecretNameChan, ltpaXMLSecretNameChan) // STATE: {reconcileResultChan: 1, semeruMarkedForDeletionChan: 1}

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
	go r.reconcileAutoscaling(reqDebugLogger, defaultMeta, instance, instanceMutex, reconcileResultChan)                                // STATE: {semeruMarkedForDeletionChan: 1, reconcileResultChan: 1}
	go r.reconcileRouteIngress(reqDebugLogger, defaultMeta, ba, instance, instanceMutex, reqLogger, reconcileResultChan)                // STATE: {semeruMarkedForDeletionChan: 1, reconcileResultChan: 2}
	go r.reconcileServiceMonitor(reqDebugLogger, defaultMeta, instance, instanceMutex, reqLogger, reconcileResultChan)                  // STATE: {semeruMarkedForDeletionChan: 1, reconcileResultChan: 3}
	go r.reconcileSemeruCloudCompilerCleanup(reqDebugLogger, instance, instanceMutex, reconcileResultChan, semeruMarkedForDeletionChan) // STATE: {reconcileResultChan: 4}
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
