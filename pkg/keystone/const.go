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

package keystone

import (
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
)

const (
	// ServiceName -
	ServiceName = "keystone"
	// DatabaseName -
	DatabaseName = "keystone"
	// DatabaseCRName -
	DatabaseCRName = "keystone"
	// DatabaseUsernamePrefix -
	DatabaseUsernamePrefix = "keystone"
	// KeystonePublicPort -
	KeystonePublicPort int32 = 5000
	// KeystoneInternalPort -
	KeystoneInternalPort int32 = 5000
	// KeystoneUID is based on kolla
	// https://github.com/openstack/kolla/blob/master/kolla/common/users.py
	KeystoneUID int64 = 42425
	// DefaultFernetMaxActiveKeys -
	DefaultFernetMaxActiveKeys = 5
	// DefaultFernetRotationDays -
	DefaultFernetRotationDays = 1
	// DBSyncCommand -
	DBSyncCommand = "keystone-manage db_sync"
	// Keystone is the global ServiceType
	Keystone storage.PropagationType = "Keystone"
	// KeystoneCronJob is the CronJob ServiceType
	KeystoneCronJob storage.PropagationType = "KeystoneCron"
	// KeystoneBootstrap is the bootstrap service
	KeystoneBootstrap storage.PropagationType = "KeystoneBootstrap"
	// FederationConfigKey - key for multi-realm federation config secret
	FederationConfigKey = "federation-config.json"
	// FederationMultiRealmSecret - secret to store processed multirealm data
	FederationMultiRealmSecret = "keystone-multirealm-federation-secret"
	// FederationDefaultMountPath - if user doesn't specify otherwise, this location is used
	FederationDefaultMountPath = "/var/lib/config-data/default/multirealm-federation"
)

// KeystoneAPIPropagation is the  definition of the Horizon propagation service
var KeystonePropagation = []storage.PropagationType{Keystone}

// DBSyncPropagation keeps track of the DBSync Service Propagation Type
var DBSyncPropagation = []storage.PropagationType{storage.DBSync}

// BootstrapPropagation keeps track of the KeystoneBootstrap Propagation Type
var BootstrapPropagation = []storage.PropagationType{KeystoneBootstrap}

// KeystoneCronJobPropagation keeps track of the KeystoneBootstrap Propagation Type
var KeystoneCronJobPropagation = []storage.PropagationType{KeystoneCronJob}
