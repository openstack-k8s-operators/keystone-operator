package keystone

import (
	corev1 "k8s.io/api/core/v1"
)

// common Keystone API Volumes
func getVolumes(name string) []corev1.Volume {

	return []corev1.Volume{
		{
			Name: "emptydir",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: ""},
			},
		},
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
		{
			Name: "db-kolla-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "db-sync-config.json",
							Path: "config.json",
						},
					},
				},
			},
		},
	}

}

// common Keystone API VolumeMounts
func getVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			MountPath: "/var/lib/config-data",
			ReadOnly:  true,
			Name:      "config-data",
		},
		{
			MountPath: "/var/lib/kolla/config_files",
			ReadOnly:  true,
			Name:      "kolla-config",
		},
		{
			MountPath: "/var/lib/fernet-keys",
			ReadOnly:  true,
			Name:      "fernet-keys",
		},
		{
			MountPath: "/var/lib/emptydir",
			ReadOnly:  false,
			Name:      "emptydir",
		},
	}

}

// common Keystone API VolumeMounts (db-kolla-config mounts passwords directly as keystone.conf)
func getDbVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			MountPath: "/var/lib/config-data",
			ReadOnly:  true,
			Name:      "config-data",
		},
		{
			MountPath: "/var/lib/kolla/config_files",
			ReadOnly:  true,
			Name:      "db-kolla-config",
		},
		{
			MountPath: "/var/lib/fernet-keys",
			ReadOnly:  true,
			Name:      "fernet-keys",
		},
		{
			MountPath: "/var/lib/emptydir",
			ReadOnly:  false,
			Name:      "emptydir",
		},
	}

}

// common Keystone API VolumeMounts for init/secrets container
func getInitVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			MountPath: "/var/lib/emptydir",
			ReadOnly:  false,
			Name:      "emptydir",
		},
	}
}
