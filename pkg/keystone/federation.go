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
	corev1 "k8s.io/api/core/v1"
	"path/filepath"
	"strconv"
)

// getFederationVolumeMounts - get federation mountpoints
func getFederationVolumeMounts(
	federationMountPath string,
	federationFilenames []string,
) []corev1.VolumeMount {
	vms := []corev1.VolumeMount{}

	for index, filename := range federationFilenames {
		vm := corev1.VolumeMount{
			Name:      "federation-realm-volume" + strconv.Itoa(index),
			MountPath: filepath.Join(federationMountPath, filename),
			SubPath:   strconv.Itoa(index),
			ReadOnly:  true,
		}
		vms = append(vms, vm)
	}

	return vms
}

// getFederationVolumes - get federation volumes
func getFederationVolumes(
	federationFilenames []string,
) []corev1.Volume {
	var config0640AccessMode int32 = 0644
	vols := []corev1.Volume{}

	for index := range federationFilenames {
		vol := corev1.Volume{
			Name: "federation-realm-volume" + strconv.Itoa(index),
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0640AccessMode,
					SecretName:  FederationMultiRealmSecret,
				},
			},
		}
		vols = append(vols, vol)
	}
	return vols
}
