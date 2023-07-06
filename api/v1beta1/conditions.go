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
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

// Keystone Condition Types used by API objects.
const (
	// KeystoneAPIReadyCondition Status=True condition which indicates if the KeystoneAPI is configured and operational
	KeystoneAPIReadyCondition condition.Type = "KeystoneAPIReady"

	// AdminServiceClientReadyCondition Status=True condition which indicates if the admin client is ready and can be used
	AdminServiceClientReadyCondition condition.Type = "AdminServiceClientReady"

	// KeystoneServiceOSServiceReadyCondition Status=True condition which indicates if the service created in the keystone instance is ready/was successful
	KeystoneServiceOSServiceReadyCondition condition.Type = "KeystoneServiceOSServiceReady"

	// KeystoneServiceOSEndpointsReadyCondition Status=True condition which indicates if the endpoints got created in the keystone instance is ready/was successful
	KeystoneServiceOSEndpointsReadyCondition condition.Type = "KeystoneServiceOSEndpointsReady"

	// KeystoneServiceOSUserReadyCondition Status=True condition which indicates if the service user got created in the keystone instance is ready/was successful
	KeystoneServiceOSUserReadyCondition condition.Type = "KeystoneServiceOSUserReady"

	// KeystoneMemcachedReadyCondition - Indicates the Keystone memcached service is ready to be consumed by Keystone
	KeystoneMemcachedReadyCondition condition.Type = "KeystoneMemcachedReady"
)

// Common Messages used by API objects.
const (

	//
	// KeystoneAPIReady condition messages
	//
	// KeystoneAPIReadyInitMessage
	KeystoneAPIReadyInitMessage = "KeystoneAPI not started"

	// KeystoneAPIReadyMessage
	KeystoneAPIReadyMessage = "KeystoneAPI ready"

	// KeystoneAPIReadyNotFoundMessage
	KeystoneAPIReadyNotFoundMessage = "KeystoneAPI not found"

	// KeystoneAPIReadyWaitingMessage
	KeystoneAPIReadyWaitingMessage = "KeystoneAPI not yet ready"

	// KeystoneAPIReadyErrorMessage
	KeystoneAPIReadyErrorMessage = "KeystoneAPI error occured %s"

	//
	// AdminServiceClientReady condition messages
	//
	// AdminServiceClientReadyInitMessage
	AdminServiceClientReadyInitMessage = "Admin client not started"

	// AdminServiceClientReadyMessage
	AdminServiceClientReadyMessage = "Admin client ready"

	// AdminServiceClientReadyWaitingMessage
	AdminServiceClientReadyWaitingMessage = "Admin client not yet ready"

	// AdminServiceClientReadyErrorMessage
	AdminServiceClientReadyErrorMessage = "Admin client error occured %s"

	//
	// KeystoneServiceOSServiceReady condition messages
	//
	// KeystoneServiceOSServiceReadyInitMessage
	KeystoneServiceOSServiceReadyInitMessage = "Keystone Service registration not started"

	// KeystoneServiceOSServiceReadyMessage
	KeystoneServiceOSServiceReadyMessage = "Keystone Service %s - %s ready"

	// AdminServiceClientReadyErrorMessage
	KeystoneServiceOSServiceReadyErrorMessage = "Keystone Service error occured %s"

	//
	// KeystoneServiceOSEndpointsReady condition messages
	//
	// KeystoneServiceOSEndpointsReadyInitMessage
	KeystoneServiceOSEndpointsReadyInitMessage = "Keystone Endpoints registration not started"

	// KeystoneServiceOSEndpointsReadyMessage
	KeystoneServiceOSEndpointsReadyMessage = "Keystone Endpoints ready: %+v"

	// KeystoneServiceOSEndpointsReadyErrorMessage
	KeystoneServiceOSEndpointsReadyErrorMessage = "Keystone Endpoints error occured %s"

	//
	// KeystoneServiceOSUserReady condition messages
	//
	// KeystoneServiceOSUserReadyInitMessage
	KeystoneServiceOSUserReadyInitMessage = "Keystone Service user registration not started"

	// KeystoneServiceOSUserReadyMessage
	KeystoneServiceOSUserReadyMessage = "Keystone Service user %s ready"

	// KeystoneServiceOSUserReadyWaitingMessage
	KeystoneServiceOSUserReadyWaitingMessage = "Keystone Service user not yet ready"

	// KeystoneServiceOSUserReadyErrorMessage
	KeystoneServiceOSUserReadyErrorMessage = "Keystone Service user error occured %s"

	//
	// KeystoneMemcachedReady condition messages
	//
	// KeystoneMemcachedReadyInitMessage -
	KeystoneMemcachedReadyInitMessage = "Keystone Memcached create not started"

	// KeystoneMemcachedReadyMessage - Provides the message to clarify memcached has been provisioned
	KeystoneMemcachedReadyMessage = "Keystone Memcached instance has been provisioned"

	// KeystoneMemcachedReadyWaitingMessage - Provides the message to clarify memcached has not been provisioned
	KeystoneMemcachedReadyWaitingMessage = "Keystone Memcached instance has not been provisioned"

	// KeystoneMemcachedReadyErrorMessage -
	KeystoneMemcachedReadyErrorMessage = "Keystone Memcached error occurred %s"
)
