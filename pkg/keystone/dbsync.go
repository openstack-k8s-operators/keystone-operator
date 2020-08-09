package keystone

import (
	comv1 "github.com/openstack-k8s-operators/keystone-operator/pkg/apis/keystone/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DbSyncJob func
func DbSyncJob(cr *comv1.KeystoneAPI, cmName string) *batchv1.Job {

	runAsUser := int64(0)

	labels := map[string]string{
		"app": "keystone-api",
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName + "-db-sync",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      "OnFailure",
					ServiceAccountName: "keystone",
					Containers: []corev1.Container{
						{
							Name:  "keystone-db-sync",
							Image: cr.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: []corev1.EnvVar{
								{
									Name:  "KOLLA_CONFIG_STRATEGY",
									Value: "COPY_ALWAYS",
								},
								{
									Name:  "KOLLA_BOOTSTRAP",
									Value: "TRUE",
								},
							},
							VolumeMounts: getVolumeMounts(),
						},
					},
				},
			},
		},
	}
	job.Spec.Template.Spec.Volumes = getVolumes(cmName)
	return job
}
