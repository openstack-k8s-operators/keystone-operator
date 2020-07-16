package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KeystoneServiceSpec defines the desired state of KeystoneService
type KeystoneServiceSpec struct {
	ServiceID          string `json:"serviceID,omitempty"`
	ServiceType        string `json:"serviceType,omitempty"`
	ServiceName        string `json:"serviceName,omitempty"`
	ServiceDescription string `json:"serviceDescription,omitempty"`
	Enabled            bool   `json:"enabled,omitempty"`
	Region             string `json:"region,omitempty"`
	Project            string `json:"project,omitempty"`
	AuthURL            string `json:"authURL,omitempty"`
	Username           string `json:"username,omitempty"`
	Password           string `json:"password,omitempty"`
	DomainName         string `json:"domainName,omitempty"`
}

// KeystoneServiceStatus defines the observed state of KeystoneService
type KeystoneServiceStatus struct {
	ServiceID string `json:"serviceID,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KeystoneService is the Schema for the keystoneservices API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=keystoneservices,scope=Namespaced
type KeystoneService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeystoneServiceSpec   `json:"spec,omitempty"`
	Status KeystoneServiceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KeystoneServiceList contains a list of KeystoneService
type KeystoneServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeystoneService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeystoneService{}, &KeystoneServiceList{})
}
