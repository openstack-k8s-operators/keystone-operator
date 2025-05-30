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
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeystoneEndpointSpec defines the desired state of KeystoneEndpoint
type KeystoneEndpointSpec struct {
	// +kubebuilder:validation:Required
	// ServiceName - Name of the service to create the endpoint for
	ServiceName string `json:"serviceName"`
	// +kubebuilder:validation:Required
	// Endpoints - map with service api endpoint URLs with the endpoint type as index
	Endpoints map[string]string `json:"endpoints"`
}

// KeystoneEndpointStatus defines the observed state of KeystoneEndpoint
type KeystoneEndpointStatus struct {
	EndpointIDs map[string]string `json:"endpointIDs,omitempty"`
	ServiceID   string            `json:"serviceID,omitempty"`
	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// Endpoints - current status of latest configured endpoints for the service
	Endpoints []Endpoint `json:"endpoints,omitempty"`

	//ObservedGeneration - the most recent generation observed for this service. If the observed generation is less than the spec generation, then the controller has not processed the latest changes.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Endpoint -
type Endpoint struct {
	// Interface - public, internal, admin
	Interface string `json:"interface"`
	// URL - endpoint url
	URL string `json:"url"`
	// ID - endpoint id
	ID string `json:"id"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// KeystoneEndpoint is the Schema for the keystoneendpoints API
type KeystoneEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeystoneEndpointSpec   `json:"spec,omitempty"`
	Status KeystoneEndpointStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KeystoneEndpointList contains a list of KeystoneEndpoint
type KeystoneEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeystoneEndpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeystoneEndpoint{}, &KeystoneEndpointList{})
}

// IsReady - returns true if KeystoneEndpoint is reconciled successfully
func (instance KeystoneEndpoint) IsReady() bool {
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}
