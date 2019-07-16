package keystone

import (
        comv1 "github.com/openstack-k8s-operators/keystone-operator/pkg/apis/keystone/v1"
        util "github.com/openstack-k8s-operators/keystone-operator/pkg/util"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type bootstrapOptions struct {
        AdminPassword string
}

func BootstrapJob(cr *comv1.KeystoneApi, configMapName string) *batchv1.Job {

        opts := bootstrapOptions{cr.Spec.AdminPassword}
	runAsUser := int64(0)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName + "-bootstrap",
			Namespace: cr.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      "OnFailure",
					ServiceAccountName: "keystone-operator",
					Containers: []corev1.Container{
						{
							Name:  "keystone-bootstrap",
							Image: cr.Spec.ContainerImage,
                                                        Command: []string{"/bin/bash", "-c", util.ExecuteTemplateFile("config/bootstrap.sh", &opts)},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: []corev1.EnvVar{
								{
									Name:  "KOLLA_CONFIG_STRATEGY",
									Value: "COPY_ALWAYS",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
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
							},
						},
					},
				},
			},
		},
	}
        job.Spec.Template.Spec.Volumes = getVolumes(configMapName)
	return job
}
