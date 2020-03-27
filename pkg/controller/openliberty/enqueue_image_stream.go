package openliberty

import (
	"context"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	appstacksutils "github.com/application-stacks/runtime-component-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = &EnqueueRequestsForImageStream{}

const (
	indexFieldImageStreamName = "spec.applicationImage"
)

// EnqueueRequestsForImageStream enqueues reconcile Requests Runtime Components if the app is relying on
// the image stream
type EnqueueRequestsForImageStream struct {
	handler.Funcs
	WatchNamespaces []string
	Client          client.Client
}

// Update implements EventHandler
func (e *EnqueueRequestsForImageStream) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	e.handle(evt.MetaNew, q)
}

// Delete implements EventHandler
func (e *EnqueueRequestsForImageStream) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	e.handle(evt.Meta, q)
}

// Generic implements EventHandler
func (e *EnqueueRequestsForImageStream) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	e.handle(evt.Meta, q)
}

// handle common implementation to enqueue reconcile Requests for applications
func (e *EnqueueRequestsForImageStream) handle(evtMeta metav1.Object, q workqueue.RateLimitingInterface) {
	apps, _ := e.matchApplication(evtMeta)
	for _, app := range apps {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: app.Namespace,
				Name:      app.Name,
			}})
	}
}

// matchApplication returns the NamespacedName of all applications using the input ImageStreamTag
func (e *EnqueueRequestsForImageStream) matchApplication(imageStreamTag metav1.Object) ([]openlibertyv1beta1.OpenLibertyApplication, error) {
	apps := []openlibertyv1beta1.OpenLibertyApplication{}
	var namespaces []string
	if appstacksutils.IsClusterWide(e.WatchNamespaces) {
		nsList := &corev1.NamespaceList{}
		if err := e.Client.List(context.Background(), nsList, client.InNamespace("")); err != nil {
			return nil, err
		}
		for _, ns := range nsList.Items {
			namespaces = append(namespaces, ns.Name)
		}
	} else {
		namespaces = e.WatchNamespaces
	}
	for _, ns := range namespaces {
		appList := &openlibertyv1beta1.OpenLibertyApplicationList{}
		err := e.Client.List(context.Background(),
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
