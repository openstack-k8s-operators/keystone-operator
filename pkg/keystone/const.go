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

const (
	// ServiceName -
	ServiceName = "keystone"
	// ServiceAccount -
	ServiceAccount = "keystone-operator-keystone"
	// DatabaseName -
	DatabaseName = "keystone"
	// KeystoneDatabasePassword - ref name to get the keystone db password from osp-secret
	KeystoneDatabasePassword = "KeystoneDatabasePassword"
	// AdminPassword - ref name to get the admin password from osp-secret
	AdminPassword = "AdminPassword"

	// InputHashName -Name of the hash of hashes of all resources used to indentify an input change
	InputHashName = "input"

	// KeystoneAdminPort -
	KeystoneAdminPort int32 = 35357
	// KeystonePublicPort -
	KeystonePublicPort int32 = 5000
	// KeystoneInternalPort -
	KeystoneInternalPort int32 = 5000

	// AppSelector -
	AppSelector = "app"

	// KollaConfig -
	KollaConfig = "/var/lib/config-data/merged/keystone-api-config.json"

	// InitContainerCommand -
	InitContainerCommand = "/usr/local/bin/container-scripts/init.sh"
	// DBSyncCommand -
	DBSyncCommand = "/usr/local/bin/kolla_set_configs && /usr/local/bin/kolla_start"
	// BootStrapCommand -
	BootStrapCommand = "/usr/local/bin/kolla_set_configs && keystone-manage bootstrap"
	// ServiceCommand -
	ServiceCommand = "/usr/local/bin/kolla_set_configs && /usr/local/bin/kolla_start"
	// DebugCommand -
	DebugCommand = "/usr/local/bin/kolla_set_configs && /bin/sleep infinity"
)
