package keystone

import (
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/lib-common/pkg/common"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DbSyncJob func
func DbSyncJob(cr *keystonev1beta1.KeystoneAPI, cmName string) (*batchv1.Job, error) {
	runAsUser := int64(0)

	passwordInitCmd, err := common.ExecuteTemplateFile("password_init.sh", nil)
	if err != nil {
		return nil, err
	}

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
					ServiceAccountName: "keystone-operator-keystone",
					Containers: []corev1.Container{
						{
							Name: "keystone-db-sync",
							//Command: []string{"/bin/sleep", "7000"},
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
					InitContainers: []corev1.Container{
						{
							Name:    "keystone-secrets",
							Image:   cr.Spec.ContainerImage,
							Command: []string{"/bin/sh", "-c", passwordInitCmd},
							Env: []corev1.EnvVar{
								{
									Name:  "DatabaseHost",
									Value: cr.Spec.DatabaseHostname,
								},
								{
									Name:  "DatabaseUser",
									Value: cr.Name,
								},
								{
									Name:  "DatabaseSchema",
									Value: cr.Name,
								},
								{
									Name: "DatabasePassword",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: cr.Spec.Secret,
											},
											Key: "DatabasePassword",
										},
									},
								},
								{
									Name: "AdminPassword",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: cr.Spec.Secret,
											},
											Key: "AdminPassword",
										},
									},
								},
							},
							VolumeMounts: getInitVolumeMounts(),
						},
					},
				},
			},
		},
	}
	job.Spec.Template.Spec.Volumes = getVolumes(cmName)
	return job, nil
}
