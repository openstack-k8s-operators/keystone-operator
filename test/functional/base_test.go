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
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	keystone_base "github.com/openstack-k8s-operators/keystone-operator/internal/keystone"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

func GetKeystoneAPISpec(fernetMaxKeys int32) map[string]any {
	return map[string]any{
		"databaseInstance":    "openstack",
		"replicas":            1,
		"secret":              SecretName,
		"databaseAccount":     AccountName,
		"fernetMaxActiveKeys": fernetMaxKeys,
	}
}

func GetDefaultKeystoneAPISpec() map[string]any {
	return GetKeystoneAPISpec(5)
}

func GetTLSKeystoneAPISpec() map[string]any {
	return map[string]any{
		"databaseInstance": "openstack",
		"replicas":         1,
		"secret":           SecretName,
		"databaseAccount":  AccountName,
		"tls": map[string]any{
			"api": map[string]any{
				"internal": map[string]any{
					"secretName": InternalCertSecretName,
				},
				"public": map[string]any{
					"secretName": PublicCertSecretName,
				},
			},
			"caBundleSecretName": CABundleSecretName,
		},
	}
}

func CreateKeystoneAPI(name types.NamespacedName, spec map[string]any) client.Object {

	raw := map[string]any{
		"apiVersion": "keystone.openstack.org/v1beta1",
		"kind":       "KeystoneAPI",
		"metadata": map[string]any{
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

// CreateKeystoneAPIInvalidSecret creates a secret with an invalid password for testing
func CreateKeystoneAPIInvalidSecret(namespace string, name string) *corev1.Secret {
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"AdminPassword": []byte("c^sometext02%text%text02$someText&"),
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
			"transport_url": fmt.Appendf(nil, "rabbit://%s/fake", name),
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
func GetSampleTopologySpec(label string) (map[string]any, []corev1.TopologySpreadConstraint) {
	// Build the topology Spec
	topologySpec := map[string]any{
		"topologySpreadConstraints": []map[string]any{
			{
				"maxSkew":           1,
				"topologyKey":       corev1.LabelHostname,
				"whenUnsatisfiable": "ScheduleAnyway",
				"labelSelector": map[string]any{
					"matchLabels": map[string]any{
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
func GetExtraMounts(kemName string, kemPath string) []map[string]any {
	return []map[string]any{
		{
			"name":   kemName,
			"region": "az0",
			"extraVol": []map[string]any{
				{
					"extraVolType": kemName,
					"propagation": []string{
						"Keystone",
					},
					"volumes": []map[string]any{
						{
							"name": kemName,
							"secret": map[string]any{
								"secretName": kemName,
							},
						},
					},
					"mounts": []map[string]any{
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

// ApplicationCredential helper functions

// Finalizer constants used across AC tests
const (
	ACCRFinalizer               = "openstack.org/applicationcredential"
	ACSecretProtectionFinalizer = "openstack.org/ac-secret-protection" //nolint:gosec
)

func CreateACWithSpec(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "keystone.openstack.org/v1beta1",
		"kind":       "KeystoneApplicationCredential",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

// GetDefaultACSpec returns a minimal, valid AC spec. Callers that need
// non-default values (roles, accessRules, etc.) should build their own map
func GetDefaultACSpec(userName, secretName string) map[string]interface{} {
	return map[string]interface{}{
		"userName":         userName,
		"secret":           secretName,
		"passwordSelector": "ServicePassword",
		"expirationDays":   30,
		"gracePeriodDays":  7,
		"roles":            []string{"service"},
	}
}

// CreateACServiceUserSecret creates the standard service-user Secret
// (key: ServicePassword) used by AC tests and returns its NamespacedName
func CreateACServiceUserSecret(namespace string) types.NamespacedName {
	name := types.NamespacedName{Name: "osp-secret", Namespace: namespace}
	th.CreateSecret(name, map[string][]byte{
		"ServicePassword": []byte("service-password"),
	})
	return name
}

// CreateProtectedACSecret creates an immutable Kubernetes Secret with the
// AC label set and the protection finalizer already applied, simulating a
// secret that was created during a previous reconcile or rotation
func CreateProtectedACSecret(name types.NamespacedName, serviceName string) *corev1.Secret {
	immutable := true
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
			Labels: map[string]string{
				"application-credentials":        "true",
				"application-credential-service": serviceName,
			},
			Finalizers: []string{ACSecretProtectionFinalizer},
		},
		Immutable: &immutable,
		Data: map[string][]byte{
			keystonev1.ACIDSecretKey:     []byte("fake-ac-id"),
			keystonev1.ACSecretSecretKey: []byte("fake-ac-secret"),
		},
	}
	Expect(k8sClient.Create(ctx, secret)).To(Succeed())
	return secret
}

// CreateOldStyleACSecret creates a mutable secret with only the
// application-credentials label (no application-credential-service label),
// simulating a secret created by the pre-immutable secret logic
func CreateOldStyleACSecret(name types.NamespacedName, acID string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
			Labels: map[string]string{
				"application-credentials": "true",
			},
			Finalizers: []string{ACSecretProtectionFinalizer},
		},
		Data: map[string][]byte{
			keystonev1.ACIDSecretKey:     []byte(acID),
			keystonev1.ACSecretSecretKey: []byte("fake-ac-secret"),
		},
	}
	Expect(k8sClient.Create(ctx, secret)).To(Succeed())
	return secret
}

// WaitForACFinalizer blocks until the AC CR carries its controller finalizer
func WaitForACFinalizer(name types.NamespacedName) {
	Eventually(func(g Gomega) {
		ac := GetApplicationCredential(name)
		g.Expect(ac.Finalizers).To(ContainElement(ACCRFinalizer))
	}, timeout, interval).Should(Succeed())
}

// DeleteACCR issues a delete on the AC CR. It does not wait for the CR to disappear
func DeleteACCR(name types.NamespacedName) {
	acInstance := GetApplicationCredential(name)
	Expect(k8sClient.Delete(ctx, acInstance)).To(Succeed())
}

// WaitForACGone blocks until the AC CR is fully removed from the API server
func WaitForACGone(name types.NamespacedName) {
	Eventually(func(g Gomega) {
		err := k8sClient.Get(ctx, name, &keystonev1.KeystoneApplicationCredential{})
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func GetApplicationCredential(name types.NamespacedName) *keystonev1.KeystoneApplicationCredential {
	instance := &keystonev1.KeystoneApplicationCredential{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func ACConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetApplicationCredential(name)
	return instance.Status.Conditions
}
