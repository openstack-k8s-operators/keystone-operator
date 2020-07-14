package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeystoneAPISpec defines the desired state of KeystoneAPI
// +k8s:openapi-gen=true
type KeystoneAPISpec struct {
	// Keystone Database Password String
	DatabasePassword string `json:"databasePassword,omitempty"`
	// Keystone Database Hostname String
	DatabaseHostname string `json:"databaseHostname,omitempty"`
	// Keystone Container Image URL
	ContainerImage string `json:"containerImage,omitempty"`
	// Mysql Container Image URL (used for database syncing)
	MysqlContainerImage string `json:"mysqlContainerImage,omitempty"`
	// Database Admin Username
	DatabaseAdminUsername string `json:"databaseAdminUsername,omitempty"`
	// Database Admin Password
	DatabaseAdminPassword string `json:"databaseAdminPassword,omitempty"`
	// Keystone API Admin Password
	AdminPassword string `json:"adminPassword,omitempty"`
	// Replicas
	Replicas int32 `json:"replicas"`
}

// KeystoneAPIStatus defines the observed state of KeystoneAPI
// +k8s:openapi-gen=true
type KeystoneAPIStatus struct {
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

// KeystoneAPI is the Schema for the keystoneapis API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type KeystoneAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeystoneAPISpec   `json:"spec,omitempty"`
	Status KeystoneAPIStatus `json:"status,omitempty"`
}

// KeystoneAPIList contains a list of KeystoneAPI
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type KeystoneAPIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeystoneAPI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeystoneAPI{}, &KeystoneAPIList{})
}
