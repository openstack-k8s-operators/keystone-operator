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
	corev1 "k8s.io/api/core/v1"
)

var (
	scriptsVolumeDefaultMode int32 = 0755
	config0640AccessMode     int32 = 0640
)

// getVolumes - service volumes
func getVolumes(instance *keystonev1.KeystoneAPI) []corev1.Volume {
	name := instance.Name

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

	vm := []corev1.Volume{
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

	domainConfig, _ := GetDomainSecretVolumes(instance.Spec.DomainConfigSecret)
	vm = append(vm, domainConfig...)
	return vm
}

// getVolumeMounts - general VolumeMounts
func getVolumeMounts(instance *keystonev1.KeystoneAPI) []corev1.VolumeMount {
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

	_, domainConfig := GetDomainSecretVolumes(instance.Spec.DomainConfigSecret)
	vm = append(vm, domainConfig...)
	return vm
}

// GetDomainSecretVolumes - Returns a potentially empty list, with a volume containing domain configuration which can be used for domain-specific LDAP backends
func GetDomainSecretVolumes(secretName string) ([]corev1.Volume, []corev1.VolumeMount) {
	secretVolumes := []corev1.Volume{}
	secretMounts := []corev1.VolumeMount{}

	if secretName != "" {
		secretVol := corev1.Volume{
			Name: secretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  secretName,
					DefaultMode: &config0640AccessMode,
				},
			},
		}
		secretMount := corev1.VolumeMount{
			Name:      secretName,
			MountPath: "/etc/keystone/domains",
			ReadOnly:  true,
		}
		secretVolumes = append(secretVolumes, secretVol)
		secretMounts = append(secretMounts, secretMount)
	}

	return secretVolumes, secretMounts
}
