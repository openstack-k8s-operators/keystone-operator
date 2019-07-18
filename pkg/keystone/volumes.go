package keystone

import (
	corev1 "k8s.io/api/core/v1"
)

// add common keystone API volumes
func getVolumes(name string) []corev1.Volume {

	return []corev1.Volume{

		{
			Name: "kolla-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "config.json",
							Path: "config.json",
						},
					},
				},
			},
		},
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "keystone.conf",
							Path: "keystone.conf",
						},
						{
							Key:  "logging.conf",
							Path: "logging.conf",
						},
						{
							Key:  "httpd.conf",
							Path: "httpd.conf",
						},
					},
				},
			},
		},
		{
			Name: "fernet-keys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: name,
				},
			},
		},
	}

}
