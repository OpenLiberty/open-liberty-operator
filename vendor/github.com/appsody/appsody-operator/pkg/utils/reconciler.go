package utils

import (
	"context"
	"fmt"
	"math"
	"time"

	appsodyv1beta1 "github.com/appsody/appsody-operator/pkg/apis/appsody/v1beta1"
	"github.com/appsody/appsody-operator/pkg/common"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

// ReconcilerBase base reconciler with some common behaviour
type ReconcilerBase struct {
	client     client.Client
	scheme     *runtime.Scheme
	recorder   record.EventRecorder
	restConfig *rest.Config
	discovery  discovery.DiscoveryInterface
}

//NewReconcilerBase creates a new ReconcilerBase
func NewReconcilerBase(client client.Client, scheme *runtime.Scheme, restConfig *rest.Config, recorder record.EventRecorder) ReconcilerBase {
	return ReconcilerBase{
		client:     client,
		scheme:     scheme,
		recorder:   recorder,
		restConfig: restConfig,
	}
}

// GetClient returns client
func (r *ReconcilerBase) GetClient() client.Client {
	return r.client
}

// GetRecorder returns the underlying recorder
func (r *ReconcilerBase) GetRecorder() record.EventRecorder {
	return r.recorder
}

// GetDiscoveryClient ...
func (r *ReconcilerBase) GetDiscoveryClient() (discovery.DiscoveryInterface, error) {
	if r.discovery == nil {
		var err error
		r.discovery, err = discovery.NewDiscoveryClientForConfig(r.restConfig)
		return r.discovery, err
	}

	return r.discovery, nil
}

// SetDiscoveryClient ...
func (r *ReconcilerBase) SetDiscoveryClient(discovery discovery.DiscoveryInterface) {
	r.discovery = discovery
}

var log = logf.Log.WithName("utils")

// CreateOrUpdate ...
func (r *ReconcilerBase) CreateOrUpdate(obj metav1.Object, owner metav1.Object, reconcile func() error) error {

	mutate := func(o runtime.Object) error {
		err := reconcile()
		return err
	}

	controllerutil.SetControllerReference(owner, obj, r.scheme)
	runtimeObj, ok := obj.(runtime.Object)
	if !ok {
		err := fmt.Errorf("%T is not a runtime.Object", obj)
		log.Error(err, "Failed to convert into runtime.Object")
		return err
	}
	result, err := controllerutil.CreateOrUpdate(context.TODO(), r.GetClient(), runtimeObj, mutate)
	if err != nil {
		return err
	}

	var gvk schema.GroupVersionKind
	gvk, err = apiutil.GVKForObject(runtimeObj, r.scheme)
	if err == nil {
		log.Info("Reconciled", "Kind", gvk.Kind, "Name", obj.GetName(), "Status", result)
	}

	return err
}

// DeleteResource deletes kubernetes resource
func (r *ReconcilerBase) DeleteResource(obj runtime.Object) error {
	err := r.client.Delete(context.TODO(), obj)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Unable to delete object ", "object", obj)
			return err
		}
		return nil
	}

	metaObj, ok := obj.(metav1.Object)
	if !ok {
		err := fmt.Errorf("%T is not a runtime.Object", obj)
		log.Error(err, "Failed to convert into runtime.Object")
		return err
	}

	var gvk schema.GroupVersionKind
	gvk, err = apiutil.GVKForObject(obj, r.scheme)
	if err == nil {
		log.Info("Reconciled", "Kind", gvk.Kind, "Name", metaObj.GetName(), "Status", "deleted")
	}
	return nil
}

// DeleteResources ...
func (r *ReconcilerBase) DeleteResources(resources []runtime.Object) error {
	for i := range resources {
		err := r.DeleteResource(resources[i])
		if err != nil {
			return err
		}
	}
	return nil
}

// GetAppsodyOpConfigMap ...
func (r *ReconcilerBase) GetAppsodyOpConfigMap(name string, ns string) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{}
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: name, Namespace: ns}, configMap)
	if err != nil {
		return nil, err
	}
	return configMap, nil
}

// ManageError ...
func (r *ReconcilerBase) ManageError(issue error, conditionType common.StatusConditionType, ba common.BaseApplication) (reconcile.Result, error) {

	s := ba.GetStatus()
	rObj := ba.(runtime.Object)
	r.GetRecorder().Event(rObj, "Warning", "ProcessingError", issue.Error())

	oldCondition := s.GetCondition(conditionType)
	if oldCondition == nil {
		oldCondition = &appsodyv1beta1.StatusCondition{LastUpdateTime: metav1.Time{}}
	}

	lastUpdate := oldCondition.GetLastUpdateTime().Time
	lastStatus := oldCondition.GetStatus()

	// Keep the old `LastTransitionTime` when status has not changed
	nowTime := metav1.Now()
	transitionTime := oldCondition.GetLastTransitionTime()
	if lastStatus == corev1.ConditionTrue {
		transitionTime = &nowTime
	}

	newCondition := s.NewCondition()
	newCondition.SetLastTransitionTime(transitionTime)
	newCondition.SetLastUpdateTime(nowTime)
	newCondition.SetReason(string(apierrors.ReasonForError(issue)))
	newCondition.SetType(conditionType)
	newCondition.SetMessage(issue.Error())
	newCondition.SetStatus(corev1.ConditionFalse)

	s.SetCondition(newCondition)

	err := r.GetClient().Status().Update(context.Background(), rObj)
	if err != nil {
		log.Error(err, "Unable to update status")
		return reconcile.Result{
			RequeueAfter: time.Second,
			Requeue:      true,
		}, nil
	}

	// StatusReasonInvalid means the requested create or update operation cannot be
	// completed due to invalid data provided as part of the request. Don't retry.
	if apierrors.IsInvalid(issue) {
		return reconcile.Result{}, nil
	}

	var retryInterval time.Duration
	if lastUpdate.IsZero() || lastStatus == corev1.ConditionTrue {
		retryInterval = time.Second
	} else {
		retryInterval = newCondition.GetLastUpdateTime().Sub(lastUpdate).Round(time.Second)
	}

	return reconcile.Result{
		RequeueAfter: time.Duration(math.Min(float64(retryInterval.Nanoseconds()*2), float64(time.Hour.Nanoseconds()*6))),
		Requeue:      true,
	}, nil
}

// ManageSuccess ...
func (r *ReconcilerBase) ManageSuccess(conditionType common.StatusConditionType, ba common.BaseApplication) (reconcile.Result, error) {
	s := ba.GetStatus()
	oldCondition := s.GetCondition(conditionType)
	if oldCondition == nil {
		oldCondition = &appsodyv1beta1.StatusCondition{LastUpdateTime: metav1.Time{}}
	}

	// Keep the old `LastTransitionTime` when status has not changed
	nowTime := metav1.Now()
	transitionTime := oldCondition.GetLastTransitionTime()
	if oldCondition.GetStatus() == corev1.ConditionFalse {
		transitionTime = &nowTime
	}

	statusCondition := s.NewCondition()
	statusCondition.SetLastTransitionTime(transitionTime)
	statusCondition.SetLastUpdateTime(nowTime)
	statusCondition.SetReason("")
	statusCondition.SetMessage("")
	statusCondition.SetStatus(corev1.ConditionTrue)

	s.SetCondition(statusCondition)
	rObj := ba.(runtime.Object)
	err := r.GetClient().Status().Update(context.Background(), rObj)
	if err != nil {
		log.Error(err, "Unable to update status")
		return reconcile.Result{
			RequeueAfter: time.Second,
			Requeue:      true,
		}, nil
	}
	return reconcile.Result{}, nil
}

// IsGroupVersionSupported ...
func (r *ReconcilerBase) IsGroupVersionSupported(groupVersion string) (bool, error) {
	cli, err := r.GetDiscoveryClient()
	if err != nil {
		log.Error(err, "Failed to return a discovery client for the current reconciler")
		return false, err
	}

	_, err = cli.ServerResourcesForGroupVersion(groupVersion)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}
