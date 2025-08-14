package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/OpenLiberty/open-liberty-operator/utils"
	"github.com/application-stacks/runtime-component-operator/common"
	oputils "github.com/application-stacks/runtime-component-operator/utils"
	"github.com/go-logr/logr"

	openlibertyv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const performanceDataFinalizer = "finalizer.openlibertyperformancedata.apps.openliberty.io"

// ReconcileOpenLibertyPerformanceData reconciles an OpenLibertyPerformanceData object
type ReconcileOpenLibertyPerformanceData struct {
	// This client, initialized using mgr.GetClient()() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	oputils.ReconcilerBase
	Log               logr.Logger
	PodInjectorClient utils.PodInjectorClient
	watchNamespaces   []string
}

// +kubebuilder:rbac:groups=apps.openliberty.io,resources=openlibertyperformancedata;openlibertyperformancedata/status;openlibertyperformancedata/finalizers,verbs=get;list;watch;create;update;patch;delete,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=core,resources=pods;pods/exec,verbs=get;list;watch;create;update;patch;delete,namespace=open-liberty-operator

// Reconcile reads that state of the cluster for an OpenLibertyPerformanceData object and makes changes based on the state read
// and what is in the OpenLibertyPerformanceData.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.

func (r *ReconcileOpenLibertyPerformanceData) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling OpenLibertyPerformanceData")

	// Fetch the OpenLibertyPerformanceData instance
	instance := &openlibertyv1.OpenLibertyPerformanceData{}
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

	// Check if the OpenLibertyPerformanceData instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isInstanceMarkedToBeDeleted := instance.GetDeletionTimestamp() != nil
	if isInstanceMarkedToBeDeleted {
		if utils.Contains(instance.GetFinalizers(), performanceDataFinalizer) {
			// Run finalization logic for performanceDataFinalizer. If the finalization logic fails, don't remove the
			// finalizer so that we can retry during the next reconciliation.
			if err := r.finalizeOpenLibertyPerformanceData(reqLogger, instance); err != nil {
				return reconcile.Result{}, err
			}

			// Remove performanceDataFinalizer. Once all finalizers have been removed, the object will be deleted.
			instance.SetFinalizers(utils.Remove(instance.GetFinalizers(), performanceDataFinalizer))
			err := r.GetClient().Update(context.TODO(), instance)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// Add finalizer for this CR
	if !utils.Contains(instance.GetFinalizers(), performanceDataFinalizer) {
		if err := r.addFinalizer(reqLogger, instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	//do not reconcile if performance data collection already completed
	oc := openlibertyv1.GetOperationCondtion(instance.Status.Conditions, openlibertyv1.OperationStatusConditionTypeCompleted)
	if oc != nil && oc.Status == corev1.ConditionTrue {
		return reconcile.Result{}, err
	}

	//check if Pod exists and running
	pod := &corev1.Pod{}

	err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: instance.Spec.PodName, Namespace: request.Namespace}, pod)
	if err != nil && kerrors.IsNotFound(err) {
		message := fmt.Sprintf("Failed to find pod %s in namespace %s", instance.Spec.PodName, request.Namespace)
		var errMessage string
		isWritingPerformanceData := isPerformanceDataRunning(instance)
		if isWritingPerformanceData {
			errMessage = utils.GetPerformanceDataConnectionLostMessage(instance.Spec.PodName)
		} else {
			errMessage = "Failed to find a pod or pod is not in running state"
		}

		reqLogger.Error(err, message)
		r.GetRecorder().Event(instance, "Warning", "ProcessingError", message)
		// Set Started condition
		c := openlibertyv1.OperationStatusCondition{
			Type:   openlibertyv1.OperationStatusConditionTypeStarted,
			Status: corev1.ConditionTrue,
		}
		instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
		// Set Completed condition
		c = openlibertyv1.OperationStatusCondition{
			Type:    openlibertyv1.OperationStatusConditionTypeCompleted,
			Status:  corev1.ConditionFalse,
			Reason:  "Error",
			Message: errMessage,
		}
		instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
		instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
		instance.Status.Versions.Reconciled = utils.OperandVersion
		r.GetClient().Status().Update(context.TODO(), instance)
		if isWritingPerformanceData {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	} else if pod.Status.Phase != corev1.PodRunning {
		c := openlibertyv1.OperationStatusCondition{
			Type:    openlibertyv1.OperationStatusConditionTypeStarted,
			Status:  corev1.ConditionFalse,
			Reason:  "Error",
			Message: fmt.Sprintf("Waiting for Pod '%s' to be in a running state.", pod.Name),
		}
		instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
		instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
		instance.Status.Versions.Reconciled = utils.OperandVersion
		r.GetClient().Status().Update(context.TODO(), instance)
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
		}, nil
	}

	if r.PodInjectorClient.Connect() != nil {
		message := "Failed to connect to the operator pod injector"
		reqLogger.Error(err, message)
		r.GetRecorder().Event(instance, "Warning", "ProcessingError", message)
		c := openlibertyv1.OperationStatusCondition{
			Type:    openlibertyv1.OperationStatusConditionTypeStarted,
			Status:  corev1.ConditionFalse,
			Reason:  "Error",
			Message: message,
		}
		instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
		instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
		instance.Status.Versions.Reconciled = utils.OperandVersion
		r.GetClient().Status().Update(context.TODO(), instance)
		return reconcile.Result{}, nil
	} else {
		c := openlibertyv1.OperationStatusCondition{
			Type:   openlibertyv1.OperationStatusConditionTypeStarted,
			Status: corev1.ConditionTrue,
		}
		instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
		r.GetClient().Status().Update(context.TODO(), instance)
	}
	defer r.PodInjectorClient.CloseConnection()

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

	// Update setttings based on operator config map
	configMap, err := r.GetOpConfigMap(OperatorName, ns)
	if err != nil {
		reqLogger.Info("Failed to get open-liberty-operator config map, error: " + err.Error())
		oputils.CreateConfigMap(OperatorName)
	} else {
		common.LoadFromConfigMapWithAddedDefaults(common.Config, configMap, utils.DefaultLibertyOpConfig)
	}

	maxWorkers := common.LoadFromConfig(common.Config, utils.OpConfigPerformanceDataMaxWorkers)
	if maxWorkers != "" {
		r.PodInjectorClient.SetMaxWorkers("linperf", pod.Name, pod.Namespace, maxWorkers)
	}

	var c openlibertyv1.OperationStatusCondition
	injectorStatus := r.PodInjectorClient.PollStatus("linperf", pod.Name, pod.Namespace, utils.EncodeLinperfAttr(instance))
	if injectorStatus != "done..." {
		// exit on error
		if strings.HasPrefix(injectorStatus, "error:") {
			errMessage := strings.TrimPrefix(injectorStatus, "error:")
			err = fmt.Errorf("%s", errMessage)
			reqLogger.Error(err, errMessage)
			r.GetRecorder().Event(instance, "Warning", "ProcessingError", err.Error())
			c = openlibertyv1.OperationStatusCondition{
				Type:   openlibertyv1.OperationStatusConditionTypeCompleted,
				Status: corev1.ConditionFalse,
			}
			instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
			c = openlibertyv1.OperationStatusCondition{
				Type:    openlibertyv1.OperationStatusConditionTypeFailed,
				Status:  corev1.ConditionTrue,
				Reason:  "Error",
				Message: err.Error(),
			}
			instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
			c = openlibertyv1.OperationStatusCondition{
				Type:    openlibertyv1.OperationStatusConditionTypeFailed,
				Status:  corev1.ConditionTrue,
				Reason:  "Error",
				Message: err.Error(),
			}
			instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
			instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
			instance.Status.Versions.Reconciled = utils.OperandVersion
			r.GetClient().Status().Update(context.TODO(), instance)
			return reconcile.Result{}, nil
		}
		// exit on connection loss
		if injectorStatus != "writing..." && isPerformanceDataRunning(instance) {
			errMessage := utils.GetPerformanceDataConnectionLostMessage(pod.Name)
			err = fmt.Errorf("%s", errMessage)
			reqLogger.Error(err, errMessage)
			r.GetRecorder().Event(instance, "Warning", "ProcessingError", err.Error())
			c = openlibertyv1.OperationStatusCondition{
				Type:   openlibertyv1.OperationStatusConditionTypeCompleted,
				Status: corev1.ConditionFalse,
			}
			instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
			c = openlibertyv1.OperationStatusCondition{
				Type:    openlibertyv1.OperationStatusConditionTypeFailed,
				Status:  corev1.ConditionTrue,
				Reason:  "ConnectionLost",
				Message: err.Error(),
			}
			instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
			c = openlibertyv1.OperationStatusCondition{
				Type:    openlibertyv1.OperationStatusConditionTypeFailed,
				Status:  corev1.ConditionTrue,
				Reason:  "ConnectionLost",
				Message: err.Error(),
			}
			instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
			instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
			instance.Status.Versions.Reconciled = utils.OperandVersion
			r.GetClient().Status().Update(context.TODO(), instance)
			return reconcile.Result{}, nil
		} else if injectorStatus == "idle..." {
			r.PodInjectorClient.StartScript("linperf", pod.Name, pod.Namespace, utils.EncodeLinperfAttr(instance))
		}

		var errMessage string
		var reason string
		if injectorStatus == "toomanyworkers..." {
			errMessage = "The operator performance data queue is full. Waiting for a worker to become available..."
			reason = "TooManyWorkers"
		} else {
			errMessage = utils.GetPerformanceDataWritingMessage(pod.Name)
			reason = "InProgress"
		}
		// requeue when waiting on performance data collection
		err = fmt.Errorf("%s", errMessage)
		reqLogger.Error(err, errMessage)
		r.GetRecorder().Event(instance, "Warning", "ProcessingError", err.Error())
		c = openlibertyv1.OperationStatusCondition{
			Type:    openlibertyv1.OperationStatusConditionTypeCompleted,
			Status:  corev1.ConditionFalse,
			Reason:  reason,
			Message: err.Error(),
		}
		instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
		instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
		instance.Status.Versions.Reconciled = utils.OperandVersion
		r.GetClient().Status().Update(context.TODO(), instance)
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
		}, nil
	}

	c = openlibertyv1.OperationStatusCondition{
		Type:   openlibertyv1.OperationStatusConditionTypeCompleted,
		Status: corev1.ConditionTrue,
	}

	performanceDataFile := ""
	fileNameOut := r.PodInjectorClient.PollLinperfFileName("linperf", pod.Name, pod.Namespace)
	if strings.HasPrefix(fileNameOut, "name:") {
		performanceDataFile = strings.TrimPrefix(fileNameOut, "name:")
		performanceDataFile = strings.TrimSuffix(performanceDataFile, "\n")
	}

	instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
	instance.Status.PerformanceDataFile = performanceDataFile
	instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
	instance.Status.Versions.Reconciled = utils.OperandVersion
	if err = r.GetClient().Status().Update(context.TODO(), instance); err == nil {
		// cleanup pod refs
		r.PodInjectorClient.CompleteScript("linperf", pod.Name, pod.Namespace)
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileOpenLibertyPerformanceData) finalizeOpenLibertyPerformanceData(reqLogger logr.Logger, olpd *openlibertyv1.OpenLibertyPerformanceData) error {
	if connErr := r.PodInjectorClient.Connect(); connErr != nil {
		return connErr
	}
	r.PodInjectorClient.CompleteScript("linperf", olpd.Spec.PodName, olpd.Namespace)
	r.PodInjectorClient.CloseConnection()
	return nil
}

func (r *ReconcileOpenLibertyPerformanceData) addFinalizer(reqLogger logr.Logger, olpd *openlibertyv1.OpenLibertyPerformanceData) error {
	reqLogger.Info("Adding Finalizer for OpenLibertyPerformanceData")
	olpd.SetFinalizers(append(olpd.GetFinalizers(), performanceDataFinalizer))

	// Update CR
	err := r.GetClient().Update(context.TODO(), olpd)
	if err != nil {
		reqLogger.Error(err, "Failed to update OpenLibertyPerformanceData with finalizer")
		return err
	}

	return nil
}

func isPerformanceDataRunning(instance *openlibertyv1.OpenLibertyPerformanceData) bool {
	isStarted := false
	isWorking := false
	isFailed := false
	for _, condition := range instance.Status.Conditions {
		if condition.Type == openlibertyv1.OperationStatusConditionTypeStarted && condition.Status == corev1.ConditionTrue {
			isStarted = true
		}
		if condition.Type == openlibertyv1.OperationStatusConditionTypeCompleted && condition.Status == corev1.ConditionFalse && condition.Reason == "InProgress" {
			isWorking = true
		}
		if condition.Type == openlibertyv1.OperationStatusConditionTypeFailed && condition.Status == corev1.ConditionTrue {
			isFailed = true
		}
	}
	return isStarted && isWorking && !isFailed
}

func (r *ReconcileOpenLibertyPerformanceData) SetupWithManager(mgr ctrl.Manager) error {

	watchNamespaces, err := oputils.GetWatchNamespaces()
	if err != nil {
		r.Log.Error(err, "Failed to get watch namespace")
		os.Exit(1)
	}

	watchNamespacesMap := make(map[string]bool)
	for _, ns := range watchNamespaces {
		watchNamespacesMap[ns] = true
	}
	isClusterWide := len(watchNamespacesMap) == 1 && watchNamespacesMap[""]

	r.Log.V(1).Info("Adding a new controller", "watchNamespaces", watchNamespaces, "isClusterWide", isClusterWide)

	pred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ignore updates to CR status in which case metadata.Generation does not change
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() && (isClusterWide || watchNamespacesMap[e.ObjectOld.GetNamespace()])
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
	return ctrl.NewControllerManagedBy(mgr).For(&openlibertyv1.OpenLibertyPerformanceData{}, builder.WithPredicates(pred)).WithOptions(controller.Options{
		MaxConcurrentReconciles: 1,
	}).Complete(r)

}
