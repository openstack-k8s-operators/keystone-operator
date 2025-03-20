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
	"os"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
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
		memcachedSpec = memcachedv1.MemcachedSpec{
			MemcachedSpecCore: memcachedv1.MemcachedSpecCore{
				Replicas: ptr.To(int32(3)),
			},
		}

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
		spec["override"] = map[string]interface{}{
			"service": map[string]interface{}{
				"internal": map[string]interface{}{},
				"wrooong":  map[string]interface{}{},
			},
		}

		raw := map[string]interface{}{
			"apiVersion": "keystone.openstack.org/v1beta1",
			"kind":       "KeystoneAPI",
			"metadata": map[string]interface{}{
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

			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
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
		keystoneSpec["topologyRef"] = map[string]interface{}{
			"name":      "foo",
			"namespace": "bar",
		}
		raw := map[string]interface{}{
			"apiVersion": "keystone.openstack.org/v1beta1",
			"kind":       "KeystoneAPI",
			"metadata": map[string]interface{}{
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
})
