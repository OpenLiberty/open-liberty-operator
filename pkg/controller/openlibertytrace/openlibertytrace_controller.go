package openlibertytrace

import (
	"context"
	"os"
	"time"

	autils "github.com/appsody/appsody-operator/pkg/utils"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	"github.com/OpenLiberty/open-liberty-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_openlibertytrace")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new OpenLibertyTrace Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileOpenLibertyTrace{client: mgr.GetClient(), scheme: mgr.GetScheme(), restConfig: mgr.GetConfig()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("openlibertytrace-controller", mgr, controller.Options{Reconciler: r})
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

	// Watch for changes to primary resource OpenLibertyTrace
	err = c.Watch(&source.Kind{Type: &openlibertyv1beta1.OpenLibertyTrace{}}, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}

	/* TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner OpenLibertyTrace
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &openlibertyv1beta1.OpenLibertyTrace{},
	})
	if err != nil {
		return err
	}*/

	return nil
}

// blank assignment to verify that ReconcileOpenLibertyTrace implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileOpenLibertyTrace{}

// ReconcileOpenLibertyTrace reconciles a OpenLibertyTrace object
type ReconcileOpenLibertyTrace struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client     client.Client
	scheme     *runtime.Scheme
	restConfig *rest.Config
}

// Reconcile reads that state of the cluster for a OpenLibertyTrace object and makes changes based on the state read
// and what is in the OpenLibertyTrace.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileOpenLibertyTrace) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling OpenLibertyTrace")
	// Fetch the OpenLibertyTrace instance
	instance := &openlibertyv1beta1.OpenLibertyTrace{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Not found. Return and don't requeue")
			return reconcile.Result{}, nil
		}
		reqLogger.Info("Error reading the object - requeue the request.")
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	//Pod is expected to be from the same namespace as the CR instance
	podNamespace := instance.Namespace
	podName := instance.Spec.PodName

	prevPodName := instance.GetStatus().GetOperatedResource().GetOperatedResourceName()
	prevTraceEnabled := instance.GetStatus().GetCondition(openlibertyv1beta1.OperationStatusConditionTypeTrace).Status
	podChanged := prevPodName != podName

	//If pod name changed, then stop tracing on previous pod (if trace was enabled on it)
	if podChanged && (prevTraceEnabled == corev1.ConditionTrue) {
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: prevPodName, Namespace: podNamespace}, &corev1.Pod{})
		if err != nil && errors.IsNotFound(err) {
			//Previous Pod is not found. No-op
			reqLogger.Info("Previous pod " + prevPodName + " was not found in namespace " + podNamespace)
		} else {
			//Stop tracing on previous Pod
			_, err = utils.ExecuteCommandInContainer(r.restConfig, prevPodName, podNamespace, "app", []string{"/bin/sh", "-c", "rm /config/configDropins/overrides/add_trace.xml", "."})
			if err == nil {
				reqLogger.Info("Disabled trace on previous pod " + prevPodName + " of namespace " + podNamespace)
			} else {
				reqLogger.Error(err, "Encountered error while disabling trace on previous pod "+podName+" of namespace "+podNamespace)
			}
		}
	} else {
		reqLogger.Info("No clean up was needed")
	}

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: podName, Namespace: podNamespace}, &corev1.Pod{})
	if err != nil && errors.IsNotFound(err) {
		//Pod is not found. Return and don't requeue
		reqLogger.Info("Pod " + podName + " was not found in namespace " + podNamespace)
		return r.UpdateStatus(openlibertyv1beta1.OperationStatusConditionTypeTrace, *instance, corev1.ConditionFalse, podName, podChanged)
	}

	if instance.Spec.DisableTrace != nil && *instance.Spec.DisableTrace {
		//Disable trace if trace was previously enabled on same pod
		if !podChanged && prevTraceEnabled == corev1.ConditionTrue {
			_, err = utils.ExecuteCommandInContainer(r.restConfig, podName, podNamespace, "app", []string{"/bin/sh", "-c", "rm /config/configDropins/overrides/add_trace.xml", "."})
			if err == nil {
				reqLogger.Info("Disabled trace for " + podName + " of namespace " + podNamespace)
				return r.UpdateStatus(openlibertyv1beta1.OperationStatusConditionTypeTrace, *instance, corev1.ConditionFalse, podName, podChanged)
			}
			reqLogger.Error(err, "Encountered error while disabling trace for "+podName+" of namespace "+podNamespace)
		} else {
			r.UpdateStatus(openlibertyv1beta1.OperationStatusConditionTypeTrace, *instance, corev1.ConditionFalse, podName, podChanged)
		}
	} else {
		_, err = utils.ExecuteCommandInContainer(r.restConfig, podName, podNamespace, "app", []string{"/bin/sh", "-c", "echo '<server><logging traceSpecification=\"" + instance.Spec.TraceSpecification + "\" logDirectory=\"/liberty/serviceability/logs/" + podNamespace + "/" + podName + "\" traceFileName=\"trace.log\" maxFileSize=\"" + instance.Spec.MaxFileSize + "\" maxFiles=\"" + instance.Spec.MaxFiles + "\"/></server>' > /config/configDropins/overrides/add_trace.xml", "."})
		if err == nil {
			if podChanged || prevTraceEnabled == corev1.ConditionFalse {
				reqLogger.Info("Enabled trace for " + podName + " of namespace " + podNamespace)
			} else {
				reqLogger.Info("Updated trace for " + podName + " of namespace " + podNamespace)
			}
			return r.UpdateStatus(openlibertyv1beta1.OperationStatusConditionTypeTrace, *instance, corev1.ConditionTrue, podName, podChanged)
		}
		reqLogger.Error(err, "Encountered error while enabling trace for "+podName+" of namespace "+podNamespace)
	}

	// reqLogger.Info("Updated status. Completed reconcile")
	return reconcile.Result{Requeue: false}, nil
}

// UpdateStatus ...
func (r *ReconcileOpenLibertyTrace) UpdateStatus(conditionType openlibertyv1beta1.OperationStatusConditionType, instance openlibertyv1beta1.OpenLibertyTrace, traceStatus corev1.ConditionStatus, podName string, podChanged bool) (reconcile.Result, error) {
	s := instance.GetStatus()

	s.SetOperatedResource(openlibertyv1beta1.OperatedResource{ResourceName: podName, ResourceType: "pod"})

	oldCondition := s.GetCondition(conditionType)
	// Keep the old `LastTransitionTime` when status has not changed
	nowTime := metav1.Now()
	transitionTime := oldCondition.GetLastTransitionTime()
	if podChanged || oldCondition.GetStatus() != traceStatus {
		transitionTime = &nowTime
	}

	statusCondition := s.NewCondition()
	statusCondition.SetLastTransitionTime(transitionTime)
	statusCondition.SetLastUpdateTime(nowTime)
	statusCondition.SetReason("")
	statusCondition.SetMessage("")
	statusCondition.SetStatus(traceStatus)

	s.SetCondition(statusCondition)

	err := r.client.Status().Update(context.Background(), &instance)
	if err != nil {
		log.Error(err, "Unable to update status")
		return reconcile.Result{
			RequeueAfter: time.Second,
			Requeue:      true,
		}, nil
	}

	log.Info("Updated status")
	return reconcile.Result{Requeue: false}, nil
}
