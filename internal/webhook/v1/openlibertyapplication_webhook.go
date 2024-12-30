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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	olv1 "github.com/OpenLiberty/open-liberty-operator/api/v1"
	olcontroller "github.com/OpenLiberty/open-liberty-operator/internal/controller"
	lutils "github.com/OpenLiberty/open-liberty-operator/utils"
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
// +kubebuilder:webhook:path=/validate-apps-openliberty-io-v1-openlibertyapplication,mutating=false,failurePolicy=fail,sideEffects=None,groups=apps.openliberty.io,resources=openlibertyapplications,verbs=create;update,versions=v1,name=vopenlibertyapplication-v1.kb.io,admissionReviewVersions=v1

// OpenLibertyApplicationCustomValidator struct is responsible for validating the OpenLibertyApplication resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type OpenLibertyApplicationCustomValidator struct {
	//TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &OpenLibertyApplicationCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OpenLibertyApplication.
func (v *OpenLibertyApplicationCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	openlibertyapplication, ok := obj.(*olv1.OpenLibertyApplication)
	if !ok {
		return nil, fmt.Errorf("expected a OpenLibertyApplication object but got %T", obj)
	}
	openlibertyapplicationlog.Info("Validation for OpenLibertyApplication upon creation", "name", openlibertyapplication.GetName())

	// TODO(user): fill in your validation logic upon object creation.
	httpClient, err := lutils.GetLibertyProxyClient(lclient, "openshift-operators", olcontroller.OperatorShortName)
	if err != nil {
		return nil, err
	}
	res, err := lutils.GetLibertyProxy("openshift-operators", httpClient, "admissionwebhook")
	if err != nil {
		openlibertyapplicationlog.Error(err, "Error calling validation webhook")
		return nil, err
	}
	openlibertyapplicationlog.Info("Received status response from calling liberty proxy: " + res.Status)
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
	// openlibertyapplication, ok := obj.(*olv1.OpenLibertyApplication)
	// if !ok {
	// 	return nil, fmt.Errorf("expected a OpenLibertyApplication object but got %T", obj)
	// }
	// openlibertyapplicationlog.Info("Validation for OpenLibertyApplication upon deletion", "name", openlibertyapplication.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
