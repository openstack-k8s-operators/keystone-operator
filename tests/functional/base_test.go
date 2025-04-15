/*
Copyright 2023.

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

package functional_test

import (
	"fmt"

	. "github.com/onsi/gomega" //revive:disable:dot-imports

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	keystone_base "github.com/openstack-k8s-operators/keystone-operator/pkg/keystone"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetKeystoneAPISpec(fernetMaxKeys int32) map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance":    "openstack",
		"replicas":            1,
		"secret":              SecretName,
		"databaseAccount":     AccountName,
		"fernetMaxActiveKeys": fernetMaxKeys,
	}
}

func GetDefaultKeystoneAPISpec() map[string]interface{} {
	return GetKeystoneAPISpec(5)
}

func GetTLSKeystoneAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance": "openstack",
		"replicas":         1,
		"secret":           SecretName,
		"databaseAccount":  AccountName,
		"tls": map[string]interface{}{
			"api": map[string]interface{}{
				"internal": map[string]interface{}{
					"secretName": InternalCertSecretName,
				},
				"public": map[string]interface{}{
					"secretName": PublicCertSecretName,
				},
			},
			"caBundleSecretName": CABundleSecretName,
		},
	}
}

func CreateKeystoneAPI(name types.NamespacedName, spec map[string]interface{}) client.Object {

	raw := map[string]interface{}{
		"apiVersion": "keystone.openstack.org/v1beta1",
		"kind":       "KeystoneAPI",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetKeystoneAPI(name types.NamespacedName) *keystonev1.KeystoneAPI {
	instance := &keystonev1.KeystoneAPI{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func CreateKeystoneAPISecret(namespace string, name string) *corev1.Secret {
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"AdminPassword": []byte("12345678"),
		},
	)
}

func KeystoneConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetKeystoneAPI(name)
	return instance.Status.Conditions
}

func GetCronJob(name types.NamespacedName) *batchv1.CronJob {
	instance := &batchv1.CronJob{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func CreateKeystoneMessageBusSecret(namespace string, name string) *corev1.Secret {
	s := th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"transport_url": []byte(fmt.Sprintf("rabbit://%s/fake", name)),
		},
	)
	logger.Info("Secret created", "name", name)
	return s
}

// GetSampleTopologySpec - A sample (and opinionated) Topology Spec used to
// test KeystoneAPI
// Note this is just an example that should not be used in production for
// multiple reasons:
// 1. It uses ScheduleAnyway as strategy, which is something we might
// want to avoid by default
// 2. Usually a topologySpreadConstraints is used to take care about
// multi AZ, which is not applicable in this context
func GetSampleTopologySpec(label string) (map[string]interface{}, []corev1.TopologySpreadConstraint) {
	// Build the topology Spec
	topologySpec := map[string]interface{}{
		"topologySpreadConstraints": []map[string]interface{}{
			{
				"maxSkew":           1,
				"topologyKey":       corev1.LabelHostname,
				"whenUnsatisfiable": "ScheduleAnyway",
				"labelSelector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"service":   keystone_base.ServiceName,
						"component": label,
					},
				},
			},
		},
	}
	// Build the topologyObj representation
	topologySpecObj := []corev1.TopologySpreadConstraint{
		{
			MaxSkew:           1,
			TopologyKey:       corev1.LabelHostname,
			WhenUnsatisfiable: corev1.ScheduleAnyway,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"service":   keystone_base.ServiceName,
					"component": label,
				},
			},
		},
	}
	return topologySpec, topologySpecObj
}

// GetExtraMounts - Utility function that simulates extraMounts pointing
// to a  secret
func GetExtraMounts(kemName string, kemPath string) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":   kemName,
			"region": "az0",
			"extraVol": []map[string]interface{}{
				{
					"extraVolType": kemName,
					"propagation": []string{
						"Keystone",
					},
					"volumes": []map[string]interface{}{
						{
							"name": kemName,
							"secret": map[string]interface{}{
								"secretName": kemName,
							},
						},
					},
					"mounts": []map[string]interface{}{
						{
							"name":      kemName,
							"mountPath": kemPath,
							"readOnly":  true,
						},
					},
				},
			},
		},
	}
}
