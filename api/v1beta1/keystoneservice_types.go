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

// KeystoneServiceSpec defines the desired state of KeystoneService
type KeystoneServiceSpec struct {
	ServiceID          string `json:"serviceID,omitempty"`
	ServiceType        string `json:"serviceType,omitempty"`
	ServiceName        string `json:"serviceName,omitempty"`
	ServiceDescription string `json:"serviceDescription,omitempty"`
	Enabled            bool   `json:"enabled,omitempty"`
	Region             string `json:"region,omitempty"`
	AdminURL           string `json:"adminURL,omitempty"`
	PublicURL          string `json:"publicURL,omitempty"`
	InternalURL        string `json:"internalURL,omitempty"`
	Username           string `json:"username,omitempty"`
	Password           string `json:"password,omitempty"`
}

// KeystoneServiceStatus defines the observed state of KeystoneService
type KeystoneServiceStatus struct {
	ServiceID string `json:"serviceID,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// KeystoneService is the Schema for the keystoneservices API
type KeystoneService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeystoneServiceSpec   `json:"spec,omitempty"`
	Status KeystoneServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeystoneServiceList contains a list of KeystoneService
type KeystoneServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeystoneService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeystoneService{}, &KeystoneServiceList{})
}
