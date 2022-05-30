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
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/lib-common/pkg/common"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StatefulSet func
func StatefulSet(
	instance *keystonev1beta1.KeystoneAPI,
	configHash string,
	labels map[string]string,
) *appsv1.StatefulSet {
	runAsUser := int64(0)

	args := []string{"-c"}
	if instance.Spec.Debug.Service {
		args = append(args, DebugCommand)
	} else {
		args = append(args, ServiceCommand)
	}

	envVars := map[string]common.EnvSetter{}
	envVars["KOLLA_CONFIG_FILE"] = common.EnvValue(KollaConfig)
	envVars["KOLLA_CONFIG_STRATEGY"] = common.EnvValue("COPY_ALWAYS")
	envVars["CONFIG_HASH"] = common.EnvValue(configHash)

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: &instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: ServiceAccount,
					Containers: []corev1.Container{
						{
							Name: ServiceName + "-api",
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
	statefulSet.Spec.Template.Spec.Volumes = getVolumes(instance.Name)
	// If possible two pods of the same service should not
	// run on the same worker node. If this is not possible
	// the get still created on the same worker node.
	statefulSet.Spec.Template.Spec.Affinity = common.DistributePods(
		AppSelector,
		[]string{
			ServiceName,
		},
		corev1.LabelHostname,
	)
	if instance.Spec.NodeSelector != nil && len(instance.Spec.NodeSelector) > 0 {
		statefulSet.Spec.Template.Spec.NodeSelector = instance.Spec.NodeSelector
	}

	initContainerDetails := APIDetails{
		ContainerImage: instance.Spec.ContainerImage,
		DatabaseHost:   instance.Status.DatabaseHostname,
		DatabaseUser:   instance.Spec.DatabaseUser,
		DatabaseName:   DatabaseName,
		OSPSecret:      instance.Spec.Secret,
		VolumeMounts:   getInitVolumeMounts(),
	}
	statefulSet.Spec.Template.Spec.InitContainers = initContainer(initContainerDetails)

	return statefulSet
}
