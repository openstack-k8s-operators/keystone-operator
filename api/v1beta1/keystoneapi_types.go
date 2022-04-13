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

// KeystoneAPISpec defines the desired state of KeystoneAPI
type KeystoneAPISpec struct {
	// Keystone Database Hostname String
	DatabaseHostname string `json:"databaseHostname,omitempty"`
	// Keystone Container Image URL
	ContainerImage string `json:"containerImage,omitempty"`
	// Keystone Secret containing DatabasePassword, AdminPassword
	Secret string `json:"secret,omitempty"`
	// Replicas
	Replicas int32 `json:"replicas"`
}

// KeystoneAPIStatus defines the observed state of KeystoneAPI
type KeystoneAPIStatus struct {
	// DbSync hash
	DbSyncHash string `json:"dbSyncHash"`
	// Deployment hash used to detect changes
	DeploymentHash string `json:"deploymentHash"`
	// bootstrap completed
	BootstrapHash string `json:"bootstrapHash"`
	// API endpoint
	APIEndpoint string `json:"apiEndpoint"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// KeystoneAPI is the Schema for the keystoneapis API
type KeystoneAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeystoneAPISpec   `json:"spec,omitempty"`
	Status KeystoneAPIStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KeystoneAPIList contains a list of KeystoneAPI
type KeystoneAPIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeystoneAPI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeystoneAPI{}, &KeystoneAPIList{})
}
