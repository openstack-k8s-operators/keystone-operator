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
	"fmt"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
	corev1 "k8s.io/api/core/v1"
)

// getVolumes - service volumes
func getVolumes(
	instance *keystonev1.KeystoneAPI,
	extraVol []keystonev1.KeystoneExtraMounts,
	svc []storage.PropagationType,
) []corev1.Volume {
	name := instance.Name
	var scriptsVolumeDefaultMode int32 = 0755
	var config0640AccessMode int32 = 0644

	fernetKeys := []corev1.KeyToPath{}
	numberKeys := int(*instance.Spec.FernetMaxActiveKeys)

	for i := 0; i < numberKeys; i++ {
		fernetKeys = append(
			fernetKeys,
			corev1.KeyToPath{
				Key:  fmt.Sprintf("FernetKeys%d", i),
				Path: fmt.Sprintf("%d", i),
			},
		)
	}

	res := []corev1.Volume{
		{
			Name: "scripts",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &scriptsVolumeDefaultMode,
					SecretName:  name + "-scripts",
				},
			},
		},
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0640AccessMode,
					SecretName:  name + "-config-data",
				},
			},
		},
		{
			Name: "fernet-keys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: ServiceName,
					Items:      fernetKeys,
				},
			},
		},
		{
			Name: "credential-keys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: ServiceName,
					Items: []corev1.KeyToPath{
						{
							Key:  "CredentialKeys0",
							Path: "0",
						},
						{
							Key:  "CredentialKeys1",
							Path: "1",
						},
					},
				},
			},
		},
	}
	for _, exv := range extraVol {
		for _, vol := range exv.Propagate(svc) {
			for _, v := range vol.Volumes {
				volumeSource, _ := v.ToCoreVolumeSource()
				convertedVolume := corev1.Volume{
					Name:         v.Name,
					VolumeSource: *volumeSource,
				}
				res = append(res, convertedVolume)
			}
		}
	}
	return res
}

// getVolumeMounts - general VolumeMounts
func getVolumeMounts(
	extraVol []keystonev1.KeystoneExtraMounts,
	svc []storage.PropagationType,
) []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		{
			Name:      "scripts",
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/var/lib/config-data/default",
			ReadOnly:  false,
		},
		{
			Name:      "config-data",
			MountPath: "/var/lib/kolla/config_files/config.json",
			SubPath:   "keystone-api-config.json",
			ReadOnly:  true,
		},
		{
			MountPath: "/var/lib/fernet-keys",
			ReadOnly:  true,
			Name:      "fernet-keys",
		},
		{
			MountPath: "/var/lib/credential-keys",
			ReadOnly:  true,
			Name:      "credential-keys",
		},
	}
	for _, exv := range extraVol {
		for _, vol := range exv.Propagate(svc) {
			vm = append(vm, vol.Mounts...)
		}
	}
	return vm
}

// getCronJobVolumeMounts - cronjob volumeMounts
func getCronJobVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/etc/keystone/keystone.conf",
			SubPath:   "keystone.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
		{
			Name:      "fernet-keys",
			MountPath: "/etc/keystone/fernet-keys",
			ReadOnly:  true,
		},
	}
}

// getDBSyncVolumeMounts - cronjob volumeMounts
func getDBSyncVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/etc/keystone/keystone.conf",
			SubPath:   "keystone.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
	}
}
