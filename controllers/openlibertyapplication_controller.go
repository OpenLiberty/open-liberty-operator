package controllers

import (
	"context"
	"fmt"
	"os"

	networkingv1 "k8s.io/api/networking/v1"

	"github.com/application-stacks/runtime-component-operator/common"
	"github.com/go-logr/logr"

	olutils "github.com/OpenLiberty/open-liberty-operator/utils"
	appstacksutils "github.com/application-stacks/runtime-component-operator/utils"

	openlibertyv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"

	imagev1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	imageutil "github.com/openshift/library-go/pkg/image/imageutil"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	OperatorFullName  = "Open Liberty Operator"
	OperatorName      = "open-liberty-operator"
	OperatorShortName = "olo"
	APIName           = "OpenLibertyApplication"
)

// ReconcileOpenLiberty reconciles an OpenLibertyApplication object
type ReconcileOpenLiberty struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	appstacksutils.ReconcilerBase
	Log             logr.Logger
	watchNamespaces []string
}

const applicationFinalizer = "finalizer.openlibertyapplications.apps.openliberty.io"

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
	reqLogger.Info("Reconcile " + APIName + " - starting")

	if ns, err := r.CheckOperatorNamespace(r.watchNamespaces); err != nil {
		return reconcile.Result{}, err
	} else {
		r.UpdateConfigMap(OperatorName, ns)
	}

	// Fetch the OpenLiberty instance
	instance := &openlibertyv1.OpenLibertyApplication{}
	if err := r.GetClient().Get(context.TODO(), request.NamespacedName, instance); err != nil {
		if kerrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	isKnativeSupported, err := r.IsGroupVersionSupported(servingv1.SchemeGroupVersion.String(), "Service")
	if err != nil {
		r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	} else if !isKnativeSupported && instance.Spec.CreateKnativeService != nil && *instance.Spec.CreateKnativeService {
		reqLogger.V(1).Info(fmt.Sprintf("%s is not supported on the cluster", servingv1.SchemeGroupVersion.String()))
	}

	// Check if there is an existing Deployment, Statefulset or Knative service by this name
	// not managed by this operator
	if err = appstacksutils.CheckForNameConflicts(APIName, instance.Name, instance.Namespace, r.GetClient(), request, isKnativeSupported); err != nil {
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	// Check if the OpenLibertyApplication instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isInstanceMarkedToBeDeleted := instance.GetDeletionTimestamp() != nil
	if isInstanceMarkedToBeDeleted {
		if olutils.Contains(instance.GetFinalizers(), applicationFinalizer) {
			// Run finalization logic for applicationFinalizer. If the finalization logic fails, don't remove the
			// finalizer so that we can retry during the next reconciliation.
			if err := r.finalizeOpenLibertyApplication(reqLogger, instance, instance.Name+"-serviceability", instance.Namespace); err != nil {
				return reconcile.Result{}, err
			}

			// Remove applicationFinalizer. Once all finalizers have been removed, the object will be deleted.
			instance.SetFinalizers(olutils.Remove(instance.GetFinalizers(), applicationFinalizer))
			err := r.GetClient().Update(context.TODO(), instance)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// Add finalizer for this CR
	if !olutils.Contains(instance.GetFinalizers(), applicationFinalizer) {
		if err := r.addFinalizer(reqLogger, instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	// initialize the OpenLibertyApplication instance
	instance.Initialize()

	// If there's any validation error, don't bother with requeuing
	if _, err = appstacksutils.Validate(instance); err != nil {
		reqLogger.Error(err, "Error validating "+APIName)
		r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		return reconcile.Result{}, nil
	}

	// If there's any validation error, don't bother with requeuing
	if _, err = olutils.Validate(instance); err != nil {
		reqLogger.Error(err, "Error validating "+APIName)
		r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		return reconcile.Result{}, nil
	}

	if r.IsOpenShift() {
		// The order of items passed to the MergeMaps matters here! Annotations from GetOpenShiftAnnotations have higher importance. Otherwise,
		// it is not possible to override converted annotations.
		instance.Annotations = appstacksutils.MergeMaps(instance.Annotations, appstacksutils.GetOpenShiftAnnotations(instance))
	}

	if err = r.GetClient().Update(context.TODO(), instance); err != nil {
		reqLogger.Error(err, "Error updating "+APIName)
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	defaultMeta := metav1.ObjectMeta{
		Name:      instance.Name,
		Namespace: instance.Namespace,
	}

	imageReferenceOld := instance.Status.ImageReference
	if err = r.UpdateImageReference(instance); err != nil {
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	if imageReferenceOld != instance.Status.ImageReference {
		// Trigger a new Semeru Cloud Compiler generation
		createNewSemeruGeneration(instance)

		// If the shared LTPA keys was not generated from the last application image, restart the key generation process
		if r.isLTPAKeySharingEnabled(instance) {
			if err := r.restartLTPAKeysGeneration(instance); err != nil {
				reqLogger.Error(err, "Error restarting the LTPA keys generation process")
				return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
			}
		}

		reqLogger.Info("Updating status.imageReference", "status.imageReference", instance.Status.ImageReference)
		if err = r.UpdateStatus(instance); err != nil {
			reqLogger.Error(err, "Error updating "+APIName+" status")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	}

	if err = r.UpdateServiceAccount(instance, defaultMeta); err != nil {
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
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
		if err = r.areSemeruCompilerResourcesReady(instance); err != nil {
			reqLogger.Error(err, message)
			return r.ManageError(err, common.StatusConditionTypeResourcesReady, instance)
		}
	}

	// If Knative is supported and being used, delete other resources and create/update Knative service
	// Otherwise, delete Knative service
	createKnativeService := instance.GetCreateKnativeService() != nil && *instance.GetCreateKnativeService()
	err = r.UpdateKnativeService(instance, defaultMeta, isKnativeSupported, createKnativeService)
	if createKnativeService {
		if err != nil {
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		} else {
			instance.Status.Versions.Reconciled = olutils.OperandVersion
			reqLogger.Info("Reconcile " + APIName + " - completed")
			return r.ManageSuccess(common.StatusConditionTypeReconciled, instance)
		}
	}

	useCertmanager, err := r.UpdateSvcCertSecret(instance, OperatorShortName, OperatorFullName, OperatorName)
	if err != nil {
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	if err = r.UpdateService(instance, defaultMeta, useCertmanager); err != nil {
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	if err = r.UpdateTLSReference(instance); err != nil {
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	if err = r.UpdateNetworkPolicy(instance, defaultMeta); err != nil {
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
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
			err = r.CreateOrUpdate(olutils.CreateServiceabilityPVC(instance), nil, func() error {
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

	err, message, ltpaSecretName := r.reconcileLTPAKeysSharing(instance)
	if err != nil {
		reqLogger.Error(err, message)
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	if instance.Spec.StatefulSet != nil {
		// Update StatefulSet - common configuration
		if err = r.UpdateStatefulSet(instance, defaultMeta); err != nil {
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}

		// Update StatefulSet - Liberty configuration
		statefulSet := &appsv1.StatefulSet{ObjectMeta: defaultMeta}
		err = r.CreateOrUpdate(statefulSet, instance, func() error {
			if err := olutils.CustomizeLibertyEnv(&statefulSet.Spec.Template, instance, r.GetClient()); err != nil {
				reqLogger.Error(err, "Failed to reconcile Liberty env, error: "+err.Error())
				return err
			}

			statefulSet.Spec.Template.Spec.Containers[0].Args = r.getSemeruJavaOptions(instance)

			olutils.CustomizeLibertyAnnotations(&statefulSet.Spec.Template, instance)
			if instance.Spec.SSO != nil {
				err = olutils.CustomizeEnvSSO(&statefulSet.Spec.Template, instance, r.GetClient(), r.IsOpenShift())
				if err != nil {
					reqLogger.Error(err, "Failed to reconcile Single sign-on configuration")
					return err
				}
			}
			olutils.ConfigureServiceability(&statefulSet.Spec.Template, instance)
			semeruCertVolume := getSemeruCertVolume(instance)
			if r.isSemeruEnabled(instance) && semeruCertVolume != nil {
				statefulSet.Spec.Template.Spec.Volumes = append(statefulSet.Spec.Template.Spec.Volumes, *semeruCertVolume)
				statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts = append(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts,
					getSemeruCertVolumeMount(instance))
				semeruTLSSecretName := instance.Status.SemeruCompiler.TLSSecretName
				err := olutils.AddSecretResourceVersionAsEnvVar(&statefulSet.Spec.Template, instance, r.GetClient(),
					semeruTLSSecretName, "SEMERU_TLS")
				if err != nil {
					return err
				}
			}

			if r.isLTPAKeySharingEnabled(instance) && len(ltpaSecretName) > 0 {
				olutils.ConfigureLTPA(&statefulSet.Spec.Template, instance, OperatorShortName)
				err := olutils.AddSecretResourceVersionAsEnvVar(&statefulSet.Spec.Template, instance, r.GetClient(), ltpaSecretName, "LTPA")
				if err != nil {
					return err
				}
			}
			return nil
		})
	} else {
		if err = r.UpdateDeployment(instance, defaultMeta); err != nil {
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}

		deploy := &appsv1.Deployment{ObjectMeta: defaultMeta}
		err = r.CreateOrUpdate(deploy, instance, func() error {
			if err := olutils.CustomizeLibertyEnv(&deploy.Spec.Template, instance, r.GetClient()); err != nil {
				reqLogger.Error(err, "Failed to reconcile Liberty env, error: "+err.Error())
				return err
			}
			deploy.Spec.Template.Spec.Containers[0].Args = r.getSemeruJavaOptions(instance)

			olutils.CustomizeLibertyAnnotations(&deploy.Spec.Template, instance)
			if instance.Spec.SSO != nil {
				err = olutils.CustomizeEnvSSO(&deploy.Spec.Template, instance, r.GetClient(), r.IsOpenShift())
				if err != nil {
					reqLogger.Error(err, "Failed to reconcile Single sign-on configuration")
					return err
				}
			}

			olutils.ConfigureServiceability(&deploy.Spec.Template, instance)
			semeruCertVolume := getSemeruCertVolume(instance)
			if r.isSemeruEnabled(instance) && semeruCertVolume != nil {
				deploy.Spec.Template.Spec.Volumes = append(deploy.Spec.Template.Spec.Volumes, *semeruCertVolume)
				deploy.Spec.Template.Spec.Containers[0].VolumeMounts = append(deploy.Spec.Template.Spec.Containers[0].VolumeMounts,
					getSemeruCertVolumeMount(instance))
				semeruTLSSecretName := instance.Status.SemeruCompiler.TLSSecretName
				err := olutils.AddSecretResourceVersionAsEnvVar(&deploy.Spec.Template, instance, r.GetClient(),
					semeruTLSSecretName, "SEMERU_TLS")
				if err != nil {
					return err
				}
			}

			if r.isLTPAKeySharingEnabled(instance) && len(ltpaSecretName) > 0 {
				olutils.ConfigureLTPA(&deploy.Spec.Template, instance, OperatorShortName)
				err := olutils.AddSecretResourceVersionAsEnvVar(&deploy.Spec.Template, instance, r.GetClient(), ltpaSecretName, "LTPA")
				if err != nil {
					return err
				}
			}
			return nil
		})
	}

	if err = r.UpdateAutoscaling(instance, defaultMeta); err != nil {
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	if err = r.UpdateRouteOrIngress(instance, defaultMeta); err != nil {
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	if err = r.UpdateServiceMonitor(instance, defaultMeta); err != nil {
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	// Delete completed Semeru instances because all pods now point to the newest Semeru service
	if areCompletedSemeruInstancesMarkedToBeDeleted && r.isOpenLibertyApplicationReady(instance) {
		if err := r.deleteCompletedSemeruInstances(instance); err != nil {
			reqLogger.Error(err, "Failed to delete completed Semeru instance")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	}

	instance.Status.Versions.Reconciled = olutils.OperandVersion
	reqLogger.Info("Reconcile " + APIName + " - completed")
	return r.ManageSuccess(common.StatusConditionTypeReconciled, instance)
}

func (r *ReconcileOpenLiberty) isOpenLibertyApplicationReady(ba common.BaseComponent) bool {
	if r.CheckApplicationStatus(ba) == corev1.ConditionTrue {
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

	watchNamespaces, err := appstacksutils.GetWatchNamespaces()
	if err != nil {
		r.Log.Error(err, "Failed to get watch namespace")
		os.Exit(1)
	}

	watchNamespacesMap := make(map[string]bool)
	for _, ns := range watchNamespaces {
		watchNamespacesMap[ns] = true
	}
	isClusterWide := appstacksutils.IsClusterWide(watchNamespaces)

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

	b := ctrl.NewControllerManagedBy(mgr).For(&openlibertyv1.OpenLibertyApplication{}, builder.WithPredicates(pred)).
		Owns(&corev1.Service{}, builder.WithPredicates(predSubResource)).
		Owns(&corev1.Secret{}, builder.WithPredicates(predSubResource)).
		Owns(&appsv1.Deployment{}, builder.WithPredicates(predSubResWithGenCheck)).
		Owns(&appsv1.StatefulSet{}, builder.WithPredicates(predSubResWithGenCheck)).
		Owns(&autoscalingv1.HorizontalPodAutoscaler{}, builder.WithPredicates(predSubResource))

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
		b = b.Watches(&source.Kind{Type: &imagev1.ImageStream{}}, &EnqueueRequestsForCustomIndexField{
			Matcher: &ImageStreamMatcher{
				Klient:          mgr.GetClient(),
				WatchNamespaces: watchNamespaces,
			},
		})
	}
	return b.Complete(r)
}

func getMonitoringEnabledLabelName(ba common.BaseComponent) string {
	return "monitor." + ba.GetGroupName() + "/enabled"
}

func (r *ReconcileOpenLiberty) finalizeOpenLibertyApplication(reqLogger logr.Logger, olapp *openlibertyv1.OpenLibertyApplication, pvcName string, pvcNamespace string) error {
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
