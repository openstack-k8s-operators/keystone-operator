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

	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"

	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/annotations"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
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
	instance *keystonev1beta1.KeystoneAPI,
	labels map[string]string,
	endpoints map[string]string,
) (*batchv1.Job, error) {
	runAsUser := int64(0)

	args := []string{"-c"}
	if instance.Spec.Debug.Bootstrap {
		args = append(args, common.DebugCommand)
	} else {
		args = append(args, BootstrapCommand)
	}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_FILE"] = env.SetValue(KollaConfig)
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = env.SetValue("true")
	envVars["OS_BOOTSTRAP_USERNAME"] = env.SetValue(instance.Spec.AdminUser)
	envVars["OS_BOOTSTRAP_PROJECT_NAME"] = env.SetValue(instance.Spec.AdminProject)
	envVars["OS_BOOTSTRAP_ROLE_NAME"] = env.SetValue(instance.Spec.AdminRole)
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

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName + "-bootstrap",
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
	job.Spec.Template.Spec.Containers[0].Env = env.MergeEnvs(job.Spec.Template.Spec.Containers[0].Env, envVars)
	job.Spec.Template.Spec.Volumes = getVolumes(instance.Name)

	// networks to attach to
	nwAnnotation, err := annotations.GetNADAnnotation(instance.Namespace, instance.Spec.NetworkAttachments)
	if err != nil {
		return nil, fmt.Errorf("failed create network annotation from %s: %w",
			instance.Spec.NetworkAttachments, err)
	}
	job.Spec.Template.Annotations = util.MergeStringMaps(job.Spec.Template.Annotations, nwAnnotation)

	initContainerDetails := APIDetails{
		ContainerImage:       instance.Spec.ContainerImage,
		DatabaseHost:         instance.Status.DatabaseHostname,
		DatabaseUser:         instance.Spec.DatabaseUser,
		DatabaseName:         DatabaseName,
		OSPSecret:            instance.Spec.Secret,
		DBPasswordSelector:   instance.Spec.PasswordSelectors.Database,
		UserPasswordSelector: instance.Spec.PasswordSelectors.Admin,
		VolumeMounts:         getInitVolumeMounts(),
	}
	job.Spec.Template.Spec.InitContainers = initContainer(initContainerDetails)

	return job, nil
}
