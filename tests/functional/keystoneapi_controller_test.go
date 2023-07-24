/*
Copyright 2022.

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

var _ = Describe("Keystone controller", func() {

	var keystoneApiName types.NamespacedName
	var dbSyncJobName types.NamespacedName
	var bootstrapJobName types.NamespacedName
	var deploymentName types.NamespacedName
	var memcachedSpec memcachedv1.MemcachedSpec

	BeforeEach(func() {

		keystoneApiName = types.NamespacedName{
			Name:      "keystone",
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
			Replicas: ptr.To(int32(3)),
		}

		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())
	})

	When("A KeystoneAPI instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneApiName, GetDefaultKeystoneAPISpec()))
		})

		It("should have the Spec fields defaulted", func() {
			Keystone := GetKeystoneAPI(keystoneApiName)
			Expect(Keystone.Spec.DatabaseInstance).Should(Equal("openstack"))
			Expect(Keystone.Spec.DatabaseUser).Should(Equal("keystone"))
			Expect(*(Keystone.Spec.Replicas)).Should(Equal(int32(1)))
		})

		It("should have the Status fields initialized", func() {
			Keystone := GetKeystoneAPI(keystoneApiName)
			Expect(Keystone.Status.Hash).To(BeEmpty())
			Expect(Keystone.Status.DatabaseHostname).To(Equal(""))
			Expect(Keystone.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("should have input not ready and unknown Conditions initialized", func() {
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)

			for _, cond := range []condition.Type{
				condition.ServiceConfigReadyCondition,
				condition.DBReadyCondition,
				condition.DBSyncReadyCondition,
				condition.ExposeServiceReadyCondition,
				condition.BootstrapReadyCondition,
				condition.DeploymentReadyCondition,
				condition.NetworkAttachmentsReadyCondition,
				condition.CronJobReadyCondition,
			} {
				th.ExpectCondition(
					keystoneApiName,
					ConditionGetterFunc(KeystoneConditionGetter),
					cond,
					corev1.ConditionUnknown,
				)
			}
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetKeystoneAPI(keystoneApiName).Finalizers
			}, timeout, interval).Should(ContainElement("KeystoneAPI"))
		})
	})

	When("The proper secret is provided", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneApiName, GetDefaultKeystoneAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
		})

		It("should have input ready and service config ready", func() {
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionUnknown,
			)
		})
	})

	When("Memcached is available", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneApiName, GetDefaultKeystoneAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
			DeferCleanup(th.DeleteMemcached, th.CreateMemcached(namespace, "memcached", memcachedSpec))
			th.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
		})

		It("should have memcached ready and service config ready", func() {
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionUnknown,
			)
		})

		It("should create a ConfigMap for keystone.conf", func() {
			cm := th.GetConfigMap(types.NamespacedName{
				Namespace: keystoneApiName.Namespace,
				Name:      fmt.Sprintf("%s-%s", keystoneApiName.Name, "config-data"),
			})
			Expect(cm.Data["keystone.conf"]).Should(
				ContainSubstring("memcache_servers=memcached-0.memcached:11211,memcached-1.memcached:11211,memcached-2.memcached:11211"))
		})
		It("should create a Secret for fernet keys", func() {
			th.GetSecret(types.NamespacedName{
				Name:      keystoneApiName.Name,
				Namespace: namespace,
			})
		})

	})

	When("DB is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneApiName, GetDefaultKeystoneAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
			DeferCleanup(th.DeleteMemcached, th.CreateMemcached(namespace, "memcached", memcachedSpec))
			th.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					namespace,
					GetKeystoneAPI(keystoneApiName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			th.SimulateMariaDBDatabaseCompleted(keystoneApiName)
		})

		It("should have db ready condition", func() {
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.BootstrapReadyCondition,
				corev1.ConditionUnknown,
			)
		})
	})

	When("DB sync is completed", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneApiName, GetDefaultKeystoneAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
			DeferCleanup(th.DeleteMemcached, th.CreateMemcached(namespace, "memcached", memcachedSpec))
			th.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					namespace,
					GetKeystoneAPI(keystoneApiName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			th.SimulateMariaDBDatabaseCompleted(keystoneApiName)
			th.SimulateJobSuccess(dbSyncJobName)
		})

		It("should have db sync ready condition and expose service ready condition", func() {
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ExposeServiceReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.BootstrapReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionUnknown,
			)
		})
	})

	When("Bootstrap is completed", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneApiName, GetDefaultKeystoneAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
			DeferCleanup(th.DeleteMemcached, th.CreateMemcached(namespace, "memcached", memcachedSpec))
			th.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					namespace,
					GetKeystoneAPI(keystoneApiName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			th.SimulateMariaDBDatabaseCompleted(keystoneApiName)
			th.SimulateJobSuccess(dbSyncJobName)
			th.SimulateJobSuccess(bootstrapJobName)
		})

		It("should have bootstrap ready condition", func() {
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.BootstrapReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.CronJobReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("Deployment is completed", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneApiName, GetDefaultKeystoneAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
			DeferCleanup(th.DeleteMemcached, th.CreateMemcached(namespace, "memcached", memcachedSpec))
			th.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					namespace,
					GetKeystoneAPI(keystoneApiName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			th.SimulateMariaDBDatabaseCompleted(keystoneApiName)
			th.SimulateJobSuccess(dbSyncJobName)
			th.SimulateJobSuccess(bootstrapJobName)
			th.SimulateDeploymentReplicaReady(deploymentName)
		})

		It("should have deployment ready condition and cronjob ready condition", func() {
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.CronJobReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should create a Deployment", func() {
			deployment := th.GetDeployment(deploymentName)
			Expect(*(deployment.Spec.Replicas)).Should(Equal(int32(1)))
		})

		It("should create a CronJob for trust flush", func() {
			cronJob := types.NamespacedName{
				Namespace: keystoneApiName.Namespace,
				Name:      fmt.Sprintf("%s-%s", keystoneApiName.Name, "cron"),
			}
			GetCronJob(cronJob)
		})

		It("should create a ConfigMap and Secret for client config", func() {
			th.GetConfigMap(types.NamespacedName{
				Namespace: keystoneApiName.Namespace,
				Name:      "openstack-config",
			})
			th.GetSecret(types.NamespacedName{
				Namespace: keystoneApiName.Namespace,
				Name:      "openstack-config-secret",
			})
		})
	})

	When("A KeystoneAPI is created with service override", func() {
		BeforeEach(func() {
			spec := GetDefaultKeystoneAPISpec()
			serviceOverride := map[string]interface{}{}
			serviceOverride["internal"] = map[string]interface{}{
				"metadata": map[string]map[string]string{
					"annotations": {
						"dnsmasq.network.openstack.org/hostname": "keystone-internal.openstack.svc",
						"metallb.universe.tf/address-pool":       "osp-internalapi",
						"metallb.universe.tf/allow-shared-ip":    "osp-internalapi",
						"metallb.universe.tf/loadBalancerIPs":    "internal-lb-ip-1,internal-lb-ip-2",
					},
					"labels": {
						"internal": "true",
						"service":  "keystone",
					},
				},
				"spec": map[string]interface{}{
					"type": "LoadBalancer",
				},
			}

			spec["override"] = map[string]interface{}{
				"service": serviceOverride,
			}

			keystone := CreateKeystoneAPI(keystoneApiName, spec)
			DeferCleanup(th.DeleteInstance, keystone)
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
			DeferCleanup(th.DeleteMemcached, th.CreateMemcached(namespace, "memcached", memcachedSpec))
			th.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					namespace,
					GetKeystoneAPI(keystoneApiName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			th.SimulateMariaDBDatabaseCompleted(keystoneApiName)
			th.SimulateJobSuccess(dbSyncJobName)
			th.SimulateJobSuccess(bootstrapJobName)
			th.SimulateDeploymentReplicaReady(deploymentName)
		})

		It("registers LoadBalancer services keystone endpoints", func() {
			instance := th.GetKeystoneAPI(keystoneApiName)
			Expect(instance).NotTo(BeNil())
			Expect(instance.Status.APIEndpoints).To(HaveKeyWithValue("public", "http://keystone-public."+keystoneApiName.Namespace+".svc:5000"))
			Expect(instance.Status.APIEndpoints).To(HaveKeyWithValue("internal", "http://keystone-internal."+keystoneApiName.Namespace+".svc:5000"))
		})

		It("creates LoadBalancer service", func() {
			// As the internal endpoint is configured in ExternalEndpoints it
			// gets a LoadBalancer Service with MetalLB annotations
			service := th.GetService(types.NamespacedName{Namespace: namespace, Name: "keystone-internal"})
			Expect(service.Annotations).To(
				HaveKeyWithValue("dnsmasq.network.openstack.org/hostname", "keystone-internal.openstack.svc"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/address-pool", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/allow-shared-ip", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/loadBalancerIPs", "internal-lb-ip-1,internal-lb-ip-2"))

			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("A KeystoneAPI is created with service override endpointURL set", func() {
		BeforeEach(func() {
			spec := GetDefaultKeystoneAPISpec()
			serviceOverride := map[string]interface{}{}
			serviceOverride["public"] = map[string]interface{}{
				"endpointURL": "http://keystone-openstack.apps-crc.testing",
			}

			spec["override"] = map[string]interface{}{
				"service": serviceOverride,
			}

			keystone := CreateKeystoneAPI(keystoneApiName, spec)
			DeferCleanup(th.DeleteInstance, keystone)
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
			DeferCleanup(th.DeleteMemcached, th.CreateMemcached(namespace, "memcached", memcachedSpec))
			th.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					namespace,
					GetKeystoneAPI(keystoneApiName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			th.SimulateMariaDBDatabaseCompleted(keystoneApiName)
			th.SimulateJobSuccess(dbSyncJobName)
			th.SimulateJobSuccess(bootstrapJobName)
			th.SimulateDeploymentReplicaReady(deploymentName)
		})

		It("registers endpointURL as public keystone endpoint", func() {
			instance := th.GetKeystoneAPI(keystoneApiName)
			Expect(instance).NotTo(BeNil())
			Expect(instance.Status.APIEndpoints).To(HaveKeyWithValue("public", "http://keystone-openstack.apps-crc.testing"))
			Expect(instance.Status.APIEndpoints).To(HaveKeyWithValue("internal", "http://keystone-internal."+keystoneApiName.Namespace+".svc:5000"))

			th.ExpectCondition(
				keystoneApiName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
})
