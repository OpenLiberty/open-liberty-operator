package controller

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	tree "github.com/OpenLiberty/open-liberty-operator/utils/tree"
	oputils "github.com/application-stacks/runtime-component-operator/utils"
	"github.com/go-logr/logr"

	openlibertyv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ReconcileOpenLibertyTrace reconciles an OpenLibertyTrace object
type ReconcileOpenLibertyTrace struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	Client     client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	RestConfig *rest.Config
	Log        logr.Logger
}

const traceFinalizer = "finalizer.openlibertytraces.apps.openliberty.io"
const traceConfigFile = "/config/configDropins/overrides/add_trace.xml"
const serviceabilityDir = "/serviceability"

// +kubebuilder:rbac:groups=apps.openliberty.io,resources=openlibertytraces;openlibertytraces/status;openlibertytraces/finalizers,verbs=get;list;watch;create;update;patch;delete,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=core,resources=pods;pods/exec,verbs=get;list;watch;create;update;patch;delete,namespace=open-liberty-operator
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete,namespace=open-liberty-operator

// Reconcile reads that state of the cluster for an OpenLibertyTrace object and makes changes based on the state read
// and what is in the OpenLibertyTrace.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.

func (r *ReconcileOpenLibertyTrace) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Namespace", request.Namespace, "Name", request.Name)
	reqLogger.Info("Reconciling OpenLibertyTrace")

	// Fetch the OpenLibertyTrace instance
	instance := &openlibertyv1.OpenLibertyTrace{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, instance)
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

	instance.Initialize()

	// Reconciles the shared Trace state for the instance namespace
	var traceMetadataList *lutils.TraceMetadataList
	var traceMetadata, prevPodTraceMetadata *lutils.TraceMetadata
	leaderMetadataList, err := r.reconcileResourceTrackingState(instance, TRACE_RESOURCE_SHARING_FILE_NAME)

	if err != nil {
		return reconcile.Result{}, err
	}
	traceMetadataList = leaderMetadataList.(*lutils.TraceMetadataList)
	if traceMetadataList != nil {
		numTraceItems := len(traceMetadataList.Items)
		if numTraceItems >= 1 {
			traceMetadata = traceMetadataList.Items[0].(*lutils.TraceMetadata)
		}
		if numTraceItems >= 2 {
			prevPodTraceMetadata = traceMetadataList.Items[1].(*lutils.TraceMetadata)
		}

	}

	leaderName, thisInstanceIsLeader, _, err := tree.ReconcileLeader(r.GetClient(), func(obj client.Object, owner metav1.Object, cb func() error) error {
		return r.CreateOrUpdate(obj, owner, cb)
	}, OperatorShortName, instance.GetName(), instance.GetNamespace(), traceMetadata, TRACE_RESOURCE_SHARING_FILE_NAME, true, true)
	if err != nil && !kerrors.IsNotFound(err) {
		return reconcile.Result{Requeue: true, RequeueAfter: time.Second}, err
	}

	//Pod is expected to be from the same namespace as the CR instance
	podNamespace := instance.Namespace
	podName := instance.Spec.PodName

	if !thisInstanceIsLeader {
		err := fmt.Errorf("Trace could not be applied. Pod '" + podName + "' is already configured by OpenLibertyTrace instance '" + leaderName + "'.")
		reqLogger.Error(err, "Trace was denied for instance '"+instance.GetName()+"'; Trace instance '"+leaderName+"' is already managing pod '"+podName+"' in namespace '"+podNamespace+"'")
		return r.UpdateStatus(err, openlibertyv1.OperationStatusConditionTypeEnabled, *instance, corev1.ConditionFalse, podName, false)
	}

	prevPodName := instance.GetStatus().GetOperatedResource().GetOperatedResourceName()
	prevTraceEnabled := instance.GetStatus().GetCondition(openlibertyv1.OperationStatusConditionTypeEnabled).Status
	podChanged := prevPodName != podName

	oldPodLeaderName := ""
	if prevPodTraceMetadata != nil {
		prevPodLeaderName, _, _, err := tree.ReconcileLeader(r.GetClient(), func(obj client.Object, owner metav1.Object, cb func() error) error {
			return r.CreateOrUpdate(obj, owner, cb)
		}, OperatorShortName, instance.GetName(), instance.GetNamespace(), prevPodTraceMetadata, TRACE_RESOURCE_SHARING_FILE_NAME, false, true)
		if err != nil && !kerrors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		if prevPodLeaderName != leaderName {
			oldPodLeaderName = prevPodLeaderName
		}
	}

	// Check if the OpenLibertyTrace instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isInstanceMarkedToBeDeleted := instance.GetDeletionTimestamp() != nil
	if isInstanceMarkedToBeDeleted {
		if lutils.Contains(instance.GetFinalizers(), traceFinalizer) {
			// Run finalization logic for traceFinalizer. If the finalization logic fails, don't remove the
			// finalizer so that we can retry during the next reconciliation.
			if err := r.finalizeOpenLibertyTrace(reqLogger, instance, prevTraceEnabled, prevPodName, podNamespace, oldPodLeaderName); err != nil {
				return reconcile.Result{}, err
			}

			// Remove traceFinalizer. Once all finalizers have been removed, the object will be deleted.
			instance.SetFinalizers(lutils.Remove(instance.GetFinalizers(), traceFinalizer))
			err := r.Client.Update(context.TODO(), instance)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// Add finalizer for this CR
	if !lutils.Contains(instance.GetFinalizers(), traceFinalizer) {
		if err := r.addFinalizer(reqLogger, instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	//If pod name changed, then stop tracing on previous pod (if trace was enabled on it) i.f.f. prevPod is not being managed by another OpenLibertyTrace leader instance
	if oldPodLeaderName == "" {
		if podChanged && (prevTraceEnabled == corev1.ConditionTrue) {
			r.disableTraceOnPrevPod(reqLogger, prevPodName, podNamespace)
		}
	}

	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: podName, Namespace: podNamespace}, &corev1.Pod{})
	if err != nil && errors.IsNotFound(err) {
		//Pod is not found. Return and don't requeue
		reqLogger.Error(err, "Pod "+podName+" was not found in namespace "+podNamespace)
		return r.UpdateStatus(err, openlibertyv1.OperationStatusConditionTypeEnabled, *instance, corev1.ConditionFalse, podName, podChanged)
	}

	if instance.Spec.Disable != nil && *instance.Spec.Disable {
		//Disable trace if trace was previously enabled on the same pod
		if !podChanged && prevTraceEnabled == corev1.ConditionTrue {
			_, err = lutils.ExecuteCommandInContainer(r.RestConfig, podName, podNamespace, "app", []string{"/bin/sh", "-c", "rm -f " + traceConfigFile})
			if err != nil {
				reqLogger.Error(err, "Encountered error while disabling trace for pod "+podName+" in namespace "+podNamespace)
				return r.UpdateStatus(err, openlibertyv1.OperationStatusConditionTypeEnabled, *instance, corev1.ConditionTrue, podName, podChanged)
			}
			reqLogger.Info("Disabled trace for pod " + podName + " in namespace " + podNamespace)
		}
		r.UpdateStatus(nil, openlibertyv1.OperationStatusConditionTypeEnabled, *instance, corev1.ConditionFalse, podName, podChanged)
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

		_, err = lutils.ExecuteCommandInContainer(r.RestConfig, podName, podNamespace, "app", []string{"/bin/sh", "-c", "mkdir -p " + traceOutputDir + " && echo '" + traceConfig + "' > " + traceConfigFile})
		if err != nil {
			reqLogger.Error(err, "Encountered error while setting up trace for pod "+podName+" in namespace "+podNamespace)
			return r.UpdateStatus(err, openlibertyv1.OperationStatusConditionTypeEnabled, *instance, corev1.ConditionFalse, podName, podChanged)
		}

		if podChanged || prevTraceEnabled == corev1.ConditionFalse {
			reqLogger.Info("Enabled trace for pod " + podName + " in namespace " + podNamespace)
		} else {
			reqLogger.Info("Updated trace for pod " + podName + " in namespace " + podNamespace)
		}
		r.UpdateStatus(nil, openlibertyv1.OperationStatusConditionTypeEnabled, *instance, corev1.ConditionTrue, podName, podChanged)
	}

	return reconcile.Result{}, nil
}

// UpdateStatus updates the status
func (r *ReconcileOpenLibertyTrace) UpdateStatus(issue error, conditionType openlibertyv1.OperationStatusConditionType, instance openlibertyv1.OpenLibertyTrace, newStatus corev1.ConditionStatus, podName string, podChanged bool) (reconcile.Result, error) {
	s := instance.GetStatus()

	s.SetOperatedResource(openlibertyv1.OperatedResource{ResourceName: podName, ResourceType: "pod"})

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
		r.Recorder.Event(&instance, "Warning", "ProcessingError", issue.Error())
	} else {
		statusCondition.SetReason("")
		statusCondition.SetMessage("")
	}

	statusCondition.SetStatus(newStatus)
	statusCondition.SetType(conditionType)

	s.SetCondition(statusCondition)

	instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
	instance.Status.Versions.Reconciled = lutils.OperandVersion

	err := r.Client.Status().Update(context.Background(), &instance)
	if err != nil {
		r.Log.Error(err, "Unable to update status")
		return reconcile.Result{
			RequeueAfter: time.Second,
			Requeue:      true,
		}, nil
	}

	return reconcile.Result{Requeue: false}, nil
}

func (r *ReconcileOpenLibertyTrace) disableTraceOnPrevPod(reqLogger logr.Logger, prevPodName string, podNamespace string) {
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: prevPodName, Namespace: podNamespace}, &corev1.Pod{})
	if err != nil && errors.IsNotFound(err) {
		//Previous Pod is not found. No-op
		reqLogger.Info("Previous pod " + prevPodName + " was not found in namespace " + podNamespace)
	} else {
		//Stop tracing on previous Pod
		_, err = lutils.ExecuteCommandInContainer(r.RestConfig, prevPodName, podNamespace, "app", []string{"/bin/sh", "-c", "rm -f " + traceConfigFile})
		if err == nil {
			reqLogger.Info("Disabled trace on previous pod " + prevPodName + " in namespace " + podNamespace)
		} else {
			reqLogger.Error(err, "Encountered error while disabling trace on previous pod "+prevPodName+" in namespace "+podNamespace)
		}
	}
}

// CreateOrUpdate ...
func (r *ReconcileOpenLibertyTrace) CreateOrUpdate(obj client.Object, owner metav1.Object, reconcile func() error) error {

	if owner != nil {
		controllerutil.SetControllerReference(owner, obj, r.Scheme)
	}

	result, err := controllerutil.CreateOrUpdate(context.TODO(), r.GetClient(), obj, reconcile)
	if err != nil {
		return err
	}

	var gvk schema.GroupVersionKind
	gvk, err = apiutil.GVKForObject(obj, r.Scheme)
	if err == nil {
		r.Log.Info("Reconciled", "Kind", gvk.Kind, "Namespace", obj.GetNamespace(), "Name", obj.GetName(), "Status", result)
	}

	return err
}

// DeleteResource deletes kubernetes resource
func (r *ReconcileOpenLibertyTrace) DeleteResource(obj client.Object) error {
	err := r.GetClient().Delete(context.TODO(), obj)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			//log.Error(err, "Unable to delete object ", "object", obj)
			return err
		}
		return nil
	}

	_, ok := obj.(metav1.Object)
	if !ok {
		err := fmt.Errorf("%T is not a metav1.Object", obj)
		//log.Error(err, "Failed to convert into metav1.Object")
		return err
	}

	_, err = apiutil.GVKForObject(obj, r.Scheme)
	if err == nil {
		//log.Info("Reconciled", "Kind", gvk.Kind, "Name", owner.GetName(), "Status", "deleted")
	}
	return nil
}

// GetClient returns client
func (r *ReconcileOpenLibertyTrace) GetClient() client.Client {
	return r.Client
}

func (r *ReconcileOpenLibertyTrace) finalizeOpenLibertyTrace(reqLogger logr.Logger, olt *openlibertyv1.OpenLibertyTrace, prevTraceEnabled corev1.ConditionStatus, prevPodName string, podNamespace string, oldPodLeaderName string) error {
	// only disable trace on prevPod if it is not being managed by another OpenLibertyTrace leader instance
	if oldPodLeaderName == "" && prevTraceEnabled == corev1.ConditionTrue {
		r.disableTraceOnPrevPod(reqLogger, prevPodName, podNamespace)
	}
	tree.RemoveLeaderTrackerReference(r.GetClient(),
		func(obj client.Object, owner metav1.Object, cb func() error) error {
			return r.CreateOrUpdate(obj, owner, cb)
		},
		func(obj client.Object) error {
			return r.DeleteResource(obj)
		}, olt.GetName(), olt.GetNamespace(), OperatorShortName, TRACE_RESOURCE_SHARING_FILE_NAME)
	return nil
}

func (r *ReconcileOpenLibertyTrace) addFinalizer(reqLogger logr.Logger, olt *openlibertyv1.OpenLibertyTrace) error {
	reqLogger.Info("Adding Finalizer for OpenLibertyTrace")
	olt.SetFinalizers(append(olt.GetFinalizers(), traceFinalizer))

	// Update CR
	err := r.Client.Update(context.TODO(), olt)
	if err != nil {
		reqLogger.Error(err, "Failed to update OpenLibertyTrace with finalizer")
		return err
	}

	return nil
}

func (r *ReconcileOpenLibertyTrace) SetupWithManager(mgr ctrl.Manager) error {

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
	return ctrl.NewControllerManagedBy(mgr).For(&openlibertyv1.OpenLibertyTrace{}, builder.WithPredicates(pred)).WithOptions(controller.Options{
		MaxConcurrentReconciles: 1,
	}).Complete(r)
}
