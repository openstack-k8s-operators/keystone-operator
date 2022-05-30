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
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"

	common "github.com/openstack-k8s-operators/lib-common/pkg/common"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BootstrapJob func
func BootstrapJob(
	instance *keystonev1beta1.KeystoneAPI,
	endpoints map[string]string,
) *batchv1.Job {
	runAsUser := int64(0)

	args := []string{"-c"}
	if instance.Spec.Debug.Bootstrap {
		args = append(args, DebugCommand)
	} else {
		args = append(args, BootStrapCommand)
	}

	envVars := map[string]common.EnvSetter{}
	envVars["KOLLA_CONFIG_FILE"] = common.EnvValue(KollaConfig)
	envVars["KOLLA_CONFIG_STRATEGY"] = common.EnvValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = common.EnvValue("true")
	envVars["OS_BOOTSTRAP_USERNAME"] = common.EnvValue(instance.Spec.AdminUser)
	envVars["OS_BOOTSTRAP_PROJECT_NAME"] = common.EnvValue(instance.Spec.AdminProject)
	envVars["OS_BOOTSTRAP_ROLE_NAME"] = common.EnvValue(instance.Spec.AdminRole)
	envVars["OS_BOOTSTRAP_SERVICE_NAME"] = common.EnvValue(ServiceName)
	envVars["OS_BOOTSTRAP_REGION_ID"] = common.EnvValue(instance.Spec.Region)

	if _, ok := endpoints["admin"]; ok {
		envVars["OS_BOOTSTRAP_ADMIN_URL"] = common.EnvValue(endpoints["admin"])
	}
	if _, ok := endpoints["internal"]; ok {
		envVars["OS_BOOTSTRAP_INTERNAL_URL"] = common.EnvValue(endpoints["internal"])
	}
	if _, ok := endpoints["public"]; ok {
		envVars["OS_BOOTSTRAP_PUBLIC_URL"] = common.EnvValue(endpoints["public"])
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName + "-bootstrap",
			Namespace: instance.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      "OnFailure",
					ServiceAccountName: ServiceAccount,
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
							VolumeMounts: getVolumeMounts(),
						},
					},
				},
			},
		},
	}
	job.Spec.Template.Spec.Containers[0].Env = common.MergeEnvs(job.Spec.Template.Spec.Containers[0].Env, envVars)
	job.Spec.Template.Spec.Volumes = getVolumes(instance.Name)

	initContainerDetails := APIDetails{
		ContainerImage: instance.Spec.ContainerImage,
		DatabaseHost:   instance.Status.DatabaseHostname,
		DatabaseUser:   instance.Spec.DatabaseUser,
		DatabaseName:   DatabaseName,
		OSPSecret:      instance.Spec.Secret,
		VolumeMounts:   getInitVolumeMounts(),
	}
	job.Spec.Template.Spec.InitContainers = initContainer(initContainerDetails)

	return job
}

// DbSyncJob func
func DbSyncJob(
	instance *keystonev1.KeystoneAPI,
) *batchv1.Job {
	runAsUser := int64(0)

	labels := map[string]string{
		"app": "keystone-api",
	}

	args := []string{"-c"}
	if instance.Spec.Debug.DBSync {
		args = append(args, DebugCommand)
	} else {
		args = append(args, DBSyncCommand)
	}

	envVars := map[string]common.EnvSetter{}
	envVars["KOLLA_CONFIG_FILE"] = common.EnvValue(KollaConfig)
	envVars["KOLLA_CONFIG_STRATEGY"] = common.EnvValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = common.EnvValue("true")

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName + "-db-sync",
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      "OnFailure",
					ServiceAccountName: ServiceAccount,
					Containers: []corev1.Container{
						{
							Name: ServiceName + "-db-sync",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: instance.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env:          common.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts: getVolumeMounts(),
						},
					},
				},
			},
		},
	}

	job.Spec.Template.Spec.Volumes = getVolumes(ServiceName)

	initContainerDetails := APIDetails{
		ContainerImage: instance.Spec.ContainerImage,
		DatabaseHost:   instance.Status.DatabaseHostname,
		DatabaseUser:   instance.Spec.DatabaseUser,
		DatabaseName:   DatabaseName,
		OSPSecret:      instance.Spec.Secret,
		VolumeMounts:   getInitVolumeMounts(),
	}
	job.Spec.Template.Spec.InitContainers = initContainer(initContainerDetails)

	return job
}
