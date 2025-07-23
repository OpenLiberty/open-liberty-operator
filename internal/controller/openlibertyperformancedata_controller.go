package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/OpenLiberty/open-liberty-operator/utils"
	oputils "github.com/application-stacks/runtime-component-operator/utils"
	"github.com/go-logr/logr"

	openlibertyv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ReconcileOpenLibertyPerformanceData reconciles an OpenLibertyPerformanceData object
type ReconcileOpenLibertyPerformanceData struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	Client            client.Client
	Scheme            *runtime.Scheme
	Recorder          record.EventRecorder
	RestConfig        *rest.Config
	Log               logr.Logger
	PodInjectorClient utils.PodInjectorClient
}

// +kubebuilder:rbac:groups=apps.openliberty.io,resources=openlibertyperformancedatas;openlibertyperformancedatas/status;openlibertyperformancedatas/finalizers,verbs=get;list;watch;create;update;patch;delete,namespace=open-liberty-operator
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
	err := r.Client.Get(context.TODO(), request.NamespacedName, instance)
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

	//do not reconcile if performance data collection already completed
	oc := openlibertyv1.GetOperationCondtion(instance.Status.Conditions, openlibertyv1.OperationStatusConditionTypeCompleted)
	if oc != nil && oc.Status == corev1.ConditionTrue {
		return reconcile.Result{}, err
	}

	//check if Pod exists and running
	pod := &corev1.Pod{}

	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.PodName, Namespace: request.Namespace}, pod)
	if err != nil || pod.Status.Phase != corev1.PodRunning {
		message := fmt.Sprintf("Failed to find pod %s in namespace %s", instance.Spec.PodName, request.Namespace)
		reqLogger.Error(err, message)
		r.Recorder.Event(instance, "Warning", "ProcessingError", message)
		c := openlibertyv1.OperationStatusCondition{
			Type:    openlibertyv1.OperationStatusConditionTypeStarted,
			Status:  corev1.ConditionFalse,
			Reason:  "Error",
			Message: "Failed to find a pod or pod is not in running state",
		}
		instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
		instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
		instance.Status.Versions.Reconciled = utils.OperandVersion
		r.Client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, nil
	}

	c := openlibertyv1.OperationStatusCondition{
		Type:   openlibertyv1.OperationStatusConditionTypeStarted,
		Status: corev1.ConditionTrue,
	}

	instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
	r.Client.Status().Update(context.TODO(), instance)

	operatorNamespace, err := oputils.GetOperatorNamespace()
	if err != nil {
		// for local dev fallback to the watch namespace
		watchNamespace, err := oputils.GetWatchNamespace()
		if err != nil {
			reqLogger.Error(err, "Failed to get watch namespace")
			return reconcile.Result{}, nil
		}
		operatorNamespace = watchNamespace
	}

	// allow operator traffic to the Pod
	networkPolicy := &networkingv1.NetworkPolicy{}
	networkPolicy.Name = OperatorShortName + "-managed-linperf-" + instance.Spec.PodName
	networkPolicy.Namespace = request.Namespace
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: networkPolicy.Name, Namespace: networkPolicy.Namespace}, networkPolicy)
	if err != nil {
		if kerrors.IsNotFound(err) {
			networkPolicy.Spec.PolicyTypes = append(networkPolicy.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
			networkPolicy.Spec.PodSelector = metav1.LabelSelector{
				MatchLabels: pod.Labels,
			}
			networkPolicy.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{},
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: utils.GetOperatorLabels(),
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": operatorNamespace,
								},
							},
						},
					},
				},
			}
			err = r.Client.Create(context.TODO(), networkPolicy)
			if err != nil {
				reqLogger.Error(err, "Failed to create performance data network policy")
				return reconcile.Result{
					Requeue:      true,
					RequeueAfter: 5 * time.Second,
				}, nil
			}
		}
		reqLogger.Error(err, "Failed to get performance data network policy")
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
		}, nil
	}

	if r.PodInjectorClient.Connect() != nil {
		message := fmt.Sprintf("Failed to connect to the operator pod injector")
		reqLogger.Error(err, message)
		r.Recorder.Event(instance, "Warning", "ProcessingError", message)
		c := openlibertyv1.OperationStatusCondition{
			Type:    openlibertyv1.OperationStatusConditionTypeStarted,
			Status:  corev1.ConditionFalse,
			Reason:  "Error",
			Message: "Failed to connect to the operator pod injector",
		}
		instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
		instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
		instance.Status.Versions.Reconciled = utils.OperandVersion
		r.Client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, nil
	}
	defer r.PodInjectorClient.CloseConnection()

	injectorStatus := r.PodInjectorClient.PollStatus("linperf", pod.Name, pod.Namespace)
	if injectorStatus != "done..." {
		if injectorStatus == "idle..." {
			r.PodInjectorClient.StartScript("linperf", pod.Name, pod.Namespace, utils.EncodeLinperfAttr(instance))
		}
		// requeue and set status that the operator is waiting
		errMessage := fmt.Sprintf("Collecting performance data for Pod '%s'...", pod.Name)
		err = fmt.Errorf("%s", errMessage)
		reqLogger.Error(err, errMessage)
		r.Recorder.Event(instance, "Warning", "ProcessingError", err.Error())
		c = openlibertyv1.OperationStatusCondition{
			Type:    openlibertyv1.OperationStatusConditionTypeCompleted,
			Status:  corev1.ConditionFalse,
			Reason:  "Error",
			Message: err.Error(),
		}
		instance.Status.Conditions = openlibertyv1.SetOperationCondtion(instance.Status.Conditions, c)
		instance.Status.ObservedGeneration = instance.GetObjectMeta().GetGeneration()
		instance.Status.Versions.Reconciled = utils.OperandVersion
		r.Client.Status().Update(context.TODO(), instance)
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
	r.Client.Status().Update(context.TODO(), instance)

	if err = r.Client.Delete(context.TODO(), networkPolicy); err != nil {
		reqLogger.Error(err, "Failed to delete performance data network policy")
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
		}, err
	}
	return reconcile.Result{}, nil
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
