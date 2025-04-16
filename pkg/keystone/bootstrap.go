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

// Package keystone contains keystone service bootstrap functionality.
package keystone

import (
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// BootstrapCommand -
	BootstrapCommand = "/usr/local/bin/kolla_set_configs && keystone-manage bootstrap"
)

// BootstrapJob func
func BootstrapJob(
	instance *keystonev1.KeystoneAPI,
	labels map[string]string,
	annotations map[string]string,
	endpoints map[string]string,
	memcached *memcachedv1.Memcached,
) *batchv1.Job {
	runAsUser := int64(0)

	args := []string{"-c", BootstrapCommand}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = env.SetValue("true")
	envVars["OS_BOOTSTRAP_USERNAME"] = env.SetValue(instance.Spec.AdminUser)
	envVars["OS_BOOTSTRAP_PROJECT_NAME"] = env.SetValue(instance.Spec.AdminProject)
	envVars["OS_BOOTSTRAP_SERVICE_NAME"] = env.SetValue(ServiceName)
	envVars["OS_BOOTSTRAP_REGION_ID"] = env.SetValue(instance.Spec.Region)

	if _, ok := endpoints["admin"]; ok {
		envVars["OS_BOOTSTRAP_ADMIN_URL"] = env.SetValue(endpoints["admin"])
	}
	if _, ok := endpoints["internal"]; ok {
		envVars["OS_BOOTSTRAP_INTERNAL_URL"] = env.SetValue(endpoints["internal"])
	}
	if _, ok := endpoints["public"]; ok {
		envVars["OS_BOOTSTRAP_PUBLIC_URL"] = env.SetValue(endpoints["public"])
	}

	// create Volume and VolumeMounts
	bootstrapExtraMounts := []keystonev1.KeystoneExtraMounts{}
	volumes := getVolumes(instance, bootstrapExtraMounts, BootstrapPropagation)
	volumeMounts := getVolumeMounts(bootstrapExtraMounts, BootstrapPropagation)

	// add CA cert if defined
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	// add MTLS cert if defined
	if memcached.GetMemcachedMTLSSecret() != "" {
		volumes = append(volumes, memcached.CreateMTLSVolume())
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      *memcached.Spec.TLS.MTLS.AuthCertSecret.SecretName,
			MountPath: "/etc/pki/tls/certs/mtls.crt",
			SubPath:   tls.CertKey,
			ReadOnly:  true,
		}, corev1.VolumeMount{
			Name:      *memcached.Spec.TLS.MTLS.AuthCertSecret.SecretName,
			MountPath: "/etc/pki/tls/private/mtls.key",
			SubPath:   tls.PrivateKey,
			ReadOnly:  true,
		}, corev1.VolumeMount{
			Name:      *memcached.Spec.TLS.MTLS.AuthCertSecret.SecretName,
			MountPath: "/etc/pki/tls/certs/mtls-ca.crt",
			SubPath:   tls.CAKey,
			ReadOnly:  true,
		})
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName + "-bootstrap",
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					ServiceAccountName: instance.RbacResourceName(),
					Containers: []corev1.Container{
						{
							Name:  ServiceName + "-bootstrap",
							Image: instance.Spec.ContainerImage,
							Command: []string{
								"/bin/bash",
							},
							Args: args,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: []corev1.EnvVar{
								{
									Name: "OS_BOOTSTRAP_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: instance.Spec.Secret,
											},
											Key: "AdminPassword",
										},
									},
								},
							},
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}
	job.Spec.Template.Spec.Containers[0].Env = env.MergeEnvs(job.Spec.Template.Spec.Containers[0].Env, envVars)

	if instance.Spec.NodeSelector != nil {
		job.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}

	return job
}
