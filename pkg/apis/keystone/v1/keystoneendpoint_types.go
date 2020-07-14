package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KeystoneEndpointSpec defines the desired state of KeystoneEndpoint
// +k8s:openapi-gen=true
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
// +k8s:openapi-gen=true
type KeystoneEndpointStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KeystoneEndpoint is the Schema for the keystoneendpoints API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=keystoneendpoints,scope=Namespaced
type KeystoneEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeystoneEndpointSpec   `json:"spec,omitempty"`
	Status KeystoneEndpointStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KeystoneEndpointList contains a list of KeystoneEndpoint
type KeystoneEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeystoneEndpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeystoneEndpoint{}, &KeystoneEndpointList{})
}
