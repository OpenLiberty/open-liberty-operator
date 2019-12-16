package openlibertydump

import (
	"context"
	"time"

	"github.com/OpenLiberty/open-liberty-operator/pkg/utils"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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

	// Watch for changes to primary resource OpenLibertyDump
	err = c.Watch(&source.Kind{Type: &openlibertyv1beta1.OpenLibertyDump{}}, &handler.EnqueueRequestForObject{})
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
	dumpCmd := "mkdir -p " + dumpFolder + ";  server dump --archive=" + dumpFolder + "/" + time.Format("2006-01-02_15:04:05") + ".zip"
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
	r.client.Status().Update(context.TODO(), instance)
	return reconcile.Result{}, nil
}
