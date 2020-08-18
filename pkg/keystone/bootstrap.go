package keystone

import (
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type bootstrapOptions struct {
	APIEndpoint string
	ServiceName string
}

// BootstrapJob func
func BootstrapJob(cr *keystonev1beta1.KeystoneAPI, configMapName string, APIEndpoint string) *batchv1.Job {

	// NOTE: as a convention the configmap is name the same as the service
	opts := bootstrapOptions{APIEndpoint, configMapName}
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
					ServiceAccountName: "keystone",

					InitContainers: []corev1.Container{
						{
							Name:    "keystone-secrets",
							Image:   cr.Spec.ContainerImage,
							Command: []string{"/bin/sh", "-c", util.ExecuteTemplateFile("password_init.sh", nil)},
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
					Containers: []corev1.Container{
						{
							Name:    configMapName + "-bootstrap",
							Image:   cr.Spec.ContainerImage,
							Command: []string{"/bin/bash", "-c", util.ExecuteTemplateFile("bootstrap.sh", &opts)},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: []corev1.EnvVar{
								{
									Name:  "KOLLA_CONFIG_STRATEGY",
									Value: "COPY_ALWAYS",
								},
							},
							VolumeMounts: getDbVolumeMounts(),
						},
					},
				},
			},
		},
	}
	job.Spec.Template.Spec.Volumes = getVolumes(configMapName)
	return job
}
