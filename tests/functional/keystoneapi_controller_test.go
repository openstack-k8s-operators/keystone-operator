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

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	mariadb_test "github.com/openstack-k8s-operators/mariadb-operator/api/test/helpers"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

var _ = Describe("Keystone controller", func() {

	var keystoneAPIName types.NamespacedName
	var keystoneAccountName types.NamespacedName
	var keystoneDatabaseName types.NamespacedName
	var keystoneAPIConfigDataName types.NamespacedName
	var dbSyncJobName types.NamespacedName
	var bootstrapJobName types.NamespacedName
	var deploymentName types.NamespacedName
	var caBundleSecretName types.NamespacedName
	var internalCertSecretName types.NamespacedName
	var publicCertSecretName types.NamespacedName
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
		keystoneAPIConfigDataName = types.NamespacedName{
			Name:      "keystone-config-data",
			Namespace: namespace,
		}
		caBundleSecretName = types.NamespacedName{
			Name:      CABundleSecretName,
			Namespace: namespace,
		}
		internalCertSecretName = types.NamespacedName{
			Name:      InternalCertSecretName,
			Namespace: namespace,
		}
		publicCertSecretName = types.NamespacedName{
			Name:      PublicCertSecretName,
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

	When("A KeystoneAPI instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()))
		})

		It("should have the Spec fields defaulted", func() {
			Keystone := GetKeystoneAPI(keystoneAPIName)
			Expect(Keystone.Spec.DatabaseInstance).Should(Equal("openstack"))
			Expect(Keystone.Spec.DatabaseAccount).Should(Equal(keystoneAccountName.Name))
			Expect(*(Keystone.Spec.Replicas)).Should(Equal(int32(1)))
		})

		It("should have the Status fields initialized", func() {
			Keystone := GetKeystoneAPI(keystoneAPIName)
			Expect(Keystone.Status.Hash).To(BeEmpty())
			Expect(Keystone.Status.DatabaseHostname).To(Equal(""))
			Expect(Keystone.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("should have input not ready and unknown Conditions initialized", func() {
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneAPIName,
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
					keystoneAPIName,
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
				return GetKeystoneAPI(keystoneAPIName).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/keystoneapi"))
		})
	})

	When("The proper secret is provided", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
		})

		It("should have input ready and service config ready", func() {
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.RabbitMqTransportURLReadyCondition,
				corev1.ConditionUnknown,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionUnknown,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionUnknown,
			)
		})
	})

	When("DB is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()))
			DeferCleanup(k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
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
		})

		It("should have db ready condition", func() {
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionUnknown,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.BootstrapReadyCondition,
				corev1.ConditionUnknown,
			)
		})
	})

	When("TransportURL is available", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneMessageBusSecret(namespace, "rabbitmq-secret"))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
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
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
		})

		It("should have TransportURL ready, but not Memcached ready", func() {
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.RabbitMqTransportURLReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionUnknown,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionUnknown,
			)
		})
	})

	When("Memcached is available", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneMessageBusSecret(namespace, "rabbitmq-secret"))
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
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
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
		})

		It("should have memcached ready and service config ready", func() {
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.RabbitMqTransportURLReadyCondition, corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should create a Secret for keystone.conf and my.cnf", func() {
			scrt := th.GetSecret(keystoneAPIConfigDataName)
			configData := string(scrt.Data["keystone.conf"])
			Expect(configData).To(
				ContainSubstring(fmt.Sprintf("memcache_servers=memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
					keystoneAPIName.Namespace, keystoneAPIName.Namespace, keystoneAPIName.Namespace)))
			mariadbAccount := mariadb.GetMariaDBAccount(keystoneAccountName)
			mariadbSecret := th.GetSecret(types.NamespacedName{Name: mariadbAccount.Spec.Secret, Namespace: keystoneAPIName.Namespace})

			Expect(configData).To(
				ContainSubstring(fmt.Sprintf("connection=mysql+pymysql://%s:%s@hostname-for-openstack.%s.svc/keystone?read_default_file=/etc/my.cnf",
					mariadbAccount.Spec.UserName, mariadbSecret.Data[mariadbv1.DatabasePasswordSelector], namespace)))
			configData = string(scrt.Data["my.cnf"])
			Expect(configData).To(
				ContainSubstring("[client]\nssl=0"))
		})
		It("should create a Secret for fernet keys", func() {
			th.GetSecret(types.NamespacedName{
				Name:      keystoneAPIName.Name,
				Namespace: namespace,
			})
		})

	})

	When("DB sync is completed", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneMessageBusSecret(namespace, "rabbitmq-secret"))
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()))
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
		})

		It("should have db sync ready condition and expose service ready condition", func() {
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ExposeServiceReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.BootstrapReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionUnknown,
			)
		})
	})

	When("Bootstrap is completed", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneMessageBusSecret(namespace, "rabbitmq-secret"))
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()))
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
		})

		It("should have bootstrap ready condition", func() {
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.BootstrapReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.CronJobReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("Deployment is completed", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneMessageBusSecret(namespace, "rabbitmq-secret"))
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()))
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
		})

		It("should have deployment ready condition and cronjob ready condition", func() {
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
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
				Namespace: keystoneAPIName.Namespace,
				Name:      fmt.Sprintf("%s-%s", keystoneAPIName.Name, "cron"),
			}
			GetCronJob(cronJob)
		})

		It("should create a ConfigMap and Secret for client config", func() {
			th.GetConfigMap(types.NamespacedName{
				Namespace: keystoneAPIName.Namespace,
				Name:      "openstack-config",
			})
			th.GetSecret(types.NamespacedName{
				Namespace: keystoneAPIName.Namespace,
				Name:      "openstack-config-secret",
			})
		})
	})

	When("Deployment is completed", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneMessageBusSecret(namespace, "rabbitmq-secret"))
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()))
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
		})

		It("removes the finalizers when deleted", func() {
			keystone := GetKeystoneAPI(keystoneAPIName)
			Expect(keystone.Finalizers).To(ContainElement("openstack.org/keystoneapi"))
			db := mariadb.GetMariaDBDatabase(keystoneAPIName)
			Expect(db.Finalizers).To(ContainElement("openstack.org/keystoneapi"))
			dbAcc := mariadb.GetMariaDBAccount(keystoneAccountName)
			Expect(dbAcc.Finalizers).To(ContainElement("openstack.org/keystoneapi"))

			th.DeleteInstance(GetKeystoneAPI(keystoneAPIName))

			db = mariadb.GetMariaDBDatabase(keystoneAPIName)
			Expect(db.Finalizers).NotTo(ContainElement("openstack.org/keystoneapi"))
			dbAcc = mariadb.GetMariaDBAccount(keystoneAccountName)
			Expect(dbAcc.Finalizers).NotTo(ContainElement("openstack.org/keystoneapi"))
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
			th.SimulateLoadBalancerServiceIP(types.NamespacedName{Namespace: namespace, Name: "keystone-internal"})
			th.SimulateJobSuccess(bootstrapJobName)
			th.SimulateDeploymentReplicaReady(deploymentName)
		})

		It("registers LoadBalancer services keystone endpoints", func() {
			instance := keystone.GetKeystoneAPI(keystoneAPIName)
			Expect(instance).NotTo(BeNil())
			Expect(instance.Status.APIEndpoints).To(HaveKeyWithValue("public", "http://keystone-public."+keystoneAPIName.Namespace+".svc:5000"))
			Expect(instance.Status.APIEndpoints).To(HaveKeyWithValue("internal", "http://keystone-internal."+keystoneAPIName.Namespace+".svc:5000"))
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
				keystoneAPIName,
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
		})

		It("registers endpointURL as public keystone endpoint", func() {
			instance := keystone.GetKeystoneAPI(keystoneAPIName)
			Expect(instance).NotTo(BeNil())
			Expect(instance.Status.APIEndpoints).To(HaveKeyWithValue("public", "http://keystone-openstack.apps-crc.testing"))
			Expect(instance.Status.APIEndpoints).To(HaveKeyWithValue("internal", "http://keystone-internal."+keystoneAPIName.Namespace+".svc:5000"))

			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("A KeystoneAPI is created with TLS", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetTLSKeystoneAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneMessageBusSecret(namespace, "rabbitmq-secret"))

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
			mariadb.SimulateMariaDBTLSDatabaseCompleted(keystoneAPIName)
			infra.SimulateTransportURLReady(types.NamespacedName{
				Name:      fmt.Sprintf("%s-keystone-transport", keystoneAPIName.Name),
				Namespace: namespace,
			})
			infra.SimulateMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})
		})

		It("reports that the CA secret is missing", func() {
			th.ExpectConditionWithDetails(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf("TLSInput is missing: %s", CABundleSecretName),
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the internal cert secret is missing", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(caBundleSecretName))
			th.ExpectConditionWithDetails(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf("TLSInput is missing: secrets \"%s in namespace %s\" not found",
					internalCertSecretName.Name, internalCertSecretName.Namespace),
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the public cert secret is missing", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(caBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(internalCertSecretName))
			th.ExpectConditionWithDetails(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf("TLSInput is missing: secrets \"%s in namespace %s\" not found",
					publicCertSecretName.Name, publicCertSecretName.Namespace),
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("it creates dbsync job with CA certs mounted", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(caBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(internalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(publicCertSecretName))

			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			j := th.GetJob(dbSyncJobName)
			th.AssertVolumeExists(caBundleSecretName.Name, j.Spec.Template.Spec.Volumes)
			th.AssertVolumeMountExists(caBundleSecretName.Name, "tls-ca-bundle.pem", j.Spec.Template.Spec.Containers[0].VolumeMounts)
		})

		It("it creates bootstrap job with CA certs mounted", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(caBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(internalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(publicCertSecretName))

			th.SimulateJobSuccess(dbSyncJobName)

			j := th.GetJob(bootstrapJobName)
			th.AssertVolumeExists(caBundleSecretName.Name, j.Spec.Template.Spec.Volumes)
			th.AssertVolumeMountExists(caBundleSecretName.Name, "tls-ca-bundle.pem", j.Spec.Template.Spec.Containers[0].VolumeMounts)
		})

		It("should create a Secret for keystone.conf and my.cnf", func() {
			scrt := th.GetSecret(keystoneAPIConfigDataName)
			configData := string(scrt.Data["keystone.conf"])
			Expect(configData).To(
				ContainSubstring(fmt.Sprintf("memcache_servers=memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
					keystoneAPIName.Namespace, keystoneAPIName.Namespace, keystoneAPIName.Namespace)))

			mariadbAccount := mariadb.GetMariaDBAccount(keystoneAccountName)
			mariadbSecret := th.GetSecret(types.NamespacedName{Name: mariadbAccount.Spec.Secret, Namespace: keystoneAPIName.Namespace})

			Expect(configData).To(
				ContainSubstring(fmt.Sprintf("connection=mysql+pymysql://%s:%s@hostname-for-openstack.%s.svc/keystone?read_default_file=/etc/my.cnf",
					mariadbAccount.Spec.UserName, mariadbSecret.Data[mariadbv1.DatabasePasswordSelector], namespace)))

			configData = string(scrt.Data["my.cnf"])
			Expect(configData).To(
				ContainSubstring("[client]\nssl-ca=/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem\nssl=1"))

		})

		It("it creates deployment with CA and service certs mounted", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(caBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(internalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(publicCertSecretName))

			th.SimulateJobSuccess(dbSyncJobName)
			th.SimulateJobSuccess(bootstrapJobName)

			d := th.GetDeployment(deploymentName)

			container := d.Spec.Template.Spec.Containers[0]

			// CA bundle
			th.AssertVolumeExists(caBundleSecretName.Name, d.Spec.Template.Spec.Volumes)
			th.AssertVolumeMountExists(caBundleSecretName.Name, "tls-ca-bundle.pem", container.VolumeMounts)

			// service certs
			th.AssertVolumeExists(internalCertSecretName.Name, d.Spec.Template.Spec.Volumes)
			th.AssertVolumeExists(publicCertSecretName.Name, d.Spec.Template.Spec.Volumes)
			th.AssertVolumeMountExists(publicCertSecretName.Name, "tls.key", container.VolumeMounts)
			th.AssertVolumeMountExists(publicCertSecretName.Name, "tls.crt", container.VolumeMounts)
			th.AssertVolumeMountExists(internalCertSecretName.Name, "tls.key", container.VolumeMounts)
			th.AssertVolumeMountExists(internalCertSecretName.Name, "tls.crt", container.VolumeMounts)

			Expect(container.ReadinessProbe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTPS))
			Expect(container.LivenessProbe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTPS))

			scrt := th.GetSecret(keystoneAPIConfigDataName)
			Expect(scrt).ShouldNot(BeNil())
			Expect(scrt.Data).Should(HaveKey("httpd.conf"))
			Expect(scrt.Data).Should(HaveKey("ssl.conf"))
			configData := string(scrt.Data["httpd.conf"])
			Expect(configData).Should(ContainSubstring("SSLEngine on"))
			Expect(configData).Should(ContainSubstring("SSLCertificateFile      \"/etc/pki/tls/certs/internal.crt\""))
			Expect(configData).Should(ContainSubstring("SSLCertificateKeyFile   \"/etc/pki/tls/private/internal.key\""))
			Expect(configData).Should(ContainSubstring("SSLCertificateFile      \"/etc/pki/tls/certs/public.crt\""))
			Expect(configData).Should(ContainSubstring("SSLCertificateKeyFile   \"/etc/pki/tls/private/public.key\""))
		})

		It("registers endpointURL as public keystone endpoint", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(caBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(internalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(publicCertSecretName))

			th.SimulateJobSuccess(dbSyncJobName)
			th.SimulateJobSuccess(bootstrapJobName)
			th.SimulateDeploymentReplicaReady(deploymentName)

			instance := keystone.GetKeystoneAPI(keystoneAPIName)
			Expect(instance).NotTo(BeNil())
			Expect(instance.Status.APIEndpoints).To(HaveKeyWithValue("public", "https://keystone-public."+keystoneAPIName.Namespace+".svc:5000"))
			Expect(instance.Status.APIEndpoints).To(HaveKeyWithValue("internal", "https://keystone-internal."+keystoneAPIName.Namespace+".svc:5000"))

			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("reconfigures the keystone pod when CA changes", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(caBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(internalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(publicCertSecretName))

			th.SimulateJobSuccess(dbSyncJobName)
			th.SimulateJobSuccess(bootstrapJobName)

			// Grab the current config hash
			originalHash := GetEnvVarValue(
				th.GetDeployment(deploymentName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
			Expect(originalHash).NotTo(BeEmpty())

			// Change the content of the CA secret
			th.UpdateSecret(caBundleSecretName, "tls-ca-bundle.pem", []byte("DifferentCAData"))

			// Assert that the deployment is updated
			Eventually(func(g Gomega) {
				newHash := GetEnvVarValue(
					th.GetDeployment(deploymentName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(newHash).NotTo(BeEmpty())
				g.Expect(newHash).NotTo(Equal(originalHash))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("A KeystoneAPI is created with TLS and service override endpointURL set", func() {
		BeforeEach(func() {
			spec := GetTLSKeystoneAPISpec()
			serviceOverride := map[string]interface{}{}
			serviceOverride["public"] = map[string]interface{}{
				"endpointURL": "https://keystone-openstack.apps-crc.testing",
			}

			spec["override"] = map[string]interface{}{
				"service": serviceOverride,
			}

			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(caBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(internalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(publicCertSecretName))
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, spec))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneMessageBusSecret(namespace, "rabbitmq-secret"))
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
		})

		It("registers endpointURL as public keystone endpoint", func() {
			instance := keystone.GetKeystoneAPI(keystoneAPIName)
			Expect(instance).NotTo(BeNil())
			Expect(instance.Status.APIEndpoints).To(HaveKeyWithValue("public", "https://keystone-openstack.apps-crc.testing"))
			Expect(instance.Status.APIEndpoints).To(HaveKeyWithValue("internal", "https://keystone-internal."+keystoneAPIName.Namespace+".svc:5000"))

			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	// Run MariaDBAccount suite tests.  these are pre-packaged ginkgo tests
	// that exercise standard account create / update patterns that should be
	// common to all controllers that ensure MariaDBAccount CRs.
	mariadbSuite := &mariadb_test.MariaDBTestHarness{
		PopulateHarness: func(harness *mariadb_test.MariaDBTestHarness) {
			harness.Setup(
				"Keystone",
				keystoneAPIName.Namespace,
				keystoneAPIName.Name,
				"openstack.org/keystoneapi",
				mariadb,
				timeout,
				interval,
			)
		},
		// Generate a fully running Keystone service given an accountName
		// needs to make it all the way to the end where the mariadb finalizers
		// are removed from unused accounts since that's part of what we are testing
		SetupCR: func(accountName types.NamespacedName) {

			spec := GetDefaultKeystoneAPISpec()
			spec["databaseAccount"] = accountName.Name

			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneMessageBusSecret(namespace, "rabbitmq-secret"))
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, spec))
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

			mariadb.SimulateMariaDBAccountCompleted(accountName)
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
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

		},
		// Change the account name in the service to a new name
		UpdateAccount: func(newAccountName types.NamespacedName) {

			Eventually(func(g Gomega) {
				keystoneapi := GetKeystoneAPI(keystoneAPIName)
				keystoneapi.Spec.DatabaseAccount = newAccountName.Name
				g.Expect(th.K8sClient.Update(ctx, keystoneapi)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

		},
		// delete the keystone instance to exercise finalizer removal
		DeleteCR: func() {
			th.DeleteInstance(GetKeystoneAPI(keystoneAPIName))
		},
	}

	mariadbSuite.RunBasicSuite()

	mariadbSuite.RunURLAssertSuite(func(accountName types.NamespacedName, username string, password string) {
		Eventually(func(g Gomega) {
			scrt := th.GetSecret(keystoneAPIConfigDataName)
			configData := string(scrt.Data["keystone.conf"])

			g.Expect(configData).To(
				ContainSubstring(fmt.Sprintf("connection=mysql+pymysql://%s:%s@hostname-for-openstack.%s.svc/keystone?read_default_file=/etc/my.cnf",
					username, password, namespace)))
		}, timeout, interval).Should(Succeed())

	})

	mariadbSuite.RunConfigHashSuite(func() string {
		deployment := th.GetDeployment(deploymentName)
		return GetEnvVarValue(deployment.Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
	})

})
