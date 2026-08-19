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
	"github.com/openstack-k8s-operators/lib-common/modules/common/volume"
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
	var configAccessMode int32 = 0440

	fernetKeys := []corev1.KeyToPath{}
	numberKeys := int(*instance.Spec.FernetMaxActiveKeys)

	for i := range numberKeys {
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
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &configAccessMode,
					SecretName:  name + "-config-data",
				},
			},
		},
		{
			Name: "fernet-keys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &configAccessMode,
					SecretName:  ServiceName,
					Items:       fernetKeys,
				},
			},
		},
		{
			Name: "credential-keys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &configAccessMode,
					SecretName:  ServiceName,
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
		volume.WritableDirVolume(volume.TmpVolumeName),
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

// getVolumeMounts - API deployment VolumeMounts
func getVolumeMounts(
	extraVol []keystonev1.KeystoneExtraMounts,
	svc []storage.PropagationType,
) []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/etc/keystone/keystone.conf",
			SubPath:   "keystone.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/keystone/keystone.conf.d/custom.conf",
			SubPath:   "custom.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/httpd/conf/httpd.conf",
			SubPath:   "httpd.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/httpd/conf.d/ssl.conf",
			SubPath:   "ssl.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
		{
			MountPath: "/etc/keystone/fernet-keys",
			ReadOnly:  true,
			Name:      "fernet-keys",
		},
		{
			MountPath: "/etc/keystone/credential-keys",
			ReadOnly:  true,
			Name:      "credential-keys",
		},
		volume.WritableDirVolumeMount(volume.RunHttpdVolumeName, volume.RunHttpdMountPath),
		volume.WritableDirVolumeMount(volume.TmpVolumeName, volume.TmpMountPath),
		volume.WritableDirVolumeMount(VarLogKeystoneVolumeName, "/var/log/keystone"),
		volume.WritableDirVolumeMount(volume.VarLogHttpdVolumeName, volume.VarLogHttpdMountPath),
	}
	for _, exv := range extraVol {
		for _, vol := range exv.Propagate(svc) {
			vm = append(vm, vol.Mounts...)
		}
	}
	return vm
}

// getBootstrapVolumeMounts - bootstrap job VolumeMounts
func getBootstrapVolumeMounts() []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/etc/keystone/keystone.conf",
			SubPath:   "keystone.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/keystone/keystone.conf.d/custom.conf",
			SubPath:   "custom.conf",
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
		{
			Name:      "credential-keys",
			MountPath: "/etc/keystone/credential-keys",
			ReadOnly:  true,
		},
		volume.WritableDirVolumeMount(volume.TmpVolumeName, volume.TmpMountPath),
	}
	return vm
}

// getCronJobVolumeMounts - cronjob volumeMounts
func getCronJobVolumeMounts() []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
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
		volume.WritableDirVolumeMount(volume.TmpVolumeName, volume.TmpMountPath),
	}
	return vm
}

// getDBSyncVolumeMounts - db-sync job volumeMounts
func getDBSyncVolumeMounts() []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
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
		volume.WritableDirVolumeMount(volume.TmpVolumeName, volume.TmpMountPath),
	}
	return vm
}
