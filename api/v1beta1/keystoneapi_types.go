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
	"fmt"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DbSyncHash hash
	DbSyncHash = "dbsync"

	// DeploymentHash hash used to detect changes
	DeploymentHash = "deployment"

	// BootstrapHash completed
	BootstrapHash = "bootstrap"

	// FernetKeysHash completed
	FernetKeysHash = "fernetkeys"

	// Container image fall-back defaults

	// KeystoneAPIContainerImage is the fall-back container image for KeystoneAPI
	KeystoneAPIContainerImage = "quay.io/podified-antelope-centos9/openstack-keystone:current-podified"
)

type KeystoneAPISpec struct {
	KeystoneAPISpecCore `json:",inline"`

	// +kubebuilder:validation:Required
	// Keystone Container Image URL (will be set to environmental default if empty)
	ContainerImage string `json:"containerImage"`
}

// KeystoneAPISpec defines the desired state of KeystoneAPI
type KeystoneAPISpecCore struct {
	// +kubebuilder:validation:Required
	// MariaDB instance name
	// Right now required by the maridb-operator to get the credentials from the instance to create the DB
	// Might not be required in future
	DatabaseInstance string `json:"databaseInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=keystone
	// DatabaseAccount - name of MariaDBAccount which will be used to connect.
	DatabaseAccount string `json:"databaseAccount"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=memcached
	// Memcached instance name.
	MemcachedInstance string `json:"memcachedInstance"`

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
	// AdminUser - admin user name
	AdminUser string `json:"adminUser"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:Minimum=0
	// Replicas of keystone API to run
	Replicas *int32 `json:"replicas"`

	// +kubebuilder:validation:Required
	// Secret containing OpenStack password information for keystone AdminPassword
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	// EnableSecureRBAC - Enable Consistent and Secure RBAC policies
	EnableSecureRBAC bool `json:"enableSecureRBAC"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=""
	// TrustFlushArgs - Arguments added to keystone-manage trust_flush command
	TrustFlushArgs string `json:"trustFlushArgs"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="1 * * * *"
	// TrustFlushSchedule - Schedule to purge expired or soft-deleted trusts from database
	TrustFlushSchedule string `json:"trustFlushSchedule"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// TrustFlushSuspend - Suspend the cron job to purge trusts
	TrustFlushSuspend bool `json:"trustFlushSuspend"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// FernetRotationDays - Rotate fernet token keys every X days
	FernetRotationDays *int32 `json:"fernetRotationDays"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=3
	// FernetMaxActiveKeys - Maximum number of fernet token keys after rotation
	FernetMaxActiveKeys *int32 `json:"fernetMaxActiveKeys"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={admin: AdminPassword, keystoneOIDCClientSecret: KeystoneClientSecret, keystoneOIDCCryptoPassphrase: KeystoneCryptoPassphrase}
	// PasswordSelectors - Selectors to identify the AdminUser, KeystoneOIDCClient, and KeystoneOIDCCryptoPassphrase passwords from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service
	NodeSelector *map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// PreserveJobs - do not delete jobs after they finished e.g. to check logs
	PreserveJobs bool `json:"preserveJobs"`

	// +kubebuilder:validation:Optional
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as custom.conf file.
	CustomServiceConfig string `json:"customServiceConfig,omitempty"`

	// +kubebuilder:validation:Optional
	// ConfigOverwrite - interface to overwrite default config files like e.g. policy.json.
	// But can also be used to add additional files. Those get added to the service config dir in /etc/<service> .
	// TODO: -> implement
	DefaultConfigOverwrite map[string]string `json:"defaultConfigOverwrite,omitempty"`

	// +kubebuilder:validation:Optional
	// HttpdCustomization - customize the httpd service
	HttpdCustomization HttpdCustomization `json:"httpdCustomization,omitempty"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +kubebuilder:validation:Optional
	// NetworkAttachments is a list of NetworkAttachment resource names to expose the services to the given network
	NetworkAttachments []string `json:"networkAttachments,omitempty"`

	// +kubebuilder:validation:Optional
	// Override, provides the ability to override the generated manifest of several child resources.
	Override APIOverrideSpec `json:"override,omitempty"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=rabbitmq
	// RabbitMQ instance name
	// Needed to request a transportURL that is created and used in Keystone
	RabbitMqClusterName string `json:"rabbitMqClusterName"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// TLS - Parameters related to the TLS
	TLS tls.API `json:"tls,omitempty"`

	// +kubebuilder:validation:Optional
	// +OIDCFederation - parameters to configure keystone for OIDC federation
	OIDCFederation *KeystoneFederationSpec `json:"oidcFederation,omitempty"`
}

// APIOverrideSpec to override the generated manifest of several child resources.
type APIOverrideSpec struct {
	// Override configuration for the Service created to serve traffic to the cluster.
	// The key must be the endpoint type (public, internal)
	Service map[service.Endpoint]service.RoutedOverrideSpec `json:"service,omitempty"`
}

// PasswordSelector to identify the DB and AdminUser password from the Secret
type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="AdminPassword"
	// Admin - Selector to get the keystone Admin password from the Secret
	Admin string `json:"admin"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="KeystoneClientSecret"
	// OIDCClientSecret - Selector to get the IdP client secret from the Secret
	KeystoneOIDCClientSecret string `json:"keystoneOIDCClientSecret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="KeystoneCryptoPassphrase"
	// OIDCCryptoPassphrase - Selector to get the OIDC crypto passphrase from the Secret
	KeystoneOIDCCryptoPassphrase string `json:"keystoneOIDCCryptoPassphrase"`
}

// KeystoneFederationSpec to provide the configuration values for OIDC Federation
type KeystoneFederationSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:default="OIDC-"
	// OIDCClaimPrefix
	OIDCClaimPrefix string `json:"oidcClaimPrefix"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default="id_token"
	// OIDCResponseType
	OIDCResponseType string `json:"oidcResponseType"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default="openid email profile"
	// OIDCScope
	OIDCScope string `json:"oidcScope"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=""
	// OIDCProviderMetadataURL
	OIDCProviderMetadataURL string `json:"oidcProviderMetadataURL"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=""
	// OIDCIntrospectionEndpoint
	OIDCIntrospectionEndpoint string `json:"oidcIntrospectionEndpoint"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=""
	// OIDCClientID
	OIDCClientID string `json:"oidcClientID"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=";"
	// OIDCClaimDelimiter
	OIDCClaimDelimiter string `json:"oidcClaimDelimiter"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default="claims"
	// OIDCPassUserInfoAs
	OIDCPassUserInfoAs string `json:"oidcPassUserInfoAs"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default="both"
	// OIDCPassClaimsAs
	OIDCPassClaimsAs string `json:"oidcPassClaimsAs"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default="memcache"
	// OIDCCacheType
	OIDCCacheType string `json:"oidcCacheType"`

	// +kubebuilder:validaton:Required
	// OIDCMemCacheServers
	OIDCMemCacheServers string `json:"oidcMemCacheServers"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default="HTTP_OIDC_ISS"
	// RemoteIDAttribute
	RemoteIDAttribute string `json:"remoteIDAttribute"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=""
	// KeystoneFederationIdentityProviderName
	KeystoneFederationIdentityProviderName string `json:"keystoneFederationIdentityProviderName"`
}

// HttpdCustomization - customize the httpd service
type HttpdCustomization struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// ProcessNumber - Number of processes running in keystone API
	ProcessNumber *int32 `json:"processNumber"`
}

// KeystoneAPIStatus defines the observed state of KeystoneAPI
type KeystoneAPIStatus struct {
	// ReadyCount of keystone API instances
	ReadyCount int32 `json:"readyCount,omitempty"`

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// API endpoint
	APIEndpoints map[string]string `json:"apiEndpoints,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// Keystone Database Hostname
	DatabaseHostname string `json:"databaseHostname,omitempty"`

	// NetworkAttachments status of the deployment pods
	NetworkAttachments map[string][]string `json:"networkAttachments,omitempty"`

	// TransportURLSecret - Secret containing RabbitMQ transportURL
	TransportURLSecret string `json:"transportURLSecret,omitempty"`

	// ObservedGeneration - the most recent generation observed for this service. If the observed generation is less than the spec generation, then the controller has not processed the latest changes.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="NetworkAttachments",type="string",JSONPath=".spec.networkAttachments",description="NetworkAttachments"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

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

// GetEndpoint - returns OpenStack endpoint url for type
func (instance KeystoneAPI) GetEndpoint(endpointType endpoint.Endpoint) (string, error) {
	if url, found := instance.Status.APIEndpoints[string(endpointType)]; found {
		return url, nil
	}
	return "", fmt.Errorf("%s endpoint not found", string(endpointType))
}

// IsReady - returns true if KeystoneAPI is reconciled successfully
func (instance KeystoneAPI) IsReady() bool {
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

// RbacConditionsSet - set the conditions for the rbac object
func (instance KeystoneAPI) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance KeystoneAPI) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance KeystoneAPI) RbacResourceName() string {
	return "keystone-" + instance.Name
}

// SetupDefaults - initializes any CRD field defaults based on environment variables (the defaulting mechanism itself is implemented via webhooks)
func SetupDefaults() {
	// Acquire environmental defaults and initialize Keystone defaults with them
	keystoneDefaults := KeystoneAPIDefaults{
		ContainerImageURL: util.GetEnvVar("RELATED_IMAGE_KEYSTONE_API_IMAGE_URL_DEFAULT", KeystoneAPIContainerImage),
	}

	SetupKeystoneAPIDefaults(keystoneDefaults)
}
