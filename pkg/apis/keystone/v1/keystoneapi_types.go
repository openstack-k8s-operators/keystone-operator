package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeystoneApiSpec defines the desired state of KeystoneApi
// +k8s:openapi-gen=true
type KeystoneApiSpec struct {
	// Keystone Database Password String
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors=true
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.displayName="Database Password"
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.x-descriptors="urn:alm:descriptor:com.tectonic.ui:password"
	DatabasePassword string `json:"databasePassword,omitempty"`
	// Keystone Database Hostname String
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors=true
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.displayName="Database Hostname"
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.x-descriptors="urn:alm:descriptor:com.tectonic.ui:text"
	DatabaseHostname string `json:"databaseHostname,omitempty"`
	// Keystone Container Image URL
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors=true
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.displayName="Container Image"
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.x-descriptors="urn:alm:descriptor:com.tectonic.ui:text"
	ContainerImage string `json:"containerImage,omitempty"`
	// Mysql Container Image URL (used for database syncing)
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors=true
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.displayName="MySQL Container Image"
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.x-descriptors="urn:alm:descriptor:com.tectonic.ui:text"
	MysqlContainerImage string `json:"mysqlContainerImage,omitempty"`
	// Database Admin Username
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors=true
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.displayName="Database Admin Username"
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.x-descriptors="urn:alm:descriptor:com.tectonic.ui:text"
	DatabaseAdminUsername string `json:"databaseAdminUsername,omitempty"`
	// Database Admin Password
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors=true
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.displayName="Database Admin Password"
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.x-descriptors="urn:alm:descriptor:com.tectonic.ui:password"
	DatabaseAdminPassword string `json:"databaseAdminPassword,omitempty"`
	// Keystone API Admin Password
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors=true
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.displayName="Admin Password"
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.x-descriptors="urn:alm:descriptor:com.tectonic.ui:password"
	AdminPassword string `json:"adminPassword,omitempty"`
	// Keystone API Endpoint, the http/https route configured to access the Keystone API
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors=true
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.displayName="API Endpoint"
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.x-descriptors="urn:alm:descriptor:com.tectonic.ui:text"
	ApiEndpoint string `json:"apiEndpoint,omitempty"`
	// Replicas
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors=true
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.displayName="Replica Count"
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.x-descriptors="urn:alm:descriptor:com.tectonic.ui:number"
	Replicas int32 `json:"replicas"`
}

// KeystoneApiStatus defines the observed state of KeystoneApi
// +k8s:openapi-gen=true
type KeystoneApiStatus struct {
	// DbSync hash
	// +operator-sdk:gen-csv:customresourcedefinitions.statusDescriptors=true
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.displayName="Database Sync Hash"
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.x-descriptors="urn:alm:descriptor:com.tectonic.ui:text"
	DbSyncHash string `json:"dbSyncHash,omitempty"`
	// Deployment hash used to detect changes
	// +operator-sdk:gen-csv:customresourcedefinitions.statusDescriptors=true
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.displayName="Deployment Hash"
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.x-descriptors="urn:alm:descriptor:com.tectonic.ui:text"
	DeploymentHash string `json:"deploymentHash,omitempty"`
	// bootstrap completed
	// +operator-sdk:gen-csv:customresourcedefinitions.statusDescriptors=true
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.displayName="Bootstrap Hash"
	// +operator-sdk:gen-csv:customresourcedefinitions.specDescriptors.x-descriptors="urn:alm:descriptor:com.tectonic.ui:text"
	BootstrapHash string `json:"bootstrapHash,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KeystoneApi is the Schema for the keystoneapis API
// +operator-sdk:gen-csv:customresourcedefinitions.displayName="KeystoneApi"
// +operator-sdk:gen-csv:customresourcedefinitions.resources="Pods,v1,\"keystone-operator\""
// +operator-sdk:gen-csv:customresourcedefinitions.resources=`ConfigMaps,v1,"keystone-operator"`
// +operator-sdk:gen-csv:customresourcedefinitions.resources=`Secrets,v1,"keystone-operator"`
// +operator-sdk:gen-csv:customresourcedefinitions.resources=`Jobs,v1,"keystone-operator"`
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
