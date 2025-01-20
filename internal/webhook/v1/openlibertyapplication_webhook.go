/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	olcontroller "github.com/OpenLiberty/open-liberty-operator/internal/controller"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
	oputils "github.com/application-stacks/runtime-component-operator/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// nolint:unused
// log is for logging in this package.
var (
	openlibertyapplicationlog = logf.Log.WithName("openlibertyapplication-resource")
	lclient                   client.Client
)

// SetupOpenLibertyApplicationWebhookWithManager registers the webhook for OpenLibertyApplication in the manager.
func SetupOpenLibertyApplicationWebhookWithManager(mgr ctrl.Manager) error {
	lclient = mgr.GetClient()

	return ctrl.NewWebhookManagedBy(mgr).For(&olv1.OpenLibertyApplication{}).
		WithValidator(&OpenLibertyApplicationCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-apps-openliberty-io-v1-openlibertyapplication,mutating=false,failurePolicy=fail,sideEffects=None,groups=apps.openliberty.io,resources=openlibertyapplications,verbs=create;delete,versions=v1,name=vopenlibertyapplication-v1.kb.io,admissionReviewVersions=v1

// OpenLibertyApplicationCustomValidator struct is responsible for validating the OpenLibertyApplication resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type OpenLibertyApplicationCustomValidator struct {
	//TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &OpenLibertyApplicationCustomValidator{}

func createCertManagerIssuerAndCerts(client client.Client, prefix, name, namespace, operatorName, CACommonName string) error {
	// if ok, err := .IsGroupVersionSupported(certmanagerv1.SchemeGroupVersion.String(), "Issuer"); err != nil {
	// 	return err
	// } else if !ok {
	// 	return fmt.Errorf("certmanager not found")
	// }
	// openlibertyapplicationlog.Info("Starting cert initialization...")

	issuer := &certmanagerv1.Issuer{ObjectMeta: metav1.ObjectMeta{
		Name:      prefix + "-self-signed",
		Namespace: namespace,
	}}
	issuer.Spec.SelfSigned = &certmanagerv1.SelfSignedIssuer{}
	issuer.Labels = oputils.MergeMaps(issuer.Labels, map[string]string{"app.kubernetes.io/managed-by": operatorName})
	client.Create(context.TODO(), issuer)
	// if err != nil {
	// 	return err
	// }
	// if err := r.checkIssuerReady(issuer); err != nil {
	// 	return err
	// }

	// create ca cert
	// caCert := &certmanagerv1.Certificate{ObjectMeta: metav1.ObjectMeta{
	// 	Name:      prefix + "-ca-cert",
	// 	Namespace: namespace,
	// }}
	// caCert.Labels = oputils.MergeMaps(caCert.Labels, map[string]string{"app.kubernetes.io/managed-by": operatorName})
	// caCert.Spec.CommonName = CACommonName
	// caCert.Spec.IsCA = true
	// caCert.Spec.SecretName = prefix + "-ca-tls"
	// caCert.Spec.IssuerRef = certmanagermetav1.ObjectReference{
	// 	Name: prefix + "-self-signed",
	// }
	// duration, err := time.ParseDuration(common.LoadFromConfig(common.Config, common.OpConfigCMCADuration))
	// if err != nil {
	// 	return err
	// }
	// caCert.Spec.Duration = &metav1.Duration{Duration: duration}

	// client.Create(context.TODO(), caCert)
	// if err != nil {
	// 	return err
	// }

	// CustomCACert := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
	// 	Name:      prefix + "-custom-ca-tls",
	// 	Namespace: namespace,
	// }}
	// customCACertFound := false
	// err = client.Get(context.TODO(), types.NamespacedName{Name: CustomCACert.GetName(),
	// 	Namespace: CustomCACert.GetNamespace()}, CustomCACert)
	// if err == nil {
	// 	customCACertFound = true
	// } else {
	// 	// if err := r.checkCertificateReady(caCert); err != nil {
	// 	// 	return err
	// 	// }
	// }

	// issuer = &certmanagerv1.Issuer{ObjectMeta: metav1.ObjectMeta{
	// 	Name:      prefix + "-ca-issuer",
	// 	Namespace: namespace,
	// }}
	// issuer.Labels = oputils.MergeMaps(issuer.Labels, map[string]string{"app.kubernetes.io/managed-by": operatorName})
	// issuer.Spec.CA = &certmanagerv1.CAIssuer{}
	// issuer.Spec.CA.SecretName = prefix + "-ca-tls"
	// if issuer.Annotations == nil {
	// 	issuer.Annotations = map[string]string{}
	// }
	// if customCACertFound {
	// 	issuer.Spec.CA.SecretName = CustomCACert.Name

	// }
	// err = client.Create(context.TODO(), issuer)

	serviceAccount := &corev1.ServiceAccount{}
	serviceAccount.Name = name
	serviceAccount.Namespace = namespace
	client.Create(context.TODO(), serviceAccount)
	openlibertyapplicationlog.Info("Reached the end of cert/SA initialization")
	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OpenLibertyApplication.
func (v *OpenLibertyApplicationCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	openlibertyapplication, ok := obj.(*olv1.OpenLibertyApplication)
	if !ok {
		return nil, fmt.Errorf("expected a OpenLibertyApplication object but got %T", obj)
	}
	openlibertyapplicationlog.Info("Validation for OpenLibertyApplication upon creation", "name", openlibertyapplication.GetName())

	if openlibertyapplication.GetExperimental() != nil && openlibertyapplication.GetExperimental().GetBypassWebhook() != nil && *openlibertyapplication.GetExperimental().GetBypassWebhook() {
		openlibertyapplicationlog.Info("Bypassing webhook call upon creation", "name", openlibertyapplication.GetName())
		return nil, nil
	}

	openlibertyapplicationlog.Info("Calling Liberty Proxy from webhook for creation", "name", openlibertyapplication.GetName())
	createCertManagerIssuerAndCerts(lclient, olcontroller.OperatorShortName, openlibertyapplication.Name, openlibertyapplication.Namespace, olcontroller.OperatorName, "Open Liberty Operator")
	// TODO(user): fill in your validation logic upon object creation.
	// httpClient, err := lutils.GetLibertyProxyClient(lclient, "openshift-operators", olcontroller.OperatorShortName)
	// if err != nil {
	// 	openlibertyapplicationlog.Error(err, "Error getting Liberty Proxy client")
	// 	// return nil, err
	// 	return nil, nil
	// }
	// res, err := lutils.GetLibertyProxy("openshift-operators", httpClient, "validatecreate", "name="+openlibertyapplication.GetName(), "namespace="+openlibertyapplication.GetNamespace(), "kind=OpenLibertyApplication")
	// if err != nil {
	// 	openlibertyapplicationlog.Error(err, "Error calling validation webhook")
	// 	// return nil, err
	// 	return nil, nil
	// }
	// defer res.Body.Close()

	// if res.StatusCode == http.StatusOK {
	// 	bodyBytes, err := io.ReadAll(res.Body)
	// 	if err != nil {
	// 		openlibertyapplicationlog.Error(err, "Failed to parse response from liberty proxy")
	// 	}
	// 	bodyString := string(bodyBytes)
	// 	openlibertyapplicationlog.Info("Received status response from calling liberty proxy: (" + res.Status + ")")
	// 	openlibertyapplicationlog.Info("    - response:" + bodyString)
	// } else {
	// 	openlibertyapplicationlog.Info("Received status response from calling liberty proxy: " + res.Status)
	// }
	return nil, nil // fmt.Errorf("err: block validate create: " + res.Status)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type OpenLibertyApplication.
func (v *OpenLibertyApplicationCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	// openlibertyapplication, ok := newObj.(*olv1.OpenLibertyApplication)
	// if !ok {
	// 	return nil, fmt.Errorf("expected a OpenLibertyApplication object for the newObj but got %T", newObj)
	// }
	// openlibertyapplicationlog.Info("Validation for OpenLibertyApplication upon update", "name", openlibertyapplication.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type OpenLibertyApplication.
func (v *OpenLibertyApplicationCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	openlibertyapplication, ok := obj.(*olv1.OpenLibertyApplication)
	if !ok {
		return nil, fmt.Errorf("expected a OpenLibertyApplication object but got %T", obj)
	}
	openlibertyapplicationlog.Info("Validation for OpenLibertyApplication upon deletion", "name", openlibertyapplication.GetName())

	if openlibertyapplication.GetExperimental() != nil && openlibertyapplication.GetExperimental().GetBypassWebhook() != nil && *openlibertyapplication.GetExperimental().GetBypassWebhook() {
		return nil, nil
	}

	httpClient, err := lutils.GetLibertyProxyClient(lclient, "openshift-operators", olcontroller.OperatorShortName)
	if err != nil {
		// return nil, err
		return nil, nil
	}

	res, err := lutils.GetLibertyProxy("openshift-operators", httpClient, "validatedelete", "name="+openlibertyapplication.GetName(), "namespace="+openlibertyapplication.GetNamespace(), "kind=OpenLibertyApplication")
	if err != nil {
		openlibertyapplicationlog.Error(err, "Error calling validation webhook")
		// return nil, err
		return nil, nil
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		bodyBytes, err := io.ReadAll(res.Body)
		if err != nil {
			openlibertyapplicationlog.Error(err, "Failed to parse response from liberty proxy")
		}
		bodyString := string(bodyBytes)
		openlibertyapplicationlog.Info("Received status response from calling liberty proxy: (" + res.Status + ")")
		openlibertyapplicationlog.Info("    - response:" + bodyString)
	} else {
		openlibertyapplicationlog.Info("Received status response from calling liberty proxy: " + res.Status)
	}
	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
