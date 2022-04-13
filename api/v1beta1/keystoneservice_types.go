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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KeystoneServiceSpec defines the desired state of KeystoneService
type KeystoneServiceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of KeystoneService. Edit keystoneservice_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// KeystoneServiceStatus defines the observed state of KeystoneService
type KeystoneServiceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
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
