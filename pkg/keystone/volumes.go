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

// getVolumes - service volumes
func getVolumes(keystoneapiinstance *keystonev1.KeystoneAPI) []corev1.Volume {
	//func getVolumes(instance *keystonev1.KeystoneAPIFernet) []corev1.Volume {
	name := keystoneapiinstance.Name
	var scriptsVolumeDefaultMode int32 = 0755
	var config0640AccessMode int32 = 0640

	fernetKeys := []corev1.KeyToPath{}
	fmt.Println("=========================")
	fmt.Println(fernetKeys)
	fmt.Println("=========================")

	fernetKeys = append(
		fernetKeys,
		corev1.KeyToPath{
			Key:  fmt.Sprintf("FernetKeys%d", 10),
			Path: fmt.Sprintf("%d", 10),
		},
	)

	fmt.Println("=========================")
	fmt.Println(fernetKeys)
	fmt.Println("=========================")
	instance := &keystonev1.KeystoneAPIFernet{KeystoneAPI: keystoneapiinstance}
	fmt.Println(instance.Spec)
	fmt.Println("=========================")
	fmt.Println(instance.Spec.FernetMaxActiveKeys)
	fmt.Println("=========================")
	//fmt.Println(*instance.Spec.FernetKeys)
	var i *int32 = new(int32)

	//for *i = 0; *i < *instance.Spec.FernetKeys; *i++ {
	for *i = 0; *i < 2; *i++ {
		fmt.Println("FOR")
		/*
			fernetKeys = append(
				fernetKeys,
				corev1.KeyToPath{
					Key:  fmt.Sprintf("FernetKeys%d", *i),
					Path: fmt.Sprintf("%d", *i),
				},
			)
		*/
	}
	fmt.Println("=========================")
	fmt.Println(fernetKeys)
	fmt.Println("=========================")

	return []corev1.Volume{
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

}

// getVolumeMounts - general VolumeMounts
func getVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
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
}
