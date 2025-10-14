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

package v1beta1

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:XValidation:rule="self.gracePeriodDays < self.expirationDays",message="gracePeriodDays must be smaller than expirationDays"
// KeystoneApplicationCredentialSpec defines what the user can set
type KeystoneApplicationCredentialSpec struct {

	// Secret containing service user password
	// +kubebuilder:validation:Required
	Secret string `json:"secret"`

	// PasswordSelector for extracting the service password
	// +kubebuilder:validation:Required
	PasswordSelector string `json:"passwordSelector"`

	// UserName - the Keystone user under which this ApplicationCredential is created
	UserName string `json:"userName"`

	// ExpirationDays sets the lifetime in days for the ApplicationCredential
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=365
	// +kubebuilder:validation:Minimum=2
	ExpirationDays int `json:"expirationDays"`

	// GracePeriodDays sets how many days before expiration the ApplicationCredential should be rotated
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=182
	// +kubebuilder:validation:Minimum=1
	GracePeriodDays int `json:"gracePeriodDays"`

	// Roles to assign to the ApplicationCredential
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Roles []string `json:"roles"`

	// Unrestricted indicates whether the ApplicationCredential may be used to create or destroy other credentials or trusts
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Unrestricted bool `json:"unrestricted"`

	// AccessRules defines which services the ApplicationCredential is permitted to access
	// +kubebuilder:validation:Optional
	AccessRules []ACRule `json:"accessRules,omitempty"`
}

// ACRule defines a additional access rule for an ApplicationCredential
type ACRule struct {
	// Service is the OpenStack service type
	// +kubebuilder:validation:Optional
	Service string `json:"service,omitempty"`

	// Path is the API path to allow
	// +kubebuilder:validation:Optional
	Path string `json:"path,omitempty"`

	// Method is the HTTP verb to allow (defaults to all if empty)
	// +kubebuilder:validation:Optional
	Method string `json:"method,omitempty"`
}

// KeystoneApplicationCredentialStatus defines the observed state
type KeystoneApplicationCredentialStatus struct {
	// ACID - the ID in Keystone for this ApplicationCredential
	ACID string `json:"acID,omitempty"`

	// SecretName - name of the k8s Secret storing the ApplicationCredential secret
	SecretName string `json:"secretName,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty"`

	// CreatedAt - timestap of creation
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// ExpiresAt - time of validity expiration
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// LastRotated - timestamp when credentials were last rotated
	LastRotated *metav1.Time `json:"lastRotated,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=appcred
//+kubebuilder:printcolumn:name="ACID",type="string",JSONPath=".status.acID",description="Keystone ApplicationCredential ID"
//+kubebuilder:printcolumn:name="SecretName",type="string",JSONPath=".status.secretName",description="Secret holding ApplicationCredential secret"
//+kubebuilder:printcolumn:name="LastRotated",type="date",JSONPath=".status.lastRotated",description="Last rotation time"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// KeystoneApplicationCredential is the Schema for the applicationcredentials API
type KeystoneApplicationCredential struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeystoneApplicationCredentialSpec   `json:"spec,omitempty"`
	Status KeystoneApplicationCredentialStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KeystoneApplicationCredentialList contains a list of ApplicationCredential
type KeystoneApplicationCredentialList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeystoneApplicationCredential `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeystoneApplicationCredential{}, &KeystoneApplicationCredentialList{})
}

// IsReady - returns true if ApplicationCredential is reconciled successfully
func (ac *KeystoneApplicationCredential) IsReady() bool {
	return ac.Status.Conditions.IsTrue(condition.ReadyCondition)
}
