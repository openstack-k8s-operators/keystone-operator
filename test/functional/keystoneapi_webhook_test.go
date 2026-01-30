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
	"errors"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
)

var _ = Describe("KeystoneAPI Webhook", func() {

	var keystoneAPIName types.NamespacedName
	var keystoneAccountName types.NamespacedName
	var keystoneDatabaseName types.NamespacedName
	var dbSyncJobName types.NamespacedName
	var bootstrapJobName types.NamespacedName
	var deploymentName types.NamespacedName
	var memcachedSpec memcachedv1.MemcachedSpec

	BeforeEach(func() {

		keystoneAPIName = types.NamespacedName{
			Name:      "keystone",
			Namespace: namespace,
		}
		keystoneAccountName = types.NamespacedName{
			Name:      AccountName,
			Namespace: namespace,
		}
		keystoneDatabaseName = types.NamespacedName{
			Name:      DatabaseCRName,
			Namespace: namespace,
		}
		dbSyncJobName = types.NamespacedName{
			Name:      "keystone-db-sync",
			Namespace: namespace,
		}
		bootstrapJobName = types.NamespacedName{
			Name:      "keystone-bootstrap",
			Namespace: namespace,
		}
		deploymentName = types.NamespacedName{
			Name:      "keystone",
			Namespace: namespace,
		}
		memcachedSpec = infra.GetDefaultMemcachedSpec()

		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())
	})

	When("A KeystoneAPI instance is created without container images", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()))
		})

		It("should have the defaults initialized by webhook", func() {
			KeystoneAPI := GetKeystoneAPI(keystoneAPIName)
			Expect(KeystoneAPI.Spec.ContainerImage).Should(Equal(
				keystonev1.KeystoneAPIContainerImage,
			))
		})
	})

	When("A KeystoneAPI instance is created with container images", func() {
		BeforeEach(func() {
			spec := GetDefaultKeystoneAPISpec()
			spec["containerImage"] = "api-container-image"
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, spec))
		})

		It("should use the given values", func() {
			KeystoneAPI := GetKeystoneAPI(keystoneAPIName)
			Expect(KeystoneAPI.Spec.ContainerImage).Should(Equal(
				"api-container-image",
			))
		})
	})

	It("rejects with wrong service override endpoint type", func() {
		spec := GetDefaultKeystoneAPISpec()
		spec["override"] = map[string]any{
			"service": map[string]any{
				"internal": map[string]any{},
				"wrooong":  map[string]any{},
			},
		}

		raw := map[string]any{
			"apiVersion": "keystone.openstack.org/v1beta1",
			"kind":       "KeystoneAPI",
			"metadata": map[string]any{
				"name":      keystoneAPIName.Name,
				"namespace": keystoneAPIName.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(
			ContainSubstring(
				"invalid: spec.override.service[wrooong]: " +
					"Invalid value: \"wrooong\": invalid endpoint type: wrooong"),
		)
	})

	When("A KeystoneAPI instance is updated with wrong service override endpoint", func() {
		BeforeEach(func() {
			spec := GetDefaultKeystoneAPISpec()
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneMessageBusSecret(namespace, "rabbitmq-secret"))
			keystone := CreateKeystoneAPI(keystoneAPIName, spec)
			DeferCleanup(th.DeleteInstance, keystone)
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetKeystoneAPI(keystoneAPIName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.SimulateMariaDBAccountCompleted(keystoneAccountName)
			mariadb.SimulateMariaDBDatabaseCompleted(keystoneDatabaseName)
			infra.SimulateTransportURLReady(types.NamespacedName{
				Name:      fmt.Sprintf("%s-keystone-transport", keystoneAPIName.Name),
				Namespace: namespace,
			})
			infra.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
			th.SimulateJobSuccess(dbSyncJobName)
			th.SimulateJobSuccess(bootstrapJobName)
			th.SimulateDeploymentReplicaReady(deploymentName)

			Eventually(func(g Gomega) {
				instance := GetKeystoneAPI(keystoneAPIName)
				g.Expect(instance).NotTo(BeNil())
				g.Expect(instance.Status.Conditions.IsTrue(condition.ReadyCondition)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("rejects update with wrong service override endpoint type", func() {
			KeystoneAPI := GetKeystoneAPI(keystoneAPIName)
			Expect(KeystoneAPI).NotTo(BeNil())
			if KeystoneAPI.Spec.Override.Service == nil {
				KeystoneAPI.Spec.Override.Service = map[service.Endpoint]service.RoutedOverrideSpec{}
			}
			KeystoneAPI.Spec.Override.Service["wrooong"] = service.RoutedOverrideSpec{}
			err := k8sClient.Update(ctx, KeystoneAPI)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				ContainSubstring(
					"invalid: spec.override.service[wrooong]: " +
						"Invalid value: \"wrooong\": invalid endpoint type: wrooong"),
			)
		})
	})
	It("rejects a wrong TopologyRef on a different namespace", func() {
		keystoneSpec := GetDefaultKeystoneAPISpec()
		// Inject a topologyRef that points to a different namespace
		keystoneSpec["topologyRef"] = map[string]any{
			"name":      "foo",
			"namespace": "bar",
		}
		raw := map[string]any{
			"apiVersion": "keystone.openstack.org/v1beta1",
			"kind":       "KeystoneAPI",
			"metadata": map[string]any{
				"name":      "keystoneapi",
				"namespace": namespace,
			},
			"spec": keystoneSpec,
		}
		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(
			ContainSubstring(
				"spec.topologyRef.namespace: Invalid value: \"namespace\": Customizing namespace field is not supported"),
		)
	})

	When("ExternalKeystoneAPI validation", func() {
		It("rejects ExternalKeystoneAPI=true with nil service override", func() {
			spec := GetDefaultKeystoneAPISpec()
			spec["externalKeystoneAPI"] = true
			// Don't set any override

			raw := map[string]any{
				"apiVersion": "keystone.openstack.org/v1beta1",
				"kind":       "KeystoneAPI",
				"metadata": map[string]any{
					"name":      keystoneAPIName.Name,
					"namespace": keystoneAPIName.Namespace,
				},
				"spec": spec,
			}

			unstructuredObj := &unstructured.Unstructured{Object: raw}
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
			Expect(err).To(HaveOccurred())
			// The validation now checks for missing endpoints rather than missing override
			Expect(err.Error()).To(
				Or(
					ContainSubstring("external Keystone API requires public endpoint to be defined"),
					ContainSubstring("external Keystone API requires internal endpoint to be defined"),
				),
			)
		})

		It("rejects ExternalKeystoneAPI=true with empty service override", func() {
			spec := GetDefaultKeystoneAPISpec()
			spec["externalKeystoneAPI"] = true
			spec["override"] = map[string]any{
				"service": map[string]any{},
			}

			raw := map[string]any{
				"apiVersion": "keystone.openstack.org/v1beta1",
				"kind":       "KeystoneAPI",
				"metadata": map[string]any{
					"name":      keystoneAPIName.Name,
					"namespace": keystoneAPIName.Namespace,
				},
				"spec": spec,
			}

			unstructuredObj := &unstructured.Unstructured{Object: raw}
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
			Expect(err).To(HaveOccurred())
			// The validation now checks for missing endpoints rather than missing override
			Expect(err.Error()).To(
				Or(
					ContainSubstring("external Keystone API requires public endpoint to be defined"),
					ContainSubstring("external Keystone API requires internal endpoint to be defined"),
				),
			)
		})

		It("rejects ExternalKeystoneAPI=true with service override but no endpoints", func() {
			spec := GetDefaultKeystoneAPISpec()
			spec["externalKeystoneAPI"] = true
			spec["override"] = map[string]any{
				"service": map[string]any{
					"admin": map[string]any{
						"metadata": map[string]any{
							"labels": map[string]any{
								"custom": "label",
							},
						},
					},
				},
			}

			raw := map[string]any{
				"apiVersion": "keystone.openstack.org/v1beta1",
				"kind":       "KeystoneAPI",
				"metadata": map[string]any{
					"name":      keystoneAPIName.Name,
					"namespace": keystoneAPIName.Namespace,
				},
				"spec": spec,
			}

			unstructuredObj := &unstructured.Unstructured{Object: raw}
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				ContainSubstring(
					"external Keystone API requires public endpoint to be defined"),
			)
		})

		It("rejects ExternalKeystoneAPI=true with missing public endpoint", func() {
			spec := GetDefaultKeystoneAPISpec()
			spec["externalKeystoneAPI"] = true
			spec["override"] = map[string]any{
				"service": map[string]any{
					"internal": map[string]any{
						"endpointURL": "http://internal.keystone.example.com:5000",
					},
				},
			}

			raw := map[string]any{
				"apiVersion": "keystone.openstack.org/v1beta1",
				"kind":       "KeystoneAPI",
				"metadata": map[string]any{
					"name":      keystoneAPIName.Name,
					"namespace": keystoneAPIName.Namespace,
				},
				"spec": spec,
			}

			unstructuredObj := &unstructured.Unstructured{Object: raw}
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				ContainSubstring(
					"external Keystone API requires public endpoint to be defined"),
			)
		})

		It("rejects ExternalKeystoneAPI=true with missing internal endpoint", func() {
			spec := GetDefaultKeystoneAPISpec()
			spec["externalKeystoneAPI"] = true
			spec["override"] = map[string]any{
				"service": map[string]any{
					"public": map[string]any{
						"endpointURL": "http://public.keystone.example.com:5000",
					},
				},
			}

			raw := map[string]any{
				"apiVersion": "keystone.openstack.org/v1beta1",
				"kind":       "KeystoneAPI",
				"metadata": map[string]any{
					"name":      keystoneAPIName.Name,
					"namespace": keystoneAPIName.Namespace,
				},
				"spec": spec,
			}

			unstructuredObj := &unstructured.Unstructured{Object: raw}
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				ContainSubstring(
					"external Keystone API requires internal endpoint to be defined"),
			)
		})

		It("rejects ExternalKeystoneAPI=true with empty endpointURL", func() {
			spec := GetDefaultKeystoneAPISpec()
			spec["externalKeystoneAPI"] = true
			spec["override"] = map[string]any{
				"service": map[string]any{
					"public": map[string]any{
						"endpointURL": "",
					},
					"internal": map[string]any{
						"endpointURL": "http://internal.keystone.example.com:5000",
					},
				},
			}

			raw := map[string]any{
				"apiVersion": "keystone.openstack.org/v1beta1",
				"kind":       "KeystoneAPI",
				"metadata": map[string]any{
					"name":      keystoneAPIName.Name,
					"namespace": keystoneAPIName.Namespace,
				},
				"spec": spec,
			}

			unstructuredObj := &unstructured.Unstructured{Object: raw}
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				ContainSubstring(
					"external Keystone API requires endpointURL to be set for public endpoint"),
			)
		})

		It("rejects ExternalKeystoneAPI=true with invalid URL format for public endpoint", func() {
			spec := GetDefaultKeystoneAPISpec()
			spec["externalKeystoneAPI"] = true
			spec["override"] = map[string]any{
				"service": map[string]any{
					"public": map[string]any{
						"endpointURL": "not-a-valid-url",
					},
					"internal": map[string]any{
						"endpointURL": "http://internal.keystone.example.com:5000",
					},
				},
			}

			raw := map[string]any{
				"apiVersion": "keystone.openstack.org/v1beta1",
				"kind":       "KeystoneAPI",
				"metadata": map[string]any{
					"name":      keystoneAPIName.Name,
					"namespace": keystoneAPIName.Namespace,
				},
				"spec": spec,
			}

			unstructuredObj := &unstructured.Unstructured{Object: raw}
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				ContainSubstring(
					"URL must include a scheme"),
			)
		})

		It("rejects ExternalKeystoneAPI=true with invalid URL format for internal endpoint", func() {
			spec := GetDefaultKeystoneAPISpec()
			spec["externalKeystoneAPI"] = true
			spec["override"] = map[string]any{
				"service": map[string]any{
					"public": map[string]any{
						"endpointURL": "http://public.keystone.example.com:5000",
					},
					"internal": map[string]any{
						"endpointURL": "://invalid-url",
					},
				},
			}

			raw := map[string]any{
				"apiVersion": "keystone.openstack.org/v1beta1",
				"kind":       "KeystoneAPI",
				"metadata": map[string]any{
					"name":      keystoneAPIName.Name,
					"namespace": keystoneAPIName.Namespace,
				},
				"spec": spec,
			}

			unstructuredObj := &unstructured.Unstructured{Object: raw}
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				Or(
					ContainSubstring("invalid URL format"),
					ContainSubstring("URL must include a scheme"),
				),
			)
		})

		It("accepts ExternalKeystoneAPI=true with valid endpoints", func() {
			spec := GetDefaultKeystoneAPISpec()
			spec["externalKeystoneAPI"] = true
			spec["override"] = map[string]any{
				"service": map[string]any{
					"public": map[string]any{
						"endpointURL": "http://public.keystone.example.com:5000",
					},
					"internal": map[string]any{
						"endpointURL": "http://internal.keystone.example.com:5000",
					},
				},
			}

			raw := map[string]any{
				"apiVersion": "keystone.openstack.org/v1beta1",
				"kind":       "KeystoneAPI",
				"metadata": map[string]any{
					"name":      keystoneAPIName.Name,
					"namespace": keystoneAPIName.Namespace,
				},
				"spec": spec,
			}

			unstructuredObj := &unstructured.Unstructured{Object: raw}
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
			Expect(err).NotTo(HaveOccurred())
		})
	})

	It("rejects update to deprecated rabbitMqClusterName field", func() {
		spec := GetDefaultKeystoneAPISpec()
		spec["rabbitMqClusterName"] = "rabbitmq"

		keystoneName := types.NamespacedName{
			Namespace: namespace,
			Name:      "keystone-webhook-test",
		}

		raw := map[string]any{
			"apiVersion": "keystone.openstack.org/v1beta1",
			"kind":       "KeystoneAPI",
			"metadata": map[string]any{
				"name":      keystoneName.Name,
				"namespace": keystoneName.Namespace,
			},
			"spec": spec,
		}

		// Create the KeystoneAPI instance
		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })
		Expect(err).ShouldNot(HaveOccurred())

		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, unstructuredObj)
		})

		// Try to update rabbitMqClusterName
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, keystoneName, unstructuredObj)).Should(Succeed())
			specMap := unstructuredObj.Object["spec"].(map[string]any)
			specMap["rabbitMqClusterName"] = "rabbitmq2"
			err := k8sClient.Update(ctx, unstructuredObj)
			g.Expect(err).Should(HaveOccurred())

			var statusError *k8s_errors.StatusError
			g.Expect(errors.As(err, &statusError)).To(BeTrue())
			g.Expect(statusError.ErrStatus.Details.Kind).To(Equal("KeystoneAPI"))
			g.Expect(statusError.ErrStatus.Message).To(
				ContainSubstring("field \"spec.rabbitMqClusterName\" is deprecated, use \"spec.notificationsBus.cluster\" instead"))
		}, timeout, interval).Should(Succeed())
	})
})
