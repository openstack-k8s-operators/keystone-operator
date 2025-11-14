/*
Copyright 2025.

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

// Package v1beta1 contains webhook implementations for KeystoneAPI v1beta1 resources.
package v1beta1

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
)

// Static errors for type assertions
var (
	errUnexpectedObjectType = errors.New("unexpected object type")
)

// nolint:unused
// log is for logging in this package.
var keystoneapilog = logf.Log.WithName("keystoneapi-resource")

// SetupKeystoneAPIWebhookWithManager registers the webhook for KeystoneAPI in the manager.
func SetupKeystoneAPIWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&keystonev1beta1.KeystoneAPI{}).
		WithValidator(&KeystoneAPICustomValidator{}).
		WithDefaulter(&KeystoneAPICustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-keystone-openstack-org-v1beta1-keystoneapi,mutating=true,failurePolicy=fail,sideEffects=None,groups=keystone.openstack.org,resources=keystoneapis,verbs=create;update,versions=v1beta1,name=mkeystoneapi-v1beta1.kb.io,admissionReviewVersions=v1

// KeystoneAPICustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind KeystoneAPI when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type KeystoneAPICustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &KeystoneAPICustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind KeystoneAPI.
func (d *KeystoneAPICustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	keystoneapi, ok := obj.(*keystonev1beta1.KeystoneAPI)

	if !ok {
		return fmt.Errorf("%w: expected an KeystoneAPI object but got %T", errUnexpectedObjectType, obj)
	}
	keystoneapilog.Info("Defaulting for KeystoneAPI", "name", keystoneapi.GetName())

	// Call the defaulting logic from api/v1beta1
	keystoneapi.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-keystone-openstack-org-v1beta1-keystoneapi,mutating=false,failurePolicy=fail,sideEffects=None,groups=keystone.openstack.org,resources=keystoneapis,verbs=create;update,versions=v1beta1,name=vkeystoneapi-v1beta1.kb.io,admissionReviewVersions=v1

// KeystoneAPICustomValidator struct is responsible for validating the KeystoneAPI resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type KeystoneAPICustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &KeystoneAPICustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type KeystoneAPI.
func (v *KeystoneAPICustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	keystoneapi, ok := obj.(*keystonev1beta1.KeystoneAPI)
	if !ok {
		return nil, fmt.Errorf("%w: expected a KeystoneAPI object but got %T", errUnexpectedObjectType, obj)
	}
	keystoneapilog.Info("Validation for KeystoneAPI upon creation", "name", keystoneapi.GetName())

	// Call the validation logic from api/v1beta1
	return keystoneapi.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type KeystoneAPI.
func (v *KeystoneAPICustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	keystoneapi, ok := newObj.(*keystonev1beta1.KeystoneAPI)
	if !ok {
		return nil, fmt.Errorf("%w: expected a KeystoneAPI object for the newObj but got %T", errUnexpectedObjectType, newObj)
	}
	keystoneapilog.Info("Validation for KeystoneAPI upon update", "name", keystoneapi.GetName())

	// Call the validation logic from api/v1beta1
	return keystoneapi.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type KeystoneAPI.
func (v *KeystoneAPICustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	keystoneapi, ok := obj.(*keystonev1beta1.KeystoneAPI)
	if !ok {
		return nil, fmt.Errorf("%w: expected a KeystoneAPI object but got %T", errUnexpectedObjectType, obj)
	}
	keystoneapilog.Info("Validation for KeystoneAPI upon deletion", "name", keystoneapi.GetName())

	// Call the validation logic from api/v1beta1
	return keystoneapi.ValidateDelete()
}
