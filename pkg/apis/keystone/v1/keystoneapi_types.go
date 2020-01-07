package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeystoneApiSpec defines the desired state of KeystoneApi
// +k8s:openapi-gen=true
type KeystoneApiSpec struct {
	// Keystone Database Password String
	DatabasePassword string `json:"databasePassword,omitempty"`
	// Keystone Database Hostname String
	DatabaseHostname string `json:"databaseHostname,omitempty"`
	// Keystone Database Username
	DatabaseUsername string `json:"databaseUsername,omitempty"`
	// Keystone Container Image URL
	ContainerImage string `json:"containerImage,omitempty"`
	// Mysql Container Image URL (used for database syncing)
	MysqlContainerImage string `json:"mysqlContainerImage,omitempty"`
	// Admin database password
	AdminDatabasePassword string `json:"adminDatabasePassword,omitempty"`
	// Keystone API Admin Password
	AdminPassword string `json:"adminPassword,omitempty"`
	// Keystone API Endpoint, the http/https route configured to access the Keystone API
	ApiEndpoint string `json:"apiEndpoint,omitempty"`
	// Replicas
	Replicas int32 `json:"replicas"`
}

// KeystoneApiStatus defines the observed state of KeystoneApi
// +k8s:openapi-gen=true
type KeystoneApiStatus struct {
	// Deployment messages
	//Messages []string `json:"messages,omitempty"`
	// DbSync hash
	DbSyncHash string `json:"dbSyncHash"`
	// Deployment hash used to detect changes
	DeploymentHash string `json:"deploymentHash"`
	// bootstrap completed
	BootstrapHash string `json:"bootstrapHash"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KeystoneApi is the Schema for the keystoneapis API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type KeystoneApi struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeystoneApiSpec   `json:"spec,omitempty"`
	Status KeystoneApiStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// KeystoneApiList contains a list of KeystoneApi
type KeystoneApiList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeystoneApi `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeystoneApi{}, &KeystoneApiList{})
}
