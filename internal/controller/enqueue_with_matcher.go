package controller

import (
	"context"

	openlibertyv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"

	appstacksutils "github.com/application-stacks/runtime-component-operator/utils"
	corev1 "k8s.io/api/core/v1"
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
	indexFieldImageStreamName = "spec.applicationImage"
)

// EnqueueRequestsForCustomIndexField enqueues reconcile Requests for OpenLiberty Applications if the app is relying on
// the modified resource
type EnqueueRequestsForCustomIndexField struct {
	handler.Funcs
	Matcher CustomMatcher
}

// Create implements EventHandler
func (e *EnqueueRequestsForCustomIndexField) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	e.handle(evt.Object, evt.Object, q)
}

// Update implements EventHandler
func (e *EnqueueRequestsForCustomIndexField) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	e.handle(evt.ObjectNew, evt.ObjectNew, q)
}

// Delete implements EventHandler
func (e *EnqueueRequestsForCustomIndexField) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	e.handle(evt.Object, evt.Object, q)
}

// Generic implements EventHandler
func (e *EnqueueRequestsForCustomIndexField) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	e.handle(evt.Object, evt.Object, q)
}

// handle common implementation to enqueue reconcile Requests for applications
func (e *EnqueueRequestsForCustomIndexField) handle(evtMeta metav1.Object, evtObj runtime.Object, q workqueue.RateLimitingInterface) {
	apps, _ := e.Matcher.Match(evtMeta)
	for _, app := range apps {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: app.Namespace,
				Name:      app.Name,
			}})
	}
}

// CustomMatcher is an interface for matching apps that satisfy a custom logic
type CustomMatcher interface {
	Match(metav1.Object) ([]openlibertyv1.OpenLibertyApplication, error)
}

// ImageStreamMatcher implements CustomMatcher for Image Streams
type ImageStreamMatcher struct {
	Klient          client.Client
	WatchNamespaces []string
}

// Match returns all applications using the input ImageStreamTag
func (i *ImageStreamMatcher) Match(imageStreamTag metav1.Object) ([]openlibertyv1.OpenLibertyApplication, error) {
	apps := []openlibertyv1.OpenLibertyApplication{}
	var namespaces []string
	if appstacksutils.IsClusterWide(i.WatchNamespaces) {
		nsList := &corev1.NamespaceList{}
		if err := i.Klient.List(context.Background(), nsList, client.InNamespace("")); err != nil {
			return nil, err
		}
		for _, ns := range nsList.Items {
			namespaces = append(namespaces, ns.Name)
		}
	} else {
		namespaces = i.WatchNamespaces
	}
	for _, ns := range namespaces {
		appList := &openlibertyv1.OpenLibertyApplicationList{}
		err := i.Klient.List(context.Background(),
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
