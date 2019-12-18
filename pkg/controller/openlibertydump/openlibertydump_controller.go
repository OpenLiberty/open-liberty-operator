package openlibertydump

import (
	"context"
	"github.com/OpenLiberty/open-liberty-operator/pkg/utils"
	"os"
	"time"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	autils "github.com/appsody/appsody-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_openlibertydump")

// Add creates a new OpenLibertyDump Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileOpenLibertyDump{client: mgr.GetClient(), scheme: mgr.GetScheme(), restConfig: mgr.GetConfig()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("openlibertydump-controller", mgr, controller.Options{Reconciler: r})
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

	// Watch for changes to primary resource OpenLibertyDump
	err = c.Watch(&source.Kind{Type: &openlibertyv1beta1.OpenLibertyDump{}}, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileOpenLibertyDump implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileOpenLibertyDump{}

// ReconcileOpenLibertyDump reconciles a OpenLibertyDump object
type ReconcileOpenLibertyDump struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client     client.Client
	scheme     *runtime.Scheme
	restConfig *rest.Config
}

// Reconcile reads that state of the cluster for a OpenLibertyDump object and makes changes based on the state read
// and what is in the OpenLibertyDump.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileOpenLibertyDump) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling OpenLibertyDump")

	// Fetch the OpenLibertyDump instance
	instance := &openlibertyv1beta1.OpenLibertyDump{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
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

	//do not reconcile if the dump already started
	oc := openlibertyv1beta1.GetOperationCondtion(instance.Status.Conditions, openlibertyv1beta1.OperationStatusConditionTypeStarted)
	if oc != nil && oc.Status == corev1.ConditionTrue {
		return reconcile.Result{}, err
	}

	//check if Pod exists and running
	pod := &corev1.Pod{}

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.PodName, Namespace: request.Namespace}, pod)
	if err != nil || pod.Status.Phase != corev1.PodRunning {
		//handle error
		log.Error(err, "Failed to find pod")
		c := openlibertyv1beta1.OperationStatusCondition{
			Type:    openlibertyv1beta1.OperationStatusConditionTypeStarted,
			Status:  corev1.ConditionFalse,
			Reason:  "Error",
			Message: "Failed to find a pod or pod is not in running state",
		}
		instance.Status.Conditions = openlibertyv1beta1.SetOperationCondtion(instance.Status.Conditions, c)
		r.client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, nil
	}

	time := time.Now()
	dumpFolder := "/serviceability/" + pod.Namespace + "/" + pod.Name
	dumpFileName := dumpFolder + "/" + time.Format("2006-01-02_15:04:05") + ".zip"
	dumpCmd := "mkdir -p " + dumpFolder + ";  server dump --archive=" + dumpFileName
	if len(instance.Spec.Include) > 0 {
		dumpCmd += " --include="
		for i := range instance.Spec.Include {
			dumpCmd += instance.Spec.Include[i] + ","
		}
	}

	c := openlibertyv1beta1.OperationStatusCondition{
		Type:   openlibertyv1beta1.OperationStatusConditionTypeStarted,
		Status: corev1.ConditionTrue,
	}

	instance.Status.Conditions = openlibertyv1beta1.SetOperationCondtion(instance.Status.Conditions, c)
	r.client.Status().Update(context.TODO(), instance)

	_, err = utils.ExecuteCommandInContainer(r.restConfig, pod.Name, pod.Namespace, "app", []string{"/bin/sh", "-c", dumpCmd})
	if err != nil {
		//handle error
		log.Error(err, "Execute dump cmd failed ", dumpCmd)
		c = openlibertyv1beta1.OperationStatusCondition{
			Type:    openlibertyv1beta1.OperationStatusConditionTypeCompleted,
			Status:  corev1.ConditionFalse,
			Reason:  "Error",
			Message: err.Error(),
		}
		instance.Status.Conditions = openlibertyv1beta1.SetOperationCondtion(instance.Status.Conditions, c)
		r.client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, nil

	}

	c = openlibertyv1beta1.OperationStatusCondition{
		Type:   openlibertyv1beta1.OperationStatusConditionTypeCompleted,
		Status: corev1.ConditionTrue,
	}

	instance.Status.Conditions = openlibertyv1beta1.SetOperationCondtion(instance.Status.Conditions, c)
	instance.Status.DumpFile = dumpFileName
	r.client.Status().Update(context.TODO(), instance)
	return reconcile.Result{}, nil
}
