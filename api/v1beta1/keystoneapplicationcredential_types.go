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
// ApplicationCredentialSpec defines what the user can set
type ApplicationCredentialSpec struct {
	// UserName - the Keystone user under which this AC is created
	UserName string `json:"userName"`

	// ExpirationDays sets the lifetime in days for the AC
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=14
	// +kubebuilder:validation:Minimum=2
	ExpirationDays int `json:"expirationDays"`

	// GracePeriodDays sets how many days before expiration the AC should be rotated
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=7
	// +kubebuilder:validation:Minimum=1
	GracePeriodDays int `json:"gracePeriodDays"`
}

// ApplicationCredentialStatus defines the observed state
type ApplicationCredentialStatus struct {
	// ACID - the ID in Keystone for this AC
	ACID string `json:"acID,omitempty"`

	// SecretName - name of the k8s Secret storing the AC secret
	SecretName string `json:"secretName,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty"`

	// CreatedAt - timestap of creation
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// ExpiresAt - time of validity expiration
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:shortName=appcred
//+kubebuilder:printcolumn:name="ACID",type="string",JSONPath=".status.acID",description="Keystone AC ID"
//+kubebuilder:printcolumn:name="SecretName",type="string",JSONPath=".status.secretName",description="Secret holding AC secret"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// ApplicationCredential is the Schema for the applicationcredentials API
type ApplicationCredential struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationCredentialSpec   `json:"spec,omitempty"`
	Status ApplicationCredentialStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ApplicationCredentialList contains a list of ApplicationCredential
type ApplicationCredentialList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationCredential `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ApplicationCredential{}, &ApplicationCredentialList{})
}

// IsReady - returns true if ApplicationCredential is reconciled successfully
func (ac *ApplicationCredential) IsReady() bool {
	return ac.Status.Conditions.IsTrue(condition.ReadyCondition)
}
