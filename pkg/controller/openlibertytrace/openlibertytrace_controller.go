package openlibertytrace

import (
	"context"
	"os"
	"strconv"
	"time"

	oputils "github.com/application-stacks/runtime-component-operator/pkg/utils"
	"github.com/go-logr/logr"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	"github.com/OpenLiberty/open-liberty-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
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

// Add creates a new OpenLibertyTrace Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileOpenLibertyTrace{client: mgr.GetClient(), scheme: mgr.GetScheme(), recorder: mgr.GetEventRecorderFor("open-liberty-operator"), restConfig: mgr.GetConfig()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("openlibertytrace-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	watchNamespaces, err := oputils.GetWatchNamespaces()
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
	recorder   record.EventRecorder
	restConfig *rest.Config
}

const traceFinalizer = "finalizer.openlibertytraces.openliberty.io"
const traceConfigFile = "/config/configDropins/overrides/add_trace.xml"
const serviceabilityDir = "/serviceability"

// Reconcile reads that state of the cluster for a OpenLibertyTrace object and makes changes based on the state read
// and what is in the OpenLibertyTrace.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileOpenLibertyTrace) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Namespace", request.Namespace, "Name", request.Name)
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
	prevTraceEnabled := instance.GetStatus().GetCondition(openlibertyv1beta1.OperationStatusConditionTypeEnabled).Status
	podChanged := prevPodName != podName

	// Check if the OpenLibertyTrace instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isInstanceMarkedToBeDeleted := instance.GetDeletionTimestamp() != nil
	if isInstanceMarkedToBeDeleted {
		if contains(instance.GetFinalizers(), traceFinalizer) {
			// Run finalization logic for traceFinalizer. If the finalization logic fails, don't remove the
			// finalizer so that we can retry during the next reconciliation.
			if err := r.finalizeOpenLibertyTrace(reqLogger, instance, prevTraceEnabled, prevPodName, podNamespace); err != nil {
				return reconcile.Result{}, err
			}

			// Remove traceFinalizer. Once all finalizers have been removed, the object will be deleted.
			instance.SetFinalizers(remove(instance.GetFinalizers(), traceFinalizer))
			err := r.client.Update(context.TODO(), instance)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// Add finalizer for this CR
	if !contains(instance.GetFinalizers(), traceFinalizer) {
		if err := r.addFinalizer(reqLogger, instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	//If pod name changed, then stop tracing on previous pod (if trace was enabled on it)
	if podChanged && (prevTraceEnabled == corev1.ConditionTrue) {
		r.disableTraceOnPrevPod(reqLogger, prevPodName, podNamespace)
	}

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: podName, Namespace: podNamespace}, &corev1.Pod{})
	if err != nil && errors.IsNotFound(err) {
		//Pod is not found. Return and don't requeue
		reqLogger.Error(err, "Pod "+podName+" was not found in namespace "+podNamespace)
		return r.UpdateStatus(err, openlibertyv1beta1.OperationStatusConditionTypeEnabled, *instance, corev1.ConditionFalse, podName, podChanged)
	}

	if instance.Spec.Disable != nil && *instance.Spec.Disable {
		//Disable trace if trace was previously enabled on the same pod
		if !podChanged && prevTraceEnabled == corev1.ConditionTrue {
			_, err = utils.ExecuteCommandInContainer(r.restConfig, podName, podNamespace, "app", []string{"/bin/sh", "-c", "rm -f " + traceConfigFile})
			if err != nil {
				reqLogger.Error(err, "Encountered error while disabling trace for pod "+podName+" in namespace "+podNamespace)
				return r.UpdateStatus(err, openlibertyv1beta1.OperationStatusConditionTypeEnabled, *instance, corev1.ConditionTrue, podName, podChanged)
			}
			reqLogger.Info("Disabled trace for pod " + podName + " in namespace " + podNamespace)
		}
		r.UpdateStatus(nil, openlibertyv1beta1.OperationStatusConditionTypeEnabled, *instance, corev1.ConditionFalse, podName, podChanged)
	} else {
		traceOutputDir := serviceabilityDir + "/" + podNamespace + "/" + podName
		traceConfig := "<server><logging traceSpecification=\"" + instance.Spec.TraceSpecification + "\" logDirectory=\"" + traceOutputDir + "\""
		if instance.Spec.MaxFileSize != nil {
			traceConfig += " maxFileSize=\"" + strconv.Itoa(int(*instance.Spec.MaxFileSize)) + "\""
		}
		if instance.Spec.MaxFiles != nil {
			traceConfig += " maxFiles=\"" + strconv.Itoa(int(*instance.Spec.MaxFiles)) + "\""
		}
		traceConfig += "/></server>"

		_, err = utils.ExecuteCommandInContainer(r.restConfig, podName, podNamespace, "app", []string{"/bin/sh", "-c", "mkdir -p " + traceOutputDir + " && echo '" + traceConfig + "' > " + traceConfigFile})
		if err != nil {
			reqLogger.Error(err, "Encountered error while setting up trace for pod "+podName+" in namespace "+podNamespace)
			return r.UpdateStatus(err, openlibertyv1beta1.OperationStatusConditionTypeEnabled, *instance, corev1.ConditionFalse, podName, podChanged)
		}

		if podChanged || prevTraceEnabled == corev1.ConditionFalse {
			reqLogger.Info("Enabled trace for pod " + podName + " in namespace " + podNamespace)
		} else {
			reqLogger.Info("Updated trace for pod " + podName + " in namespace " + podNamespace)
		}
		r.UpdateStatus(nil, openlibertyv1beta1.OperationStatusConditionTypeEnabled, *instance, corev1.ConditionTrue, podName, podChanged)
	}

	return reconcile.Result{}, nil
}

// UpdateStatus updates the status
func (r *ReconcileOpenLibertyTrace) UpdateStatus(issue error, conditionType openlibertyv1beta1.OperationStatusConditionType, instance openlibertyv1beta1.OpenLibertyTrace, newStatus corev1.ConditionStatus, podName string, podChanged bool) (reconcile.Result, error) {
	s := instance.GetStatus()

	s.SetOperatedResource(openlibertyv1beta1.OperatedResource{ResourceName: podName, ResourceType: "pod"})

	oldCondition := s.GetCondition(conditionType)
	// Keep the old `LastTransitionTime` when pod and status have not changed
	nowTime := metav1.Now()
	transitionTime := oldCondition.GetLastTransitionTime()
	if podChanged || oldCondition.GetStatus() != newStatus {
		transitionTime = &nowTime
	}

	statusCondition := s.NewCondition()
	statusCondition.SetLastTransitionTime(transitionTime)
	statusCondition.SetLastUpdateTime(nowTime)

	if issue != nil {
		statusCondition.SetReason("Error")
		statusCondition.SetMessage(issue.Error())
		r.recorder.Event(&instance, "Warning", "ProcessingError", issue.Error())
	} else {
		statusCondition.SetReason("")
		statusCondition.SetMessage("")
	}

	statusCondition.SetStatus(newStatus)
	statusCondition.SetType(conditionType)

	s.SetCondition(statusCondition)

	err := r.client.Status().Update(context.Background(), &instance)
	if err != nil {
		log.Error(err, "Unable to update status")
		return reconcile.Result{
			RequeueAfter: time.Second,
			Requeue:      true,
		}, nil
	}

	return reconcile.Result{Requeue: false}, nil
}

func (r *ReconcileOpenLibertyTrace) disableTraceOnPrevPod(reqLogger logr.Logger, prevPodName string, podNamespace string) {
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: prevPodName, Namespace: podNamespace}, &corev1.Pod{})
	if err != nil && errors.IsNotFound(err) {
		//Previous Pod is not found. No-op
		reqLogger.Info("Previous pod " + prevPodName + " was not found in namespace " + podNamespace)
	} else {
		//Stop tracing on previous Pod
		_, err = utils.ExecuteCommandInContainer(r.restConfig, prevPodName, podNamespace, "app", []string{"/bin/sh", "-c", "rm -f " + traceConfigFile})
		if err == nil {
			reqLogger.Info("Disabled trace on previous pod " + prevPodName + " in namespace " + podNamespace)
		} else {
			reqLogger.Error(err, "Encountered error while disabling trace on previous pod "+prevPodName+" in namespace "+podNamespace)
		}
	}
}

func (r *ReconcileOpenLibertyTrace) finalizeOpenLibertyTrace(reqLogger logr.Logger, olt *openlibertyv1beta1.OpenLibertyTrace, prevTraceEnabled corev1.ConditionStatus, prevPodName string, podNamespace string) error {
	if prevTraceEnabled == corev1.ConditionTrue {
		r.disableTraceOnPrevPod(reqLogger, prevPodName, podNamespace)
	}
	return nil
}

func (r *ReconcileOpenLibertyTrace) addFinalizer(reqLogger logr.Logger, olt *openlibertyv1beta1.OpenLibertyTrace) error {
	reqLogger.Info("Adding Finalizer for OpenLibertyTrace")
	olt.SetFinalizers(append(olt.GetFinalizers(), traceFinalizer))

	// Update CR
	err := r.client.Update(context.TODO(), olt)
	if err != nil {
		reqLogger.Error(err, "Failed to update OpenLibertyTrace with finalizer")
		return err
	}

	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func remove(list []string, s string) []string {
	for i, v := range list {
		if v == s {
			list = append(list[:i], list[i+1:]...)
		}
	}
	return list
}
