package openliberty

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/appsody/appsody-operator/pkg/common"

	lutils "github.com/OpenLiberty/open-liberty-operator/pkg/utils"
	autils "github.com/appsody/appsody-operator/pkg/utils"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	prometheusv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_openlibertyapplication")

//Constant Values
const ssoSecretNameSuffix = "-olapp-sso"

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new OpenLiberty Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	reconciler := &ReconcileOpenLiberty{ReconcilerBase: autils.NewReconcilerBase(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig(), mgr.GetEventRecorderFor(("open-liberty-operator")))}
	return reconciler
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("openliberty-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: 10})
	if err != nil {
		return err
	}

	watchNamespaces, err := autils.GetWatchNamespaces()
	if err != nil {
		log.Error(err, "Failed to get watch namespace")
		os.Exit(1)
	}

	watchNamespacesMap := make(map[string]bool)
	for _, ns := range watchNamespaces {
		watchNamespacesMap[ns] = true
	}
	isClusterWide := len(watchNamespacesMap) == 1 && watchNamespacesMap[""]

	log.V(1).Info("Adding a new controller", "watchNamespaces", watchNamespaces, "isClusterWide", isClusterWide)

	pred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ignore updates to CR status in which case metadata.Generation does not change
			return e.MetaOld.GetGeneration() != e.MetaNew.GetGeneration() && (isClusterWide || watchNamespacesMap[e.MetaOld.GetNamespace()])
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return isClusterWide || watchNamespacesMap[e.Meta.GetNamespace()]
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return isClusterWide || watchNamespacesMap[e.Meta.GetNamespace()]
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return isClusterWide || watchNamespacesMap[e.Meta.GetNamespace()]
		},
	}

	// Watch for changes to primary resource OpenLiberty
	err = c.Watch(&source.Kind{Type: &openlibertyv1beta1.OpenLibertyApplication{}}, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner OpenLiberty
	// err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
	// 	IsController: true,
	// 	OwnerType:    &openlibertyv1beta1.OpenLibertyApplication{},
	// })
	// if err != nil {
	// 	return err
	// }

	predSubResource := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ignore updates to CR status in which case metadata.Generation does not change
			e.ObjectOld.DeepCopyObject()
			return (isClusterWide || watchNamespacesMap[e.MetaOld.GetNamespace()])
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return isClusterWide || watchNamespacesMap[e.Meta.GetNamespace()]
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	// err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
	// 	IsController: true,
	// 	OwnerType:    &openlibertyv1beta1.OpenLibertyApplication{},
	// }, predSubResource)
	// if err != nil {
	// 	return err
	// }

	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &openlibertyv1beta1.OpenLibertyApplication{},
	}, predSubResource)
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &openlibertyv1beta1.OpenLibertyApplication{},
	}, predSubResource)
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &autoscalingv1.HorizontalPodAutoscaler{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &openlibertyv1beta1.OpenLibertyApplication{},
	}, predSubResource)
	if err != nil {
		return err
	}

	// Setup watch for SSO secret
	err = c.Watch(
		&source.Kind{Type: &corev1.Secret{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
				if !strings.HasSuffix(a.Meta.GetName(), ssoSecretNameSuffix) {
					return nil
				}
				return []reconcile.Request{
					{NamespacedName: types.NamespacedName{
						Name:      strings.NewReplacer(ssoSecretNameSuffix, "").Replace(a.Meta.GetName()),
						Namespace: a.Meta.GetNamespace(),
					}},
				}
			}),
		})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &routev1.Route{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &openlibertyv1beta1.OpenLibertyApplication{},
	}, predSubResource)

	err = c.Watch(&source.Kind{Type: &servingv1alpha1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &openlibertyv1beta1.OpenLibertyApplication{},
	}, predSubResource)

	return nil
}

// blank assignment to verify that ReconcileOpenLiberty implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileOpenLiberty{}

// ReconcileOpenLiberty reconciles a OpenLiberty object
type ReconcileOpenLiberty struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	autils.ReconcilerBase
}

// Reconcile reads that state of the cluster for a OpenLiberty object and makes changes based on the state read
// and what is in the OpenLiberty.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileOpenLiberty) Reconcile(request reconcile.Request) (reconcile.Result, error) {

	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling OpenLibertyApplication")

	// Fetch the OpenLiberty instance
	instance := &openlibertyv1beta1.OpenLibertyApplication{}
	var ba common.BaseApplication
	ba = instance
	err := r.GetClient().Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	_, err = autils.Validate(instance)
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

	instance.Initialize()

	currentGen := instance.Generation
	err = r.GetClient().Update(context.TODO(), instance)
	if err != nil {
		reqLogger.Error(err, "Error updating OpenLibertyApplication")
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	if currentGen == 1 {
		return reconcile.Result{}, nil
	}

	defaultMeta := metav1.ObjectMeta{
		Name:      instance.Name,
		Namespace: instance.Namespace,
	}

	result, err := r.ReconcileProvides(instance)
	if err != nil || result != (reconcile.Result{}) {
		return result, err
	}

	result, err = r.ReconcileConsumes(instance)
	if err != nil || result != (reconcile.Result{}) {
		return result, err
	}

	if instance.Spec.ServiceAccountName == nil || *instance.Spec.ServiceAccountName == "" {
		serviceAccount := &corev1.ServiceAccount{ObjectMeta: defaultMeta}
		err = r.CreateOrUpdate(serviceAccount, instance, func() error {
			autils.CustomizeServiceAccount(serviceAccount, instance)
			return nil
		})
		if err != nil {
			reqLogger.Error(err, "Failed to reconcile ServiceAccount")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	} else {
		serviceAccount := &corev1.ServiceAccount{ObjectMeta: defaultMeta}
		err = r.DeleteResource(serviceAccount)
		if err != nil {
			reqLogger.Error(err, "Failed to delete ServiceAccount")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	}

	if instance.Spec.CreateKnativeService != nil && *instance.Spec.CreateKnativeService {
		ksvc := &servingv1alpha1.Service{ObjectMeta: defaultMeta}
		err = r.CreateOrUpdate(ksvc, instance, func() error {
			autils.CustomizeKnativeService(ksvc, instance)
			return nil
		})

		if err != nil {
			reqLogger.Error(err, "Failed to reconcile Knative Service")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}

		// Clean up non-Knative resources
		resources := []runtime.Object{
			&corev1.Service{ObjectMeta: defaultMeta},
			&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: instance.Name + "-headless", Namespace: instance.Namespace}},
			&appsv1.Deployment{ObjectMeta: defaultMeta},
			&appsv1.StatefulSet{ObjectMeta: defaultMeta},
			&routev1.Route{ObjectMeta: defaultMeta},
			&autoscalingv1.HorizontalPodAutoscaler{ObjectMeta: defaultMeta},
		}
		err = r.DeleteResources(resources)
		if err != nil {
			reqLogger.Error(err, "Failed to clean up non-Knative resources")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}

		return r.ManageSuccess(common.StatusConditionTypeReconciled, instance)
	}

	// Check if Knative is supported and delete Knative service if supported
	if ok, err := r.IsGroupVersionSupported(servingv1alpha1.SchemeGroupVersion.String()); err != nil {
		reqLogger.Error(err, fmt.Sprintf("Failed to check if %s is supported", servingv1alpha1.SchemeGroupVersion.String()))
		r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	} else if ok {
		ksvc := &servingv1alpha1.Service{ObjectMeta: defaultMeta}
		err = r.DeleteResource(ksvc)
		if err != nil {
			reqLogger.Error(err, "Failed to delete Knative Service")
			r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
	} else {
		reqLogger.V(1).Info(fmt.Sprintf("%s is not supported. Skip deleting the resource", servingv1alpha1.SchemeGroupVersion.String()))
	}

	svc := &corev1.Service{ObjectMeta: defaultMeta}
	err = r.CreateOrUpdate(svc, instance, func() error {
		autils.CustomizeService(svc, ba)
		svc.Annotations = instance.Spec.Service.Annotations
		if instance.Spec.Monitoring != nil {
			svc.Labels["app."+ba.GetGroupName()+"/monitor"] = "true"
		} else {
			if _, ok := svc.Labels["app."+ba.GetGroupName()+"/monitor"]; ok {
				delete(svc.Labels, "app."+ba.GetGroupName()+"/monitor")
			}
		}
		return nil
	})
	if err != nil {
		reqLogger.Error(err, "Failed to reconcile Service")
		return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	}

	if instance.Spec.Serviceability != nil {
		if instance.Spec.Serviceability.VolumeClaimName != "" {
			pvcName := instance.Spec.Serviceability.VolumeClaimName
			err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: pvcName, Namespace: instance.Namespace}, &corev1.PersistentVolumeClaim{})
			if err != nil && errors.IsNotFound(err) {
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
	}

	if instance.Spec.Storage != nil {
		// Delete Deployment if exists
		deploy := &appsv1.Deployment{ObjectMeta: defaultMeta}
		err = r.DeleteResource(deploy)

		if err != nil {
			reqLogger.Error(err, "Failed to delete Deployment")
			return r.ManageError(err, common.StatusConditionTypeReconciled, instance)
		}
		svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: instance.Name + "-headless", Namespace: instance.Namespace}}
		err = r.CreateOrUpdate(svc, instance, func() error {
			autils.CustomizeService(svc, instance)
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
			autils.CustomizeStatefulSet(statefulSet, instance)
			autils.CustomizePodSpec(&statefulSet.Spec.Template, instance)
			autils.CustomizePersistence(statefulSet, instance)
			lutils.CustomizeLibertyEnv(&statefulSet.Spec.Template, instance)
			if instance.GetSSO() != nil {
				secretName := instance.GetName() + ssoSecretNameSuffix
				ssoSecret := &corev1.Secret{}
				err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: instance.GetNamespace()}, ssoSecret)
				if err != nil {
					reqLogger.Error(nil, "Secret for Single sign-on (SSO) was not found. Create a secret named '"+secretName+"' in namespace '"+instance.GetNamespace()+"' with the credentials for the login providers you selected in application image.")
					return err
				}
				lutils.CustomizeSSOEnv(&statefulSet.Spec.Template, instance, ssoSecret)
			}
			lutils.ConfigureServiceability(&statefulSet.Spec.Template, instance)
			if instance.Spec.CreateAppDefinition == nil || *instance.Spec.CreateAppDefinition {
				m := make(map[string]string)
				m["kappnav.subkind"] = "Liberty"
				statefulSet.Annotations = autils.MergeMaps(statefulSet.GetAnnotations(), m)
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
			autils.CustomizeDeployment(deploy, instance)
			autils.CustomizePodSpec(&deploy.Spec.Template, instance)
			lutils.CustomizeLibertyEnv(&deploy.Spec.Template, instance)
			//*&deploy.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{}
			if instance.GetSSO() != nil {
				secretName := instance.GetName() + ssoSecretNameSuffix
				ssoSecret := &corev1.Secret{}
				err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: instance.GetNamespace()}, ssoSecret)
				if err != nil {
					reqLogger.Error(nil, "Secret for Single sign-on (SSO) was not found. Create a secret named '"+secretName+"' in namespace '"+instance.GetNamespace()+"' with the credentials for the login providers you selected in application image.")
					return err
				}
				lutils.CustomizeSSOEnv(&deploy.Spec.Template, instance, ssoSecret)
			}

			lutils.ConfigureServiceability(&deploy.Spec.Template, instance)
			if instance.Spec.CreateAppDefinition == nil || *instance.Spec.CreateAppDefinition {
				m := make(map[string]string)
				m["kappnav.subkind"] = "Liberty"
				deploy.Annotations = autils.MergeMaps(deploy.GetAnnotations(), m)
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
			autils.CustomizeHPA(hpa, instance)
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

	if ok, err := r.IsGroupVersionSupported(routev1.SchemeGroupVersion.String()); err != nil {
		reqLogger.Error(err, fmt.Sprintf("Failed to check if %s is supported", routev1.SchemeGroupVersion.String()))
		r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	} else if ok {
		if instance.Spec.Expose != nil && *instance.Spec.Expose {
			route := &routev1.Route{ObjectMeta: defaultMeta}
			err = r.CreateOrUpdate(route, instance, func() error {
				autils.CustomizeRoute(route, instance)
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
		reqLogger.V(1).Info(fmt.Sprintf("%s is not supported", routev1.SchemeGroupVersion.String()))
	}

	if ok, err := r.IsGroupVersionSupported(prometheusv1.SchemeGroupVersion.String()); err != nil {
		reqLogger.Error(err, fmt.Sprintf("Failed to check if %s is supported", routev1.SchemeGroupVersion.String()))
		r.ManageError(err, common.StatusConditionTypeReconciled, instance)
	} else if ok {
		if instance.Spec.Monitoring != nil && (instance.Spec.CreateKnativeService == nil || !*instance.Spec.CreateKnativeService) {
			sm := &prometheusv1.ServiceMonitor{ObjectMeta: defaultMeta}
			err = r.CreateOrUpdate(sm, instance, func() error {
				autils.CustomizeServiceMonitor(sm, instance)
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
		reqLogger.V(1).Info(fmt.Sprintf("%s is not supported", routev1.SchemeGroupVersion.String()))
	}

	return r.ManageSuccess(common.StatusConditionTypeReconciled, instance)
}
