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
)
