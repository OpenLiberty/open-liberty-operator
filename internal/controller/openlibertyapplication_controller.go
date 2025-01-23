package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/application-stacks/runtime-component-operator/common"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/go-logr/logr"

	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	oputils "github.com/application-stacks/runtime-component-operator/utils"

	openlibertyv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"

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
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	OperatorName      = "open-liberty-operator"
	OperatorShortName = "olo"
)

// ReconcileOpenLiberty reconciles an OpenLibertyApplication object
type ReconcileOpenLiberty struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	oputils.ReconcilerBase
	Log             logr.Logger
	watchNamespaces []string
}

const applicationFinalizer = "finalizer.openlibertyapplications.apps.openliberty.io"

var APIVersionNotFoundError = errors.New("APIVersion is not available")

// var workerCache *WorkerCache

// func init() {
// 	workerCache = &WorkerCache{}
// 	workerCache.Init()
// }

// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,resourceNames=restricted,verbs=use,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=apps.openliberty.io,resources=openlibertyapplications;openlibertyapplications/status;openlibertyapplications/finalizers,verbs=get;list;watch;create;update;patch;delete,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;delete,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=apps,resources=deployments/finalizers;statefulsets,verbs=update,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=core,resources=services;secrets;serviceaccounts;configmaps;persistentvolumeclaims,verbs=get;list;watch;create;update;delete,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;delete,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;delete,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;delete,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses;networkpolicies,verbs=get;list;watch;create;update;delete,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes;routes/custom-host,verbs=get;list;watch;create;update;delete,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=image.openshift.io,resources=imagestreams;imagestreamtags,verbs=get;list;watch,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;create;update;delete,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;delete,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates;issuers,verbs=get;list;watch;create;update;delete,namespace=open-liberty-operator

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ReconcileOpenLiberty) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqDebugLogger := reqLogger.V(common.LogLevelDebug)
	reqLogger.Info("Reconcile OpenLibertyApplication - starting")
	ns, err := oputils.GetOperatorNamespace()
	if err != nil {
		reqLogger.Info("Failed to get operator namespace, error: " + err.Error())
	}
	// When running the operator locally, `ns` will be empty string
	if ns == "" {
		// Since this method can be called directly from unit test, populate `watchNamespaces`.
		if r.watchNamespaces == nil {
			r.watchNamespaces, err = oputils.GetWatchNamespaces()
			if err != nil {
				reqLogger.Error(err, "Error getting watch namespace")
				return reconcile.Result{}, err
			}
		}
		// If the operator is running locally, use the first namespace in the `watchNamespaces`
		// `watchNamespaces` must have at least one item
		ns = r.watchNamespaces[0]
	}

	configMap, err := r.GetOpConfigMap(OperatorName, ns)
	if err != nil {
		if kerrors.IsNotFound(err) {
			reqLogger.Info("Failed to get open-liberty-operator config map, error: " + err.Error())
			oputils.CreateConfigMap(OperatorName)
		} else {
			// Error reading the object - requeue the request.
			reqLogger.Info("Failed to get open-liberty-operator config map, error: " + err.Error())
			return reconcile.Result{}, err
		}
	} else {
		common.LoadFromConfigMap(common.Config, configMap)
	}

	// if workerCache == nil {
	// 	maxWorkers, _ := strconv.ParseInt(common.LoadFromConfig(common.Config, common.OpConfigMaxWorkers), 10, 64)
	// 	maxCertManagerWorkers, _ := strconv.ParseInt(common.LoadFromConfig(common.Config, common.OpConfigMaxCertManagerWorkers), 10, 64)

	// 	workerCache = &WorkerCache{}
	// 	workerCache.Init(int(maxWorkers), int(maxCertManagerWorkers))
	// }

	// Fetch the OpenLiberty instance
	instance := &openlibertyv1.OpenLibertyApplication{}
	var ba common.BaseComponent = instance
	err = r.GetClient().Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if err = common.CheckValidValue(common.Config, common.OpConfigReconcileIntervalMinimum, OperatorName); err != nil {
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	if err = common.CheckValidValue(common.Config, common.OpConfigReconcileIntervalPercentage, OperatorName); err != nil {
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	isKnativeSupported, err := r.IsGroupVersionSupported(servingv1.SchemeGroupVersion.String(), "Service")
	if err != nil {
		r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	} else if !isKnativeSupported && instance.Spec.CreateKnativeService != nil && *instance.Spec.CreateKnativeService {
		reqLogger.V(1).Info(fmt.Sprintf("%s is not supported on the cluster", servingv1.SchemeGroupVersion.String()))
	}

	// Check if there is an existing Deployment, Statefulset or Knative service by this name
	// not managed by this operator
	err = oputils.CheckForNameConflicts("OpenLibertyApplication", instance.Name, instance.Namespace, r.GetClient(), request, isKnativeSupported)
	if err != nil {
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	// Check if the OpenLibertyApplication instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isInstanceMarkedToBeDeleted := instance.GetDeletionTimestamp() != nil
	if isInstanceMarkedToBeDeleted {
		if lutils.Contains(instance.GetFinalizers(), applicationFinalizer) {
			// Run finalization logic for applicationFinalizer. If the finalization logic fails, don't remove the
			// finalizer so that we can retry during the next reconciliation.
			if err := r.finalizeOpenLibertyApplication(reqLogger, instance, instance.Name+"-serviceability", instance.Namespace); err != nil {
				return reconcile.Result{}, err
			}

			// Remove applicationFinalizer. Once all finalizers have been removed, the object will be deleted.
			instance.SetFinalizers(lutils.Remove(instance.GetFinalizers(), applicationFinalizer))
			err := r.GetClient().Update(context.TODO(), instance)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// Add finalizer for this CR
	if !lutils.Contains(instance.GetFinalizers(), applicationFinalizer) {
		if err := r.addFinalizer(reqLogger, instance); err != nil {
			return reconcile.Result{}, err
		}
	}
	instance.Initialize()

	_, err = oputils.Validate(instance)
	// If there's any validation error, don't bother with requeuing
	if err != nil {
		reqLogger.Error(err, "Error validating OpenLibertyApplication")
		r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		return reconcile.Result{}, nil
	}

	_, err = lutils.Validate(instance)
	// If there's any validation error, don't bother with requeuing
	if err != nil {
		reqLogger.Error(err, "Error validating OpenLibertyApplication")
		r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		return reconcile.Result{}, nil
	}

	if r.IsOpenShift() {
		// The order of items passed to the MergeMaps matters here! Annotations from GetOpenShiftAnnotations have higher importance. Otherwise,
		// it is not possible to override converted annotations.
		instance.Annotations = oputils.MergeMaps(instance.Annotations, oputils.GetOpenShiftAnnotations(instance))
	}

	err = r.GetClient().Update(context.TODO(), instance)
	if err != nil {
		reqLogger.Error(err, "Error updating OpenLibertyApplication")
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	// if currentGen == 1 {
	// 	return reconcile.Result{}, nil
	// }

	// From here, the Open Liberty Application instance is stored in shared memory and can begin concurrent actions.
	if !r.isConcurrencyEnabled(instance) {
		return r.concurrentReconcile(ns, ba, instance, reqLogger, isKnativeSupported, ctx, request)
	} else {
		return r.sequentialReconcile(ns, ba, instance, reqLogger, reqDebugLogger, isKnativeSupported, ctx, request)
	}
}

func (r *ReconcileOpenLiberty) checkCertificateReady(cert *certmanagerv1.Certificate) error {
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: cert.Name, Namespace: cert.Namespace}, cert)
	if err != nil {
		return err
	}
	isReady := false
	for _, condition := range cert.Status.Conditions {
		if condition.Type == certmanagerv1.CertificateConditionReady {
			if condition.Status == certmanagermetav1.ConditionTrue {
				isReady = true
			}
		}
	}
	if !isReady {
		return fmt.Errorf("certificate %s is not ready", cert.Name)
	}
	return nil
}

func (r *ReconcileOpenLiberty) checkIssuerReady(issuer *certmanagerv1.Issuer) error {
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: issuer.Name, Namespace: issuer.Namespace}, issuer)
	if err != nil {
		return err
	}
	isReady := false
	for _, condition := range issuer.Status.Conditions {
		if condition.Type == certmanagerv1.IssuerConditionReady {
			if condition.Status == certmanagermetav1.ConditionTrue {
				isReady = true
			}
		}
	}
	if !isReady {
		return fmt.Errorf("issuer %s is not ready", issuer.Name)
	}
	return nil
}

func (r *ReconcileOpenLiberty) generateCMIssuer(namespace string, prefix string, CACommonName string, operatorName string) (string, error) {
	if ok, err := r.IsGroupVersionSupported(certmanagerv1.SchemeGroupVersion.String(), "Issuer"); err != nil {
		return "", err
	} else if !ok {
		return "", APIVersionNotFoundError
	}

	issuer := &certmanagerv1.Issuer{ObjectMeta: metav1.ObjectMeta{
		Name:      prefix + "-self-signed",
		Namespace: namespace,
	}}
	err := r.CreateOrUpdate(issuer, nil, func() error {
		issuer.Spec.SelfSigned = &certmanagerv1.SelfSignedIssuer{}
		issuer.Labels = oputils.MergeMaps(issuer.Labels, map[string]string{"app.kubernetes.io/managed-by": operatorName})
		return nil
	})
	if err != nil {
		return "", err
	}
	if err := r.checkIssuerReady(issuer); err != nil {
		return "", err
	}

	caCert := &certmanagerv1.Certificate{ObjectMeta: metav1.ObjectMeta{
		Name:      prefix + "-ca-cert",
		Namespace: namespace,
	}}

	caCertSecretName := prefix + "-ca-tls"
	err = r.CreateOrUpdate(caCert, nil, func() error {
		caCert.Labels = oputils.MergeMaps(caCert.Labels, map[string]string{"app.kubernetes.io/managed-by": operatorName})
		caCert.Spec.CommonName = CACommonName
		caCert.Spec.IsCA = true
		caCert.Spec.SecretName = caCertSecretName
		caCert.Spec.IssuerRef = certmanagermetav1.ObjectReference{
			Name: prefix + "-self-signed",
		}

		duration, err := time.ParseDuration(common.LoadFromConfig(common.Config, common.OpConfigCMCADuration))
		if err != nil {
			return err
		}

		caCert.Spec.Duration = &metav1.Duration{Duration: duration}
		return nil
	})
	if err != nil {
		return caCertSecretName, err
	}

	CustomCACert := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      prefix + "-custom-ca-tls",
		Namespace: namespace,
	}}
	customCACertFound := false
	err = r.GetClient().Get(context.Background(), types.NamespacedName{Name: CustomCACert.GetName(),
		Namespace: CustomCACert.GetNamespace()}, CustomCACert)
	if err == nil {
		customCACertFound = true
	} else {
		if err := r.checkCertificateReady(caCert); err != nil {
			return caCertSecretName, err
		}
	}

	issuer = &certmanagerv1.Issuer{ObjectMeta: metav1.ObjectMeta{
		Name:      prefix + "-ca-issuer",
		Namespace: namespace,
	}}
	err = r.CreateOrUpdate(issuer, nil, func() error {
		issuer.Labels = oputils.MergeMaps(issuer.Labels, map[string]string{"app.kubernetes.io/managed-by": operatorName})
		issuer.Spec.CA = &certmanagerv1.CAIssuer{}
		issuer.Spec.CA.SecretName = prefix + "-ca-tls"
		if issuer.Annotations == nil {
			issuer.Annotations = map[string]string{}
		}
		if customCACertFound {
			issuer.Spec.CA.SecretName = CustomCACert.Name

		}
		return nil
	})
	if err != nil {
		return caCertSecretName, err
	}

	for i := range issuer.Status.Conditions {
		if issuer.Status.Conditions[i].Type == certmanagerv1.IssuerConditionReady && issuer.Status.Conditions[i].Status == certmanagermetav1.ConditionFalse {
			return caCertSecretName, errors.New("Certificate Issuer is not ready")
		}
		if issuer.Status.Conditions[i].Type == certmanagerv1.IssuerConditionReady && issuer.Status.Conditions[i].ObservedGeneration != issuer.ObjectMeta.Generation {
			return caCertSecretName, errors.New("Certificate Issuer is not ready")
		}
	}
	return caCertSecretName, nil
}

func (r *ReconcileOpenLiberty) generateSvcCertIssuer(ba common.BaseComponent, instance *openlibertyv1.OpenLibertyApplication, prefix string, CACommonName string, operatorName string, addOwnerReference bool, requiresReservation bool) (bool, string, error) {
	delete(ba.GetStatus().GetReferences(), common.StatusReferenceCertSecretName)
	cleanup := func() {
		if ok, err := r.IsGroupVersionSupported(certmanagerv1.SchemeGroupVersion.String(), "Certificate"); err != nil {
			return
		} else if ok {
			obj := ba.(metav1.Object)
			svcCert := &certmanagerv1.Certificate{}
			svcCert.Name = obj.GetName() + "-svc-tls-cm"
			svcCert.Namespace = obj.GetNamespace()
			r.GetClient().Delete(context.Background(), svcCert)
		}
	}

	if ba.GetCreateKnativeService() != nil && *ba.GetCreateKnativeService() {
		cleanup()
		return false, "", nil
	}
	if ba.GetService() != nil && ba.GetService().GetCertificateSecretRef() != nil {
		cleanup()
		return false, "", nil
	}
	if ba.GetManageTLS() != nil && !*ba.GetManageTLS() {
		cleanup()
		return false, "", nil
	}
	if ba.GetService() != nil && ba.GetService().GetAnnotations() != nil {
		if _, ok := ba.GetService().GetAnnotations()["service.beta.openshift.io/serving-cert-secret-name"]; ok {
			cleanup()
			return false, "", nil
		}
		if _, ok := ba.GetService().GetAnnotations()["service.alpha.openshift.io/serving-cert-secret-name"]; ok {
			cleanup()
			return false, "", nil
		}
	}
	if ok, err := r.IsGroupVersionSupported(certmanagerv1.SchemeGroupVersion.String(), "Certificate"); err != nil {
		return false, "", err
	} else if ok {
		bao := ba.(metav1.Object)

		issuerSecretName, cmIssuerErr := r.generateCMIssuer(bao.GetNamespace(), prefix, CACommonName, operatorName)
		if cmIssuerErr != nil {
			if errors.Is(cmIssuerErr, APIVersionNotFoundError) {
				return false, issuerSecretName, nil
			}
			return true, issuerSecretName, cmIssuerErr
		}
	} else {
		return false, "", nil
	}
	return true, "", nil
}

func (r *ReconcileOpenLiberty) generateSvcCertSecret(ba common.BaseComponent, instance *openlibertyv1.OpenLibertyApplication, prefix string, CACommonName string, operatorName string, addOwnerReference bool, requiresReservation bool) (bool, error) {
	if ok, err := r.IsGroupVersionSupported(certmanagerv1.SchemeGroupVersion.String(), "Certificate"); err != nil {
		return false, err
	} else if ok {
		bao := ba.(metav1.Object)

		// if requiresReservation && !workerCache.ReserveWorkingInstance(WORKER, instance.GetNamespace(), instance.GetName()) {
		// 	if workerCache.PeekIssuerWork() == nil {
		// 		workerCache.CreateCertificateWork(bao.GetNamespace(), bao.GetName(), &Resource{
		// 			resourceName: "Certificate",
		// 			namespace:    bao.GetNamespace(),
		// 			name:         bao.GetName(),
		// 			instance:     instance,
		// 			priority:     5,
		// 		})
		// 	}
		// 	return true, fmt.Errorf("too many certificate workers, throttling...")
		// }

		svcCertSecretName := bao.GetName() + "-svc-tls-cm"

		svcCert := &certmanagerv1.Certificate{ObjectMeta: metav1.ObjectMeta{
			Name:      svcCertSecretName,
			Namespace: bao.GetNamespace(),
		}}

		customIssuer := &certmanagerv1.Issuer{ObjectMeta: metav1.ObjectMeta{
			Name:      prefix + "-custom-issuer",
			Namespace: bao.GetNamespace(),
		}}

		customIssuerFound := false
		err = r.GetClient().Get(context.Background(), types.NamespacedName{Name: customIssuer.Name,
			Namespace: customIssuer.Namespace}, customIssuer)
		if err == nil {
			customIssuerFound = true
		}

		shouldRefreshCertSecret := false
		var owner metav1.Object
		if addOwnerReference {
			owner = bao
		} else {
			owner = nil
		}
		err = r.CreateOrUpdate(svcCert, owner, func() error {
			svcCert.Labels = ba.GetLabels()
			svcCert.Annotations = oputils.MergeMaps(svcCert.Annotations, ba.GetAnnotations())
			if ba.GetService() != nil {
				if ba.GetService().GetCertificate() != nil {
					if ba.GetService().GetCertificate().GetAnnotations() != nil {
						svcCert.Annotations = oputils.MergeMaps(svcCert.Annotations, ba.GetService().GetCertificate().GetAnnotations())
					}
				}
			}

			svcCert.Spec.CommonName = trimCommonName(bao.GetName(), bao.GetNamespace())
			svcCert.Spec.DNSNames = make([]string, 4)
			svcCert.Spec.DNSNames[0] = bao.GetName() + "." + bao.GetNamespace() + ".svc"
			svcCert.Spec.DNSNames[1] = bao.GetName() + "." + bao.GetNamespace() + ".svc.cluster.local"
			svcCert.Spec.DNSNames[2] = bao.GetName() + "." + bao.GetNamespace()
			svcCert.Spec.DNSNames[3] = bao.GetName()
			if ba.GetStatefulSet() != nil {
				svcCert.Spec.DNSNames = append(svcCert.Spec.DNSNames, bao.GetName()+"-headless."+bao.GetNamespace()+".svc")
				svcCert.Spec.DNSNames = append(svcCert.Spec.DNSNames, bao.GetName()+"-headless."+bao.GetNamespace()+".svc.cluster.local")
				svcCert.Spec.DNSNames = append(svcCert.Spec.DNSNames, bao.GetName()+"-headless."+bao.GetNamespace())
				svcCert.Spec.DNSNames = append(svcCert.Spec.DNSNames, bao.GetName()+"-headless")
				// Wildcard entries for the pods
				svcCert.Spec.DNSNames = append(svcCert.Spec.DNSNames, "*."+bao.GetName()+"-headless."+bao.GetNamespace()+".svc")
				svcCert.Spec.DNSNames = append(svcCert.Spec.DNSNames, "*."+bao.GetName()+"-headless."+bao.GetNamespace()+".svc.cluster.local")
				svcCert.Spec.DNSNames = append(svcCert.Spec.DNSNames, "*."+bao.GetName()+"-headless."+bao.GetNamespace())
				svcCert.Spec.DNSNames = append(svcCert.Spec.DNSNames, "*."+bao.GetName()+"-headless")
			}
			svcCert.Spec.IsCA = false
			svcCert.Spec.IssuerRef = certmanagermetav1.ObjectReference{
				Name: prefix + "-ca-issuer",
			}
			if customIssuerFound {
				svcCert.Spec.IssuerRef.Name = customIssuer.Name
			}

			rVersion, _ := oputils.GetIssuerResourceVersion(r.GetClient(), svcCert)
			if svcCert.Spec.SecretTemplate == nil {
				svcCert.Spec.SecretTemplate = &certmanagerv1.CertificateSecretTemplate{
					Annotations: map[string]string{},
				}
			}

			if svcCert.Spec.SecretTemplate.Annotations[ba.GetGroupName()+"/cm-issuer-version"] != rVersion {
				if svcCert.Spec.SecretTemplate.Annotations == nil {
					svcCert.Spec.SecretTemplate.Annotations = map[string]string{}
				}
				svcCert.Spec.SecretTemplate.Annotations[ba.GetGroupName()+"/cm-issuer-version"] = rVersion
				shouldRefreshCertSecret = true
			}

			svcCert.Spec.SecretName = svcCertSecretName

			duration, err := time.ParseDuration(common.LoadFromConfig(common.Config, common.OpConfigCMCertDuration))
			if err != nil {
				return err
			}
			svcCert.Spec.Duration = &metav1.Duration{Duration: duration}

			return nil
		})
		if err != nil {
			return true, err
		}
		if shouldRefreshCertSecret {
			r.DeleteResource(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: svcCertSecretName, Namespace: svcCert.Namespace}})
		}
		ba.GetStatus().SetReference(common.StatusReferenceCertSecretName, svcCertSecretName)
	} else {
		return false, nil
	}
	return true, nil
}

// Create a common name for a certificate that is no longer
// that 64 bytes
func trimCommonName(compName string, ns string) (cn string) {

	commonName := compName + "." + ns + ".svc"
	if len(commonName) > 64 {
		// Try removing '.svc'
		commonName = compName + "." + ns
	}
	if len(commonName) > 64 {
		// Try removing the namespace
		commonName = compName
	}
	if len(commonName) > 64 {
		// Just have to truncate
		commonName = commonName[:64]
	}

	return commonName
}

func (r *ReconcileOpenLiberty) sequentialReconcile(operatorNamespace string, ba common.BaseComponent, instance *openlibertyv1.OpenLibertyApplication, reqLogger logr.Logger, reqDebugLogger logr.Logger, isKnativeSupported bool, ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	defaultMeta := metav1.ObjectMeta{
		Name:      instance.Name,
		Namespace: instance.Namespace,
	}

	imageReferenceOld := instance.Status.ImageReference
	instance.Status.ImageReference = instance.Spec.ApplicationImage
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
				return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
			}
		}
	}

	// Reconciles the shared LTPA state for the instance namespace
	var ltpaMetadataList *lutils.LTPAMetadataList
	var ltpaKeysMetadata, ltpaConfigMetadata *lutils.LTPAMetadata
	if r.isLTPAKeySharingEnabled(instance) {
		leaderMetadataList, err := r.reconcileResourceTrackingState(instance, LTPA_RESOURCE_SHARING_FILE_NAME, r.isCachingEnabled(instance))
		if err != nil {
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
		ltpaMetadataList = leaderMetadataList.(*lutils.LTPAMetadataList)
		if ltpaMetadataList != nil && len(ltpaMetadataList.Items) == 2 {
			ltpaKeysMetadata = ltpaMetadataList.Items[0].(*lutils.LTPAMetadata)
			ltpaConfigMetadata = ltpaMetadataList.Items[1].(*lutils.LTPAMetadata)
		}
	}
	// Reconciles the shared password encryption key state for the instance namespace only if the shared key already exists
	var passwordEncryptionMetadataList *lutils.PasswordEncryptionMetadataList
	passwordEncryptionMetadata := &lutils.PasswordEncryptionMetadata{}
	if r.isUsingPasswordEncryptionKeySharing(instance, passwordEncryptionMetadata) {
		leaderMetadataList, err := r.reconcileResourceTrackingState(instance, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME, r.isCachingEnabled(instance))
		if err != nil {
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
		passwordEncryptionMetadataList = leaderMetadataList.(*lutils.PasswordEncryptionMetadataList)
		if passwordEncryptionMetadataList != nil && len(passwordEncryptionMetadataList.Items) == 1 {
			passwordEncryptionMetadata = passwordEncryptionMetadataList.Items[0].(*lutils.PasswordEncryptionMetadata)
		}
	} else if r.isPasswordEncryptionKeySharingEnabled(instance) {
		// error if the password encryption key sharing is enabled but the Secret is not found
		passwordEncryptionSecretName := lutils.PasswordEncryptionKeyRootName + passwordEncryptionMetadata.Name
		err := errors.Wrapf(fmt.Errorf("secret %q not found", passwordEncryptionSecretName), "Secret for Password Encryption was not found. Create a secret named %q in namespace %q with the encryption key specified in data field %q.", passwordEncryptionSecretName, instance.GetNamespace(), "passwordEncryptionKey")
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	if imageReferenceOld != instance.Status.ImageReference {
		// Trigger a new Semeru Cloud Compiler generation
		createNewSemeruGeneration(instance)

		reqLogger.Info("Updating status.imageReference", "status.imageReference", instance.Status.ImageReference)
		err := r.UpdateStatus(instance)
		if err != nil {
			reqLogger.Error(err, "Error updating Open Liberty application status")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	}

	if operatorNamespace == "" {
		operatorNamespace = instance.GetNamespace()
	}
	message, err := r.reconcileLibertyProxy(operatorNamespace)
	if err != nil {
		reqLogger.Error(err, "Failed to reconcile Liberty proxy: "+message)
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	serviceAccountName := oputils.GetServiceAccountName(instance)
	if serviceAccountName != defaultMeta.Name {
		if serviceAccountName == "" {
			serviceAccount := &corev1.ServiceAccount{ObjectMeta: defaultMeta}
			err := r.CreateOrUpdate(serviceAccount, instance, func() error {
				return oputils.CustomizeServiceAccount(serviceAccount, instance, r.GetClient())
			})
			if err != nil {
				reqLogger.Error(err, "Failed to reconcile ServiceAccount")
				return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
			}
		} else {
			serviceAccount := &corev1.ServiceAccount{ObjectMeta: defaultMeta}
			err := r.DeleteResource(serviceAccount)
			if err != nil {
				reqLogger.Error(err, "Failed to delete ServiceAccount")
				return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
			}
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
	message = "Start Semeru Compiler reconcile"
	reqDebugLogger.Info(message)
	err, message, areCompletedSemeruInstancesMarkedToBeDeleted := r.reconcileSemeruCompiler(instance)
	if err != nil {
		reqLogger.Error(err, message)
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}
	// If semeru compiler is enabled, make sure its ready
	if r.isSemeruEnabled(instance) {
		message = "Check Semeru Compiler resources ready"
		reqDebugLogger.Info(message)
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
			reqDebugLogger.Info("Knative is supported and Knative Service is enabled")
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
	useCertmanager, issuerSecretName, err := r.generateSvcCertIssuer(ba, instance, OperatorShortName, "Open Liberty Operator", OperatorName, r.isCertOwnerEnabled(instance), true)
	if err != nil {
		reqLogger.Error(err, "Failed to reconcile CertManager Issuer")
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}
	if secretName := issuerSecretName; secretName != "" {
		secret := &corev1.Secret{}
		secret.Name = secretName
		secret.Namespace = instance.GetNamespace()
		err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: instance.GetNamespace()}, secret)
		if err != nil {
			return r.ManageError(fmt.Errorf("Secret %q was not found in namespace %q, %w", secretName, instance.GetNamespace(), err), common.StatusConditionTypeReconciled, instance)
		}
	}

	if useCertmanager {
		_, err := r.generateSvcCertSecret(ba, instance, OperatorShortName, "Open Liberty Operator", OperatorName, r.isCertOwnerEnabled(instance), true)
		if err != nil {
			reqLogger.Error(err, "Failed to reconcile CertManager Certificate")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	}

	if ba.GetService().GetCertificateSecretRef() != nil {
		ba.GetStatus().SetReference(common.StatusReferenceCertSecretName, *ba.GetService().GetCertificateSecretRef())
	}
	if secretName := ba.GetStatus().GetReferences()[common.StatusReferenceCertSecretName]; secretName != "" {
		secret := &corev1.Secret{}
		secret.Name = secretName
		secret.Namespace = instance.GetNamespace()
		err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: instance.GetNamespace()}, secret)
		if err != nil {
			return r.ManageError(fmt.Errorf("Secret %q was not found in namespace %q, %w", secretName, instance.GetNamespace(), err), common.StatusConditionTypeReconciled, instance)
		}
		// else {
		// 	workerCache.ReleaseWorkingInstance(WORKER, instance.GetNamespace(), instance.GetName())
		// 	if useCertmanager && workerCache.PeekIssuerWork() == nil {
		// 		// perform certificate work
		// 		item := workerCache.GetCertificateWork()
		// 		if item != nil {
		// 			r.generateSvcCertSecret(ba, item.instance, OperatorShortName, "Open Liberty Operator", OperatorName, r.isCertOwnerEnabled(instance), false) // pre-load for the next instance without reservation
		// 		}
		// 	}
		// }
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

	// if (ba.GetManageTLS() == nil || *ba.GetManageTLS()) &&
	// 	ba.GetStatus().GetReferences()[common.StatusReferenceCertSecretName] == "" {
	// 	return r.ManageError(errors.New("Failed to generate TLS certificate. Ensure cert-manager is installed and running"),
	// 		common.StatusConditionTypeReconciled, instance)
	// }

	networkPolicy := &networkingv1.NetworkPolicy{ObjectMeta: defaultMeta}
	if np := instance.Spec.NetworkPolicy; np == nil || np != nil && !np.IsDisabled() {
		err = r.CreateOrUpdate(networkPolicy, instance, func() error {
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
			// 			},
			// 		},
			// 	})
			// }
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
	message, ltpaSecretName, ltpaKeysLastRotation, err := r.reconcileLTPAKeys(operatorNamespace, instance, ltpaKeysMetadata)
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
	message, ltpaXMLSecretName, err := r.reconcileLTPAConfig(operatorNamespace, instance, ltpaKeysMetadata, ltpaConfigMetadata, passwordEncryptionMetadata, ltpaKeysLastRotation, lastKeyRelatedRotation)
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

	if (ba.GetManageTLS() == nil || *ba.GetManageTLS()) &&
		ba.GetStatus().GetReferences()[common.StatusReferenceCertSecretName] == "" {
		return r.ManageError(errors.New("Failed to generate TLS certificate. Ensure cert-manager is installed and running"),
			common.StatusConditionTypeReconciled, instance)
	}

	instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
	instance.Status.Versions.Reconciled = lutils.OperandVersion
	reqLogger.Info("Reconcile OpenLibertyApplication - completed")
	return r.ManageSuccess(common.StatusConditionTypeReconciled, instance)
}

func (r *ReconcileOpenLiberty) isOpenLibertyApplicationReady(ba common.BaseComponent) bool {
	_, condition := r.CheckApplicationStatus(ba)
	if condition.GetStatus() == corev1.ConditionTrue {
		statusCondition := ba.GetStatus().GetCondition(common.StatusConditionTypeReady)
		return statusCondition != nil && statusCondition.GetMessage() == common.StatusConditionTypeReadyMessage
	}
	return false
}

func (r *ReconcileOpenLiberty) SetupWithManager(mgr ctrl.Manager) error {

	mgr.GetFieldIndexer().IndexField(context.Background(), &openlibertyv1.OpenLibertyApplication{}, indexFieldImageStreamName, func(obj client.Object) []string {
		instance := obj.(*openlibertyv1.OpenLibertyApplication)
		image, err := imageutil.ParseDockerImageReference(instance.Spec.ApplicationImage)
		if err == nil {
			imageNamespace := image.Namespace
			if imageNamespace == "" {
				imageNamespace = instance.Namespace
			}
			fullName := fmt.Sprintf("%s/%s", imageNamespace, image.Name)
			return []string{fullName}
		}
		return nil
	})

	watchNamespaces, err := oputils.GetWatchNamespaces()
	if err != nil {
		r.Log.Error(err, "Failed to get watch namespace")
		os.Exit(1)
	}

	watchNamespacesMap := make(map[string]bool)
	for _, ns := range watchNamespaces {
		watchNamespacesMap[ns] = true
	}
	isClusterWide := oputils.IsClusterWide(watchNamespaces)

	pred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ignore updates to CR status in which case metadata.Generation does not change
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() && (isClusterWide || watchNamespacesMap[e.ObjectNew.GetNamespace()])
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return isClusterWide || watchNamespacesMap[e.Object.GetNamespace()]
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return isClusterWide || watchNamespacesMap[e.Object.GetNamespace()]
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return isClusterWide || watchNamespacesMap[e.Object.GetNamespace()]
		},
	}

	predSubResource := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return (isClusterWide || watchNamespacesMap[e.ObjectOld.GetNamespace()])
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return isClusterWide || watchNamespacesMap[e.Object.GetNamespace()]
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	predSubResWithGenCheck := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ignore updates to CR status in which case metadata.Generation does not change
			return (isClusterWide || watchNamespacesMap[e.ObjectOld.GetNamespace()]) && e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return isClusterWide || watchNamespacesMap[e.Object.GetNamespace()]
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	b := ctrl.NewControllerManagedBy(mgr).For(&openlibertyv1.OpenLibertyApplication{}, builder.WithPredicates(pred))

	if !oputils.GetOperatorDisableWatches() {
		b = b.Owns(&corev1.Service{}, builder.WithPredicates(predSubResource)).
			Owns(&corev1.Secret{}, builder.WithPredicates(predSubResource)).
			Owns(&appsv1.Deployment{}, builder.WithPredicates(predSubResWithGenCheck)).
			Owns(&appsv1.StatefulSet{}, builder.WithPredicates(predSubResWithGenCheck))

		if oputils.GetOperatorWatchHPA() {
			b = b.Owns(&autoscalingv1.HorizontalPodAutoscaler{}, builder.WithPredicates(predSubResource))
		}

		ok, _ := r.IsGroupVersionSupported(routev1.SchemeGroupVersion.String(), "Route")
		if ok {
			b = b.Owns(&routev1.Route{}, builder.WithPredicates(predSubResource))
		}
		ok, _ = r.IsGroupVersionSupported(networkingv1.SchemeGroupVersion.String(), "Ingress")
		if ok {
			b = b.Owns(&networkingv1.Ingress{}, builder.WithPredicates(predSubResource))
		}
		ok, _ = r.IsGroupVersionSupported(servingv1.SchemeGroupVersion.String(), "Service")
		if ok {
			b = b.Owns(&servingv1.Service{}, builder.WithPredicates(predSubResource))
		}
		ok, _ = r.IsGroupVersionSupported(prometheusv1.SchemeGroupVersion.String(), "ServiceMonitor")
		if ok {
			b = b.Owns(&prometheusv1.ServiceMonitor{}, builder.WithPredicates(predSubResource))
		}
		ok, _ = r.IsGroupVersionSupported(imagev1.SchemeGroupVersion.String(), "ImageStream")
		if ok {
			b = b.Watches(&imagev1.ImageStream{}, &EnqueueRequestsForCustomIndexField{
				Matcher: &ImageStreamMatcher{
					Klient:          mgr.GetClient(),
					WatchNamespaces: watchNamespaces,
				},
			})
		}
	}

	maxConcurrentReconciles := oputils.GetMaxConcurrentReconciles()

	return b.WithOptions(controller.Options{
		MaxConcurrentReconciles: maxConcurrentReconciles,
	}).Complete(r)
}

func getMonitoringEnabledLabelName(ba common.BaseComponent) string {
	return "monitor." + ba.GetGroupName() + "/enabled"
}

func (r *ReconcileOpenLiberty) finalizeOpenLibertyApplication(reqLogger logr.Logger, olapp *openlibertyv1.OpenLibertyApplication, pvcName string, pvcNamespace string) error {
	r.RemoveLeaderTrackerReference(olapp, LTPA_RESOURCE_SHARING_FILE_NAME)
	r.RemoveLeaderTrackerReference(olapp, PASSWORD_ENCRYPTION_RESOURCE_SHARING_FILE_NAME)
	r.deletePVC(reqLogger, pvcName, pvcNamespace)
	return nil
}

func (r *ReconcileOpenLiberty) addFinalizer(reqLogger logr.Logger, olapp *openlibertyv1.OpenLibertyApplication) error {
	reqLogger.Info("Adding Finalizer for OpenLibertyApplication")
	olapp.SetFinalizers(append(olapp.GetFinalizers(), applicationFinalizer))

	// Update CR
	err := r.GetClient().Update(context.TODO(), olapp)
	if err != nil {
		reqLogger.Error(err, "Failed to update OpenLibertyApplication with finalizer")
		return err
	}

	return nil
}

func (r *ReconcileOpenLiberty) deletePVC(reqLogger logr.Logger, pvcName string, pvcNamespace string) {
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: pvcName, Namespace: pvcNamespace}, pvc)
	if err == nil {
		if pvc.Status.Phase != "Bound" {
			reqLogger.Info("Deleting dangling PVC that is not in Bound state")
			err = r.DeleteResource(pvc)
			if err != nil {
				reqLogger.Error(err, "Failed to delete dangling PersistentVolumeClaim for Serviceability")
			}
		}
	}
}
