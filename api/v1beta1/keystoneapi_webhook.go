/*
Copyright 2022.

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

package v1beta1

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var keystoneapilog = logf.Log.WithName("keystoneapi-resource")

// SetupWebhookWithManager sets up Webhook with Manager
func (r *KeystoneAPI) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-keystone-openstack-org-v1beta1-keystoneapi,mutating=false,failurePolicy=fail,sideEffects=None,groups=keystone.openstack.org,resources=keystoneapis,verbs=create;update,versions=v1beta1,name=vkeystoneapi.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &KeystoneAPI{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *KeystoneAPI) ValidateCreate() error {
	keystoneapilog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *KeystoneAPI) ValidateUpdate(old runtime.Object) error {
	keystoneapilog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *KeystoneAPI) ValidateDelete() error {
	keystoneapilog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

//+kubebuilder:webhook:path=/mutate-keystone-openstack-org-v1beta1-keystoneapi,mutating=true,failurePolicy=fail,sideEffects=None,groups=keystone.openstack.org,resources=keystoneapis,verbs=create;update,versions=v1beta1,name=mkeystoneapi.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &KeystoneAPI{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *KeystoneAPI) Default() {
	keystoneapilog.Info("Before calling webhook")
	// Default CRD fields as needed

	keystoneapilog.Info("Webhook Called")
}
