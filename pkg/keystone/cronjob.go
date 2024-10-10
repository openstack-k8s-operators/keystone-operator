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
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// TrustFlushCommand -
	TrustFlushCommand = "/usr/local/bin/kolla_set_configs && keystone-manage trust_flush"
)

// CronJob func
func CronJob(
	instance *keystonev1.KeystoneAPI,
	labels map[string]string,
	annotations map[string]string,
) *batchv1.CronJob {
	runAsUser := int64(0)

	args := []string{"-c", TrustFlushCommand + instance.Spec.TrustFlushArgs}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")

	parallelism := int32(1)
	completions := int32(1)

	// create Volume and VolumeMounts
	volumes := getVolumes(instance)
	volumeMounts := getVolumeMounts()

	// add CA cert if defined
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(getVolumes(instance), instance.Spec.TLS.CreateVolume())
		volumeMounts = append(getVolumeMounts(), instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName + "-cron",
			Namespace: instance.Namespace,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          instance.Spec.TrustFlushSchedule,
			Suspend:           &instance.Spec.TrustFlushSuspend,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: batchv1.JobSpec{
					Parallelism: &parallelism,
					Completions: &completions,
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  ServiceName + "-cron",
									Image: instance.Spec.ContainerImage,
									Command: []string{
										"/bin/bash",
									},
									Args:         args,
									Env:          env.MergeEnvs([]corev1.EnvVar{}, envVars),
									VolumeMounts: volumeMounts,
									SecurityContext: &corev1.SecurityContext{
										RunAsUser: &runAsUser,
									},
								},
							},
							Volumes:            volumes,
							RestartPolicy:      corev1.RestartPolicyNever,
							ServiceAccountName: instance.RbacResourceName(),
						},
					},
				},
			},
		},
	}
	if len(instance.Spec.NodeSelector) > 0 {
		cronjob.Spec.JobTemplate.Spec.Template.Spec.NodeSelector = instance.Spec.NodeSelector
	}

	return cronjob
}
