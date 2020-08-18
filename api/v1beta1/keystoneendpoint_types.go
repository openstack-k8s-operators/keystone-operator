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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeystoneEndpointSpec defines the desired state of KeystoneEndpoint
type KeystoneEndpointSpec struct {
	AdminURL    string `json:"adminURL,omitempty"`
	PublicURL   string `json:"publicURL,omitempty"`
	InternalURL string `json:"internalURL,omitempty"`
	ServiceType string `json:"serviceType,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	Region      string `json:"region,omitempty"`
	Project     string `json:"project,omitempty"`
	AuthURL     string `json:"authURL,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	DomainName  string `json:"domainName,omitempty"`
}

// KeystoneEndpointStatus defines the observed state of KeystoneEndpoint
type KeystoneEndpointStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// KeystoneEndpoint is the Schema for the keystoneendpoints API
type KeystoneEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeystoneEndpointSpec   `json:"spec,omitempty"`
	Status KeystoneEndpointStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeystoneEndpointList contains a list of KeystoneEndpoint
type KeystoneEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeystoneEndpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeystoneEndpoint{}, &KeystoneEndpointList{})
}
