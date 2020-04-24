package openliberty

import (
	"context"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	appstacksutils "github.com/application-stacks/runtime-component-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = &EnqueueRequestsForCustomIndexField{}

const (
	indexFieldImageStreamName     = "spec.applicationImage"
	indexFieldBindingsResourceRef = "spec.bindings.resourceRef"
)

// EnqueueRequestsForCustomIndexField enqueues reconcile Requests Open Liberty Applications if the app is relying on
// the modified resource
type EnqueueRequestsForCustomIndexField struct {
	handler.Funcs
	Matcher func(metav1.Object) ([]openlibertyv1beta1.OpenLibertyApplication, error)
}

// Update implements EventHandler
func (e *EnqueueRequestsForCustomIndexField) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	e.handle(evt.MetaNew, evt.ObjectNew, q)
}

// Delete implements EventHandler
func (e *EnqueueRequestsForCustomIndexField) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	e.handle(evt.Meta, evt.Object, q)
}

// Generic implements EventHandler
func (e *EnqueueRequestsForCustomIndexField) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	e.handle(evt.Meta, evt.Object, q)
}

// handle common implementation to enqueue reconcile Requests for applications
func (e *EnqueueRequestsForCustomIndexField) handle(evtMeta metav1.Object, evtObj runtime.Object, q workqueue.RateLimitingInterface) {
	apps, _ := e.Matcher(evtMeta)
	for _, app := range apps {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: app.Namespace,
				Name:      app.Name,
			}})
	}
}

// CreateImageStreamMatcher return a func that matches all applications using the input ImageStreamTag
func CreateImageStreamMatcher(clnt client.Client, watchNamespaces []string) func(metav1.Object) ([]openlibertyv1beta1.OpenLibertyApplication, error) {
	matcher := func(imageStreamTag metav1.Object) ([]openlibertyv1beta1.OpenLibertyApplication, error) {
		apps := []openlibertyv1beta1.OpenLibertyApplication{}
		var namespaces []string
		if appstacksutils.IsClusterWide(watchNamespaces) {
			nsList := &corev1.NamespaceList{}
			if err := clnt.List(context.Background(), nsList, client.InNamespace("")); err != nil {
				return nil, err
			}
			for _, ns := range nsList.Items {
				namespaces = append(namespaces, ns.Name)
			}
		} else {
			namespaces = watchNamespaces
		}
		for _, ns := range namespaces {
			appList := &openlibertyv1beta1.OpenLibertyApplicationList{}
			err := clnt.List(context.Background(),
				appList,
				client.InNamespace(ns),
				client.MatchingFields{indexFieldImageStreamName: imageStreamTag.GetNamespace() + "/" + imageStreamTag.GetName()})
			if err != nil {
				return nil, err
			}
			apps = append(apps, appList.Items...)
		}
		return apps, nil
	}
	return matcher
}

//CreateBindingSecretMatcher return a func that matches all applications that "could" rely on the secret as a secret binding
func CreateBindingSecretMatcher(clnt client.Client) func(metav1.Object) ([]openlibertyv1beta1.OpenLibertyApplication, error) {
	matcher := func(secret metav1.Object) ([]openlibertyv1beta1.OpenLibertyApplication, error) {
		apps := []openlibertyv1beta1.OpenLibertyApplication{}

		// Adding apps which have this secret defined in the spec.bindings.resourceRef
		appList := &openlibertyv1beta1.OpenLibertyApplicationList{}
		err := clnt.List(context.Background(),
			appList,
			client.InNamespace(secret.GetNamespace()),
			client.MatchingFields{indexFieldBindingsResourceRef: secret.GetName()})
		if err != nil {
			return nil, err
		}
		apps = append(apps, appList.Items...)

		// If we are able to find an app with the secret name, add the app. This is to cover the autoDetect scenario
		app := &openlibertyv1beta1.OpenLibertyApplication{}
		err = clnt.Get(context.Background(), types.NamespacedName{Name: secret.GetName(), Namespace: secret.GetNamespace()}, app)
		if err != nil {
			if !kerrors.IsNotFound(err) {
				return nil, err
			}
		}
		apps = append(apps, *app)
		return apps, nil
	}
	return matcher
}
