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
	condition "github.com/openstack-k8s-operators/lib-common/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeystoneServiceSpec defines the desired state of KeystoneService
type KeystoneServiceSpec struct {
	// +kubebuilder:validation:Required
	// ServiceType - Type is the type of the service.
	ServiceType string `json:"serviceType,omitempty"`
	// +kubebuilder:validation:Required
	// ServiceName - Name of the service.
	ServiceName string `json:"serviceName,omitempty"`
	// +kubebuilder:validation:Optional
	// ServiceDescription - Description for the service.
	ServiceDescription string `json:"serviceDescription,omitempty"`
	// +kubebuilder:validation:Required
	// Enabled - whether or not the service is enabled.
	Enabled bool `json:"enabled,omitempty"`
	// APIEndpoints - service api endpoint URLs
	APIEndpoints map[string]string `json:"apiEndpoints,omitempty"`
	// +kubebuilder:validation:Required
	// ServiceUser - optional username used for this service
	ServiceUser string `json:"serviceUser,omitempty"`
	// +kubebuilder:validation:Required
	// Secret containing OpenStack password information for the ServiceUser
	Secret string `json:"secret,omitempty"`
	// +kubebuilder:validation:Required
	// PasswordSelector - Selector to get the ServiceUser password from the Secret, e.g. PlacementPassword
	PasswordSelector string `json:"passwordSelector,omitempty"`
}

// KeystoneServiceStatus defines the observed state of KeystoneService
type KeystoneServiceStatus struct {
	ServiceID string `json:"serviceID,omitempty"`
	// Conditions
	Conditions condition.List `json:"conditions,omitempty" optional:"true"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// KeystoneService is the Schema for the keystoneservices API
type KeystoneService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeystoneServiceSpec   `json:"spec,omitempty"`
	Status KeystoneServiceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KeystoneServiceList contains a list of KeystoneService
type KeystoneServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeystoneService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeystoneService{}, &KeystoneServiceList{})
}
