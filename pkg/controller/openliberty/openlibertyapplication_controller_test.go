package openliberty

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	"strconv"

	openlibertyv1beta1 "github.com/OpenLiberty/open-liberty-operator/pkg/apis/openliberty/v1beta1"
	oputils "github.com/application-stacks/runtime-component-operator/pkg/utils"
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	coretesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	name                       = "app"
	namespace                  = "openliberty"
	appImage                   = "my-image"
	ksvcAppImage               = "ksvc-image"
	defaultMeta                = metav1.ObjectMeta{Name: name, Namespace: namespace}
	replicas             int32 = 3
	autoscaling                = &openlibertyv1beta1.OpenLibertyApplicationAutoScaling{MaxReplicas: 3}
	pullPolicy                 = corev1.PullAlways
	serviceType                = corev1.ServiceTypeClusterIP
	service                    = &openlibertyv1beta1.OpenLibertyApplicationService{Type: serviceType, Port: 9080}
	expose                     = true
	serviceAccountName         = "service-account"
	volumeCT                   = &corev1.PersistentVolumeClaim{TypeMeta: metav1.TypeMeta{Kind: "StatefulSet"}}
	storage                    = openlibertyv1beta1.OpenLibertyApplicationStorage{Size: "10Mi", MountPath: "/mnt/data", VolumeClaimTemplate: volumeCT}
	createKnativeService       = true
	statefulSetSN              = name + "-headless"
)

type Test struct {
	test     string
	expected interface{}
	actual   interface{}
}

func TestOpenLibertyController(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	os.Setenv("WATCH_NAMESPACE", namespace)

	spec := openlibertyv1beta1.OpenLibertyApplicationSpec{}
	openliberty := createOpenLibertyApp(name, namespace, spec)

	// Set objects to track in the fake client and register operator types with the runtime scheme.
	objs, s := []runtime.Object{openliberty}, scheme.Scheme

	// Add third party resrouces to scheme
	if err := servingv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add servingv1alpha1 scheme: (%v)", err)
	}

	if err := routev1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add route scheme: (%v)", err)
	}

	s.AddKnownTypes(openlibertyv1beta1.SchemeGroupVersion, openliberty)

	// Create a fake client to mock API calls.
	cl := fakeclient.NewFakeClient(objs...)

	rb := oputils.NewReconcilerBase(cl, s, &rest.Config{}, record.NewFakeRecorder(10))

	// Create a ReconcileAppsodyApplication object
	r := &ReconcileOpenLiberty{ReconcilerBase: rb}
	r.SetDiscoveryClient(createFakeDiscoveryClient())

	if err := testBasicReconcile(t, r, rb); err != nil {
		t.Fatalf("%v", err)
	}

	if err := testStorage(t, r, rb); err != nil {
		t.Fatalf("%v", err)
	}

	if err := testKnativeService(t, r, rb); err != nil {
		t.Fatalf("%v", err)
	}

	if err := testExposeRoute(t, r, rb); err != nil {
		t.Fatalf("%v", err)
	}

	if err := testAutoscaling(t, r, rb); err != nil {
		t.Fatalf("%v", err)
	}

	if err := testServiceMonitoring(t, r, rb); err != nil {
		t.Fatalf("%v", err)
	}

	if err := testServiceAccount(t, r, rb); err != nil {
		t.Fatalf("%v", err)
	}
}

// Test methods
func testBasicReconcile(t *testing.T, r *ReconcileOpenLiberty, rb oputils.ReconcilerBase) error {
	// Mock request to simulate Reconcile being called on an event for a watched resource
	// then ensure reconcile is successful and does not return an empty result
	req := createReconcileRequest(name, namespace)
	res, err := r.Reconcile(req)

	if err = verifyReconcile(res, err); err != nil {
		return err
	}

	// Check if deployment has been created
	dep := &appsv1.Deployment{}
	if err = r.GetClient().Get(context.TODO(), req.NamespacedName, dep); err != nil {
		return fmt.Errorf("Get Deployment: (%v)", err)
	}

	return nil
}

func testStorage(t *testing.T, r *ReconcileOpenLiberty, rb oputils.ReconcilerBase) error {
	spec := openlibertyv1beta1.OpenLibertyApplicationSpec{}
	openliberty := createOpenLibertyApp(name, namespace, spec)
	req := createReconcileRequest(name, namespace)

	openliberty.Spec = openlibertyv1beta1.OpenLibertyApplicationSpec{
		Storage:          &storage,
		Replicas:         &replicas,
		ApplicationImage: appImage,
	}
	updateOpenLiberty(r, openliberty, t)

	res, err := r.Reconcile(req)
	if err = verifyReconcile(res, err); err != nil {
		return err
	}

	statefulset := &appsv1.StatefulSet{}
	if err = r.GetClient().Get(context.TODO(), req.NamespacedName, statefulset); err != nil {
		return fmt.Errorf("Get StatefulSet (%v)", err)
	}

	// Storage is on so the previously created deployment should be deleted
	dep := &appsv1.Deployment{}
	if err = r.GetClient().Get(context.TODO(), req.NamespacedName, dep); err == nil {
		return fmt.Errorf("Deployment was not deleted with storage enabled")
	}

	tests := []Test{
		{"replicas", replicas, *statefulset.Spec.Replicas},
		{"service image name", appImage, statefulset.Spec.Template.Spec.Containers[0].Image},
		{"pull policy", name, statefulset.Spec.Template.Spec.ServiceAccountName},
		{"service account name", statefulSetSN, statefulset.Spec.ServiceName},
	}
	if err = verifyTests(tests); err != nil {
		return err
	}

	return nil
}

func testKnativeService(t *testing.T, r *ReconcileOpenLiberty, rb oputils.ReconcilerBase) error {
	spec := openlibertyv1beta1.OpenLibertyApplicationSpec{}
	openliberty := createOpenLibertyApp(name, namespace, spec)
	req := createReconcileRequest(name, namespace)

	openliberty.Spec = openlibertyv1beta1.OpenLibertyApplicationSpec{
		CreateKnativeService: &createKnativeService,
		PullPolicy:           &pullPolicy,
		ApplicationImage:     ksvcAppImage,
	}
	updateOpenLiberty(r, openliberty, t)

	res, err := r.Reconcile(req)
	if err = verifyReconcile(res, err); err != nil {
		return err
	}

	// Create KnativeService
	ksvc := &servingv1alpha1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "serving.knative.dev/v1alpha1",
			Kind:       "Service",
		},
	}
	if err = r.GetClient().Get(context.TODO(), req.NamespacedName, ksvc); err != nil {
		return fmt.Errorf("Get KnativeService: (%v)", err)
	}

	statefulset := &appsv1.StatefulSet{}
	// KnativeService is enabled so non-Knative resources should be deleted
	if err = r.GetClient().Get(context.TODO(), req.NamespacedName, statefulset); err == nil {
		return fmt.Errorf("StatefulSet was not deleted")
	}

	// Check updated values in KnativeService
	ksvcTests := []Test{
		{"service image name", ksvcAppImage, ksvc.Spec.Template.Spec.Containers[0].Image},
		{"pull policy", pullPolicy, ksvc.Spec.Template.Spec.Containers[0].ImagePullPolicy},
		{"service account name", name, ksvc.Spec.Template.Spec.ServiceAccountName},
	}
	if err = verifyTests(ksvcTests); err != nil {
		return err
	}

	return nil
}

func testExposeRoute(t *testing.T, r *ReconcileOpenLiberty, rb oputils.ReconcilerBase) error {
	spec := openlibertyv1beta1.OpenLibertyApplicationSpec{}
	openliberty := createOpenLibertyApp(name, namespace, spec)
	req := createReconcileRequest(name, namespace)

	expose := true
	openliberty.Spec = openlibertyv1beta1.OpenLibertyApplicationSpec{
		Expose: &expose,
	}
	updateOpenLiberty(r, openliberty, t)

	res, err := r.Reconcile(req)
	if rerr := verifyReconcile(res, err); rerr != nil {
		return err
	}

	route := &routev1.Route{}
	if err = r.GetClient().Get(context.TODO(), req.NamespacedName, route); err != nil {
		return fmt.Errorf("Route (%v)", err)
	}

	routeTests := []Test{{"target port", intstr.FromString(strconv.Itoa(int(service.Port)) + "-tcp"), route.Spec.Port.TargetPort}}
	if err = verifyTests(routeTests); err != nil {
		return err
	}

	return nil
}

func testAutoscaling(t *testing.T, r *ReconcileOpenLiberty, rb oputils.ReconcilerBase) error {
	spec := openlibertyv1beta1.OpenLibertyApplicationSpec{}
	openliberty := createOpenLibertyApp(name, namespace, spec)
	req := createReconcileRequest(name, namespace)

	openliberty.Spec = openlibertyv1beta1.OpenLibertyApplicationSpec{
		Autoscaling: autoscaling,
	}
	updateOpenLiberty(r, openliberty, t)

	res, err := r.Reconcile(req)
	if err = verifyReconcile(res, err); err != nil {
		return err
	}

	hpa := &autoscalingv1.HorizontalPodAutoscaler{}
	if err = r.GetClient().Get(context.TODO(), req.NamespacedName, hpa); err != nil {
		return fmt.Errorf("Autoscaling (%v)", err)
	}
	// verify that the route has been deleted now that expose is disabled
	route := &routev1.Route{}
	if err = r.GetClient().Get(context.TODO(), req.NamespacedName, route); err == nil {
		return fmt.Errorf("Failed to delete Route")
	}

	// Check updated values in hpa
	hpaTests := []Test{{"max replicas", autoscaling.MaxReplicas, hpa.Spec.MaxReplicas}}
	if err = verifyTests(hpaTests); err != nil {
		return err
	}
	return nil
}

func testServiceAccount(t *testing.T, r *ReconcileOpenLiberty, rb oputils.ReconcilerBase) error {
	spec := openlibertyv1beta1.OpenLibertyApplicationSpec{}
	openliberty := createOpenLibertyApp(name, namespace, spec)
	req := createReconcileRequest(name, namespace)

	updateOpenLiberty(r, openliberty, t)

	res, err := r.Reconcile(req)
	if err = verifyReconcile(res, err); err != nil {
		return err
	}

	serviceaccount := &corev1.ServiceAccount{ObjectMeta: defaultMeta}
	if err = r.GetClient().Get(context.TODO(), req.NamespacedName, serviceaccount); err != nil {
		return err
	}

	openliberty.Spec = openlibertyv1beta1.OpenLibertyApplicationSpec{
		ServiceAccountName: &serviceAccountName,
	}
	updateOpenLiberty(r, openliberty, t)

	// check that the default service account was deleted
	if err = r.GetClient().Get(context.TODO(), req.NamespacedName, serviceaccount); err == nil {
		return err
	}
	return nil
}

// most of this functionality is handled by autils, only verifying liberty logic
func testServiceMonitoring(t *testing.T, r *ReconcileOpenLiberty, rb oputils.ReconcilerBase) error {
	spec := openlibertyv1beta1.OpenLibertyApplicationSpec{}
	openliberty := createOpenLibertyApp(name, namespace, spec)
	req := createReconcileRequest(name, namespace)

	// Test with monitoring specified
	openliberty.Spec.Monitoring = &openlibertyv1beta1.OpenLibertyApplicationMonitoring{}
	updateOpenLiberty(r, openliberty, t)
	res, err := r.Reconcile(req)
	if err = verifyReconcile(res, err); err != nil {
		return err
	}

	svc := &corev1.Service{ObjectMeta: defaultMeta}
	if err = r.GetClient().Get(context.TODO(), req.NamespacedName, svc); err != nil {
		return err
	}

	monitorTests := []Test{
		{"Monitor label assigned", "true", svc.Labels["app."+openliberty.GetGroupName()+"/monitor"]},
	}
	if err = verifyTests(monitorTests); err != nil {
		return err
	}

	// Test without monitoring on
	openliberty.Spec.Monitoring = nil
	updateOpenLiberty(r, openliberty, t)
	res, err = r.Reconcile(req)
	if err = verifyReconcile(res, err); err != nil {
		return err
	}

	svc = &corev1.Service{ObjectMeta: defaultMeta}
	if err = r.GetClient().Get(context.TODO(), req.NamespacedName, svc); err != nil {
		return err
	}

	monitorTests = []Test{
		{"Monitor label unassigned", "", svc.Labels["app."+openliberty.GetGroupName()+"/monitor"]},
	}
	if err = verifyTests(monitorTests); err != nil {
		return err
	}

	return nil
}

// Helper Functions
func createOpenLibertyApp(n, ns string, spec openlibertyv1beta1.OpenLibertyApplicationSpec) *openlibertyv1beta1.OpenLibertyApplication {
	app := &openlibertyv1beta1.OpenLibertyApplication{
		ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: ns},
		Spec:       spec,
	}
	return app
}

func createFakeDiscoveryClient() discovery.DiscoveryInterface {
	fakeDiscoveryClient := &fakediscovery.FakeDiscovery{Fake: &coretesting.Fake{}}
	fakeDiscoveryClient.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: routev1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "routes", Namespaced: true, Kind: "Route"},
			},
		},
		{
			GroupVersion: servingv1alpha1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{Name: "services", Namespaced: true, Kind: "Service", SingularName: "service"},
			},
		},
	}

	return fakeDiscoveryClient
}

func createReconcileRequest(n, ns string) reconcile.Request {
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: n, Namespace: ns},
	}
	return req
}

func createConfigMap(n, ns string, data map[string]string) *corev1.ConfigMap {
	app := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: ns},
		Data:       data,
	}
	return app
}

// verifyReconcile checks that there was no error and that the reconcile is valid
func verifyReconcile(res reconcile.Result, err error) error {
	if err != nil {
		return fmt.Errorf("reconcile: (%v)", err)
	}

	if res != (reconcile.Result{}) {
		return fmt.Errorf("reconcile did not return an empty result (%v)", res)
	}

	return nil
}

func verifyTests(tests []Test) error {
	for _, tt := range tests {
		if !reflect.DeepEqual(tt.actual, tt.expected) {
			return fmt.Errorf("%s test expected: (%v) actual: (%v)", tt.test, tt.expected, tt.actual)
		}
	}
	return nil
}

func updateOpenLiberty(r *ReconcileOpenLiberty, openliberty *openlibertyv1beta1.OpenLibertyApplication, t *testing.T) {
	if err := r.GetClient().Update(context.TODO(), openliberty); err != nil {
		t.Fatalf("Update openliberty: (%v)", err)
	}
}
