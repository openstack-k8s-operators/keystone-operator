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
	condition "github.com/openstack-k8s-operators/lib-common/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// KeystoneFinalizer allows KeystoneAPIReconciler to clean up resources associated with KeystoneAPI before
	// removing it from the apiserver.
	KeystoneFinalizer = "keystoneapi.osp-director.openstack.org"

	// DbSyncHash hash
	DbSyncHash = "dbsync"

	// DeploymentHash hash used to detect changes
	DeploymentHash = "deployment"

	// BootstrapHash completed
	BootstrapHash = "bootstrap"

	// FernetKeysHash completed
	FernetKeysHash = "fernetkeys"
)

// KeystoneAPISpec defines the desired state of KeystoneAPI
type KeystoneAPISpec struct {
	// +kubebuilder:validation:Required
	// MariaDB instance name
	// Right now required by the maridb-operator to get the credentials from the instance to create the DB
	// Might not be required in future
	DatabaseInstance string `json:"databaseInstance,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=keystone
	// DatabaseUser - optional username used for keystone DB, defaults to keystone
	// TODO: -> implement needs work in mariadb-operator, right now only keystone
	DatabaseUser string `json:"databaseUser"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=regionOne
	// Region - optional region name for the keystone service
	Region string `json:"region"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=admin
	// AdminProject - admin project name
	AdminProject string `json:"adminProject"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=admin
	// AdminRole - admin role name
	AdminRole string `json:"adminRole"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=admin
	// AdminUser - admin user name
	AdminUser string `json:"adminUser"`

	// +kubebuilder:validation:Required
	// Keystone Container Image URL
	ContainerImage string `json:"containerImage,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:Minimum=0
	// Replicas of keystone API to run
	Replicas int32 `json:"replicas"`

	// +kubebuilder:validation:Required
	// Secret containing OpenStack password information for keystone KeystoneDatabasePassword, AdminPassword
	Secret string `json:"secret,omitempty"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container is used, it runs and the
	// actual action pod gets started with sleep infinity
	Debug KeystoneDebug `json:"debug,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// PreserveJobs - do not delete jobs after they finished e.g. to check logs
	PreserveJobs bool `json:"preserveJobs,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="# add your customization here"
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as custom.conf file.
	CustomServiceConfig string `json:"customServiceConfig,omitempty"`

	// +kubebuilder:validation:Optional
	// ConfigOverwrite - interface to overwrite default config files like e.g. logging.conf or policy.json.
	// But can also be used to add additional files. Those get added to the service config dir in /etc/<service> .
	// TODO: -> implement
	DefaultConfigOverwrite map[string]string `json:"defaultConfigOverwrite,omitempty"`
}

// KeystoneDebug defines the observed state of KeystoneAPI
type KeystoneDebug struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// DBSync enable debug
	DBSync bool `json:"dbSync,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// ReadyCount enable debug
	Bootstrap bool `json:"bootstrap,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// Service enable debug
	Service bool `json:"service,omitempty"`
}

// KeystoneAPIStatus defines the observed state of KeystoneAPI
type KeystoneAPIStatus struct {
	// ReadyCount of keystone API instances
	ReadyCount int `json:"readyCount,omitempty"`

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// API endpoint
	APIEndpoints map[string]string `json:"apiEndpoint,omitempty"`

	// Conditions
	Conditions condition.List `json:"conditions,omitempty" optional:"true"`

	// Keystone Database Hostname
	DatabaseHostname string `json:"databaseHostname,omitempty"`
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

/*
// GetConditions returns the set of conditions for this object.
func (k *KeystoneAPI) GetConditions() Conditions {
	return k.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (k *KeystoneAPI) SetConditions(conditions Conditions) {
	k.Status.Conditions = conditions
}
*/
