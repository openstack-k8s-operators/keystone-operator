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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	"github.com/openstack-k8s-operators/lib-common/modules/common"
	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	mariadb_test "github.com/openstack-k8s-operators/mariadb-operator/api/test/helpers"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	var cronJobName types.NamespacedName
	var keystoneAPITopologies []types.NamespacedName
	var acName types.NamespacedName
	var serviceUserSecret types.NamespacedName

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
		memcachedSpec = infra.GetDefaultMemcachedSpec()
		cronJobName = types.NamespacedName{
			Namespace: keystoneAPIName.Namespace,
			Name:      "keystone-cron",
		}
		keystoneAPITopologies = []types.NamespacedName{
			{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-topology", keystoneAPIName.Name),
			},
			{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-topology-alt", keystoneAPIName.Name),
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
				condition.CreateServiceReadyCondition,
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
				ContainSubstring("backend = dogpile.cache.memcached"))
			Expect(configData).To(
				ContainSubstring(fmt.Sprintf("memcache_servers=inet:memcached-0.memcached.%s.svc:11211,inet:memcached-1.memcached.%s.svc:11211,inet:memcached-2.memcached.%s.svc:11211",
					keystoneAPIName.Namespace, keystoneAPIName.Namespace, keystoneAPIName.Namespace)))
			mariadbAccount := mariadb.GetMariaDBAccount(keystoneAccountName)
			mariadbSecret := th.GetSecret(types.NamespacedName{Name: mariadbAccount.Spec.Secret, Namespace: keystoneAPIName.Namespace})

			Expect(configData).To(
				ContainSubstring(fmt.Sprintf("connection=mysql+pymysql://%s:%s@hostname-for-openstack.%s.svc/keystone?read_default_file=/etc/my.cnf",
					mariadbAccount.Spec.UserName, mariadbSecret.Data[mariadbv1.DatabasePasswordSelector], namespace)))
			configData = string(scrt.Data["my.cnf"])
			Expect(configData).To(
				ContainSubstring("[client]\nssl=0"))
			httpdConfData := string(scrt.Data["httpd.conf"])
			Expect(httpdConfData).To(
				ContainSubstring("TimeOut 60"))
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
				condition.CreateServiceReadyCondition,
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
			GetCronJob(cronJobName)
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

	When("Deployment rollout is progressing", func() {
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
			th.SimulateDeploymentProgressing(deploymentName)
		})

		It("shows the deployment progressing in DeploymentReadyCondition", func() {
			th.ExpectConditionWithDetails(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("still shows the deployment progressing in DeploymentReadyCondition when rollout hits ProgressDeadlineExceeded", func() {
			th.SimulateDeploymentProgressDeadlineExceeded(deploymentName)
			th.ExpectConditionWithDetails(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reaches Ready when deployment rollout finished", func() {
			th.ExpectConditionWithDetails(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			th.SimulateDeploymentReplicaReady(deploymentName)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("A KeystoneAPI is created with service override", func() {
		BeforeEach(func() {
			spec := GetDefaultKeystoneAPISpec()
			serviceOverride := map[string]any{}
			serviceOverride["internal"] = map[string]any{
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
				"spec": map[string]any{
					"type": "LoadBalancer",
				},
			}

			spec["override"] = map[string]any{
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
			serviceOverride := map[string]any{}
			serviceOverride["public"] = map[string]any{
				"endpointURL": "http://keystone-openstack.apps-crc.testing",
			}

			spec["override"] = map[string]any{
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
			infra.SimulateTLSMemcachedReady(types.NamespacedName{
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
				condition.ErrorReason,
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
			th.AssertVolumeMountPathExists(caBundleSecretName.Name, "", "tls-ca-bundle.pem", j.Spec.Template.Spec.Containers[0].VolumeMounts)
		})

		It("it creates bootstrap job with CA certs mounted", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(caBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(internalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(publicCertSecretName))

			th.SimulateJobSuccess(dbSyncJobName)

			j := th.GetJob(bootstrapJobName)
			th.AssertVolumeExists(caBundleSecretName.Name, j.Spec.Template.Spec.Volumes)
			th.AssertVolumeMountPathExists(caBundleSecretName.Name, "", "tls-ca-bundle.pem", j.Spec.Template.Spec.Containers[0].VolumeMounts)
		})

		It("should create a Secret for keystone.conf and my.cnf", func() {
			scrt := th.GetSecret(keystoneAPIConfigDataName)
			configData := string(scrt.Data["keystone.conf"])
			Expect(configData).To(
				ContainSubstring("backend = oslo_cache.memcache_pool"))
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
			th.AssertVolumeMountPathExists(caBundleSecretName.Name, "", "tls-ca-bundle.pem", container.VolumeMounts)

			// service certs
			th.AssertVolumeExists(internalCertSecretName.Name, d.Spec.Template.Spec.Volumes)
			th.AssertVolumeExists(publicCertSecretName.Name, d.Spec.Template.Spec.Volumes)
			th.AssertVolumeMountPathExists(publicCertSecretName.Name, "", "tls.key", container.VolumeMounts)
			th.AssertVolumeMountPathExists(publicCertSecretName.Name, "", "tls.crt", container.VolumeMounts)
			th.AssertVolumeMountPathExists(internalCertSecretName.Name, "", "tls.key", container.VolumeMounts)
			th.AssertVolumeMountPathExists(internalCertSecretName.Name, "", "tls.crt", container.VolumeMounts)

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
			serviceOverride := map[string]any{}
			serviceOverride["public"] = map[string]any{
				"endpointURL": "https://keystone-openstack.apps-crc.testing",
			}

			spec["override"] = map[string]any{
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

	When("When FernetMaxActiveKeys is created with a number lower than 3", func() {
		It("should fail", func() {
			err := InterceptGomegaFailure(
				func() {
					CreateKeystoneAPI(keystoneAPIName, GetKeystoneAPISpec(-1))
				})
			Expect(err).Should(HaveOccurred())
		})
	})

	When("When the fernet keys are created with FernetMaxActiveKeys as 3", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneMessageBusSecret(namespace, "rabbitmq-secret"))
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetKeystoneAPISpec(3)))
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

		It("creates 3 keys", func() {
			secret := th.GetSecret(types.NamespacedName{Namespace: keystoneAPIName.Namespace, Name: "keystone"})
			Expect(secret).ToNot(BeNil())

			Eventually(func(g Gomega) {
				numberFernetKeys := 0
				for k := range secret.Data {
					if strings.HasPrefix(k, "FernetKeys") {
						numberFernetKeys++
					}
				}

				g.Expect(numberFernetKeys).Should(BeNumerically("==", 3))
				for i := range 3 {
					g.Expect(secret.Data["FernetKeys"+strconv.Itoa(i)]).NotTo(BeNil())
				}
			}, timeout, interval).Should(Succeed())
		})
	})

	When("When the fernet keys are created with FernetMaxActiveKeys as 100", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneMessageBusSecret(namespace, "rabbitmq-secret"))
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetKeystoneAPISpec(100)))
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

		It("creates 100 keys", func() {
			secret := th.GetSecret(types.NamespacedName{Namespace: keystoneAPIName.Namespace, Name: "keystone"})
			Expect(secret).ToNot(BeNil())

			Eventually(func(g Gomega) {
				numberFernetKeys := 0
				for k := range secret.Data {
					if strings.HasPrefix(k, "FernetKeys") {
						numberFernetKeys++
					}
				}

				g.Expect(numberFernetKeys).Should(BeNumerically("==", 100))
				for i := range 100 {
					g.Expect(secret.Data["FernetKeys"+strconv.Itoa(i)]).NotTo(BeNil())
				}
			}, timeout, interval).Should(Succeed())
		})
	})

	When("When the fernet keys are updated from 5 to 4", func() {
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

		It("removes the additional key", func() {
			secret := th.GetSecret(types.NamespacedName{Namespace: keystoneAPIName.Namespace, Name: "keystone"})
			Expect(secret).ToNot(BeNil())

			keystone := GetKeystoneAPI(keystoneAPIName)

			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, keystone, func() error {
					keystone.Spec.FernetMaxActiveKeys = ptr.To(int32(4))
					return nil
				})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				secret = th.GetSecret(types.NamespacedName{Namespace: keystoneAPIName.Namespace, Name: "keystone"})
				numberFernetKeys := 0
				for k := range secret.Data {
					if strings.HasPrefix(k, "FernetKeys") {
						numberFernetKeys++
					}
				}

				g.Expect(numberFernetKeys).Should(BeNumerically("==", 4))
				for i := range 4 {
					g.Expect(secret.Data["FernetKeys"+strconv.Itoa(i)]).NotTo(BeNil())
				}
			}, timeout, interval).Should(Succeed())
		})
	})

	When("When the fernet keys are updated from 5 to 6", func() {
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

		It("creates an additional key", func() {
			secret := th.GetSecret(types.NamespacedName{Namespace: keystoneAPIName.Namespace, Name: "keystone"})
			Expect(secret).ToNot(BeNil())

			keystone := GetKeystoneAPI(keystoneAPIName)

			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, keystone, func() error {
					keystone.Spec.FernetMaxActiveKeys = ptr.To(int32(6))
					return nil
				})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				secret = th.GetSecret(types.NamespacedName{Namespace: keystoneAPIName.Namespace, Name: "keystone"})
				numberFernetKeys := 0
				for k := range secret.Data {
					if strings.HasPrefix(k, "FernetKeys") {
						numberFernetKeys++
					}
				}

				g.Expect(numberFernetKeys).Should(BeNumerically("==", 6))
				for i := range 6 {
					g.Expect(secret.Data["FernetKeys"+strconv.Itoa(i)]).NotTo(BeNil())
				}
			}, timeout, interval).Should(Succeed())
		})
	})

	// Set rotated at to past date, triggering rotation
	When("When the fernet token rotate", func() {
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

		It("rotates the fernet keys", func() {
			keystone := GetKeystoneAPI(keystoneAPIName)
			currentHash := keystone.Status.Hash["input"]

			currentSecret := th.GetSecret(types.NamespacedName{Namespace: keystoneAPIName.Namespace, Name: "keystone"})
			Expect(currentSecret).ToNot(BeNil())

			rotatedAt, err := time.Parse(time.RFC3339, currentSecret.Annotations["keystone.openstack.org/rotatedat"])
			Expect(err).ToNot(HaveOccurred())

			// set date to yesterday
			currentSecret.Annotations["keystone.openstack.org/rotatedat"] = rotatedAt.Add(-25 * time.Hour).Format(time.RFC3339)
			err = k8sClient.Update(ctx, ptr.To(currentSecret), &client.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				keystone = GetKeystoneAPI(keystoneAPIName)
				// With the new direct mounting approach, the input hash should NOT change
				g.Expect(keystone.Status.Hash["input"]).To(Equal(currentHash))

				updatedSecret := th.GetSecret(types.NamespacedName{Namespace: keystoneAPIName.Namespace, Name: "keystone"})
				g.Expect(updatedSecret).ToNot(BeNil())

				for i := range 4 {

					// old idx 0 > new 4
					if i == 0 {
						oldKey := string(currentSecret.Data["FernetKeys"+strconv.Itoa(0)])
						newKey := string(updatedSecret.Data["FernetKeys"+strconv.Itoa((4))])
						g.Expect(oldKey).To(Equal(newKey))
						continue
					}

					// old idx > new idx-1, except idx 1 which should be gone and not match new idx 0
					oldKey := string(currentSecret.Data["FernetKeys"+strconv.Itoa(i)])
					newKey := string(updatedSecret.Data["FernetKeys"+strconv.Itoa((i-1))])
					if i == 1 {
						g.Expect(oldKey).ToNot(Equal(newKey))
					} else {
						g.Expect(oldKey).To(Equal(newKey))
					}
				}
			}, timeout, interval).Should(Succeed())

		})
	})

	When("Topology is referenced", func() {
		var topologyRef, topologyRefAlt *topologyv1.TopoRef
		BeforeEach(func() {

			// Define the two topology references used in this test
			topologyRef = &topologyv1.TopoRef{
				Name:      keystoneAPITopologies[0].Name,
				Namespace: keystoneAPITopologies[0].Namespace,
			}
			topologyRefAlt = &topologyv1.TopoRef{
				Name:      keystoneAPITopologies[1].Name,
				Namespace: keystoneAPITopologies[1].Namespace,
			}

			// Create Test Topologies
			for _, t := range keystoneAPITopologies {
				// Build the topology Spec
				topologySpec, _ := GetSampleTopologySpec(t.Name)
				infra.CreateTopology(t, topologySpec)
			}
			spec := GetDefaultKeystoneAPISpec()
			spec["topologyRef"] = map[string]any{
				"name": topologyRef.Name,
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

		It("check topology has been applied", func() {
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRef.Name,
					Namespace: topologyRef.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).To(HaveLen(1))
				keystoneAPI := GetKeystoneAPI(keystoneAPIName)
				g.Expect(keystoneAPI.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(keystoneAPI.Status.LastAppliedTopology).To(Equal(topologyRef))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/keystoneapi-%s", keystoneAPIName.Name)))
			}, timeout, interval).Should(Succeed())
		})
		It("sets topology in resource specs", func() {
			Eventually(func(g Gomega) {
				_, topologySpecObj := GetSampleTopologySpec(topologyRef.Name)
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.Affinity).To(BeNil())
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.TopologySpreadConstraints).ToNot(BeNil())
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.TopologySpreadConstraints).To(Equal(topologySpecObj))
			}, timeout, interval).Should(Succeed())
		})
		It("updates topology when the reference changes", func() {
			Eventually(func(g Gomega) {
				keystoneAPI := GetKeystoneAPI(keystoneAPIName)
				keystoneAPI.Spec.TopologyRef.Name = topologyRefAlt.Name
				g.Expect(k8sClient.Update(ctx, keystoneAPI)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).To(HaveLen(1))

				keystoneAPI := GetKeystoneAPI(keystoneAPIName)
				g.Expect(keystoneAPI.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(keystoneAPI.Status.LastAppliedTopology).To(Equal(topologyRefAlt))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/keystoneapi-%s", keystoneAPIName.Name)))

				// Verify the previous referenced topology has no finalizers
				tp = infra.GetTopology(types.NamespacedName{
					Name:      topologyRef.Name,
					Namespace: topologyRef.Namespace,
				})
				finalizers = tp.GetFinalizers()
				g.Expect(finalizers).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})
		It("removes topologyRef from the spec", func() {
			Eventually(func(g Gomega) {
				keystoneAPI := GetKeystoneAPI(keystoneAPIName)
				// Remove the TopologyRef from the existing KeystoneAPI .Spec
				keystoneAPI.Spec.TopologyRef = nil
				g.Expect(k8sClient.Update(ctx, keystoneAPI)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				keystoneAPI := GetKeystoneAPI(keystoneAPIName)
				g.Expect(keystoneAPI.Status.LastAppliedTopology).Should(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.TopologySpreadConstraints).To(BeNil())
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.Affinity).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			// Verify the existing topologies have no finalizer anymore
			Eventually(func(g Gomega) {
				for _, topology := range keystoneAPITopologies {
					tp := infra.GetTopology(types.NamespacedName{
						Name:      topology.Name,
						Namespace: topology.Namespace,
					})
					finalizers := tp.GetFinalizers()
					g.Expect(finalizers).To(BeEmpty())
				}
			}, timeout, interval).Should(Succeed())
		})
	})

	When("A KeystoneAPI is created with nodeSelector", func() {
		BeforeEach(func() {
			spec := GetDefaultKeystoneAPISpec()
			spec["nodeSelector"] = map[string]any{
				"foo": "bar",
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

		It("sets nodeSelector in resource specs", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(bootstrapJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(dbSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(GetCronJob(cronJobName).Spec.JobTemplate.Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())
		})

		It("updates nodeSelector in resource specs when changed", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(bootstrapJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(dbSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(GetCronJob(cronJobName).Spec.JobTemplate.Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				keystone := GetKeystoneAPI(keystoneAPIName)
				newNodeSelector := map[string]string{
					"foo2": "bar2",
				}
				keystone.Spec.NodeSelector = &newNodeSelector
				g.Expect(k8sClient.Update(ctx, keystone)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				th.SimulateJobSuccess(dbSyncJobName)
				th.SimulateJobSuccess(bootstrapJobName)
				th.SimulateDeploymentReplicaReady(deploymentName)
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
				g.Expect(th.GetJob(bootstrapJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
				g.Expect(th.GetJob(dbSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
				g.Expect(GetCronJob(cronJobName).Spec.JobTemplate.Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
			}, timeout, interval).Should(Succeed())
		})

		It("removes nodeSelector from resource specs when cleared", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(bootstrapJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(dbSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(GetCronJob(cronJobName).Spec.JobTemplate.Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				keystone := GetKeystoneAPI(keystoneAPIName)
				emptyNodeSelector := map[string]string{}
				keystone.Spec.NodeSelector = &emptyNodeSelector
				g.Expect(k8sClient.Update(ctx, keystone)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				th.SimulateJobSuccess(dbSyncJobName)
				th.SimulateJobSuccess(bootstrapJobName)
				th.SimulateDeploymentReplicaReady(deploymentName)
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetJob(bootstrapJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetJob(dbSyncJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(GetCronJob(cronJobName).Spec.JobTemplate.Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("removes nodeSelector from resource specs when nilled", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(bootstrapJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(dbSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(GetCronJob(cronJobName).Spec.JobTemplate.Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				keystone := GetKeystoneAPI(keystoneAPIName)
				keystone.Spec.NodeSelector = nil
				g.Expect(k8sClient.Update(ctx, keystone)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				th.SimulateJobSuccess(dbSyncJobName)
				th.SimulateJobSuccess(bootstrapJobName)
				th.SimulateDeploymentReplicaReady(deploymentName)
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetJob(bootstrapJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetJob(dbSyncJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(GetCronJob(cronJobName).Spec.JobTemplate.Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})
	})

	When("A KeystoneAPI is created with HttpdCustomization.OverrideSecret", func() {
		BeforeEach(func() {
			customServiceConfigSecretName := types.NamespacedName{Name: "foo", Namespace: namespace}
			customConfig := []byte(`OIDCResponseType "id_token"
OIDCMemCacheServers "{{ .MemcachedServers }}"
OIDCRedirectURI "{{ .KeystoneEndpointPublic }}/v3/auth/OS-FEDERATION/websso/openid"`)
			th.CreateSecret(
				customServiceConfigSecretName,
				map[string][]byte{
					"bar.conf": customConfig,
				},
			)

			spec := GetDefaultKeystoneAPISpec()
			spec["httpdCustomization"] = map[string]any{
				"customConfigSecret": customServiceConfigSecretName.Name,
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

		It("it renders the custom template and adds it to the keystone-config-data secret", func() {
			scrt := th.GetSecret(keystoneAPIConfigDataName)
			Expect(scrt).ShouldNot(BeNil())
			Expect(scrt.Data).Should(HaveKey(common.TemplateParameters))
			configData := string(scrt.Data[common.TemplateParameters])
			memcachedServers := fmt.Sprintf("memcached-0.memcached.%s.svc:11211,memcached-1.memcached.%s.svc:11211,memcached-2.memcached.%s.svc:11211",
				namespace, namespace, namespace)
			Expect(configData).Should(ContainSubstring(fmt.Sprintf("MemcachedServers: %s", memcachedServers)))

			for _, cfg := range []string{"httpd_custom_internal_bar.conf", "httpd_custom_public_bar.conf"} {
				Expect(scrt.Data).Should(HaveKey(cfg))
				configData := string(scrt.Data[cfg])
				Expect(configData).Should(ContainSubstring("OIDCResponseType \"id_token\""))
				Expect(configData).Should(ContainSubstring(fmt.Sprintf("OIDCMemCacheServers \"%s\"", memcachedServers)))
				Expect(configData).Should(ContainSubstring(
					fmt.Sprintf("OIDCRedirectURI \"http://keystone-public.%s.svc:5000/v3/auth/OS-FEDERATION/websso/openid\"", namespace)))
			}
		})
	})
	When("Keystone CR is built with ExtraMounts", func() {
		var keystoneExtraMountsSecretName, keystoneExtraMountsPath string
		BeforeEach(func() {
			spec := GetDefaultKeystoneAPISpec()
			keystoneExtraMountsPath = "/var/log/foo"
			keystoneExtraMountsSecretName = "foo"
			spec["extraMounts"] = GetExtraMounts(keystoneExtraMountsSecretName, keystoneExtraMountsPath)
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
		It("Check extraMounts of the resulting Deployment", func() {
			th.SimulateDeploymentReplicaReady(deploymentName)
			// Get Keystone Deployment
			dp := th.GetDeployment(deploymentName)
			// Check the resulting deployment fields
			Expect(dp.Spec.Template.Spec.Volumes).To(HaveLen(5))
			Expect(dp.Spec.Template.Spec.Containers).To(HaveLen(1))
			// Get the keystone-api container
			container := dp.Spec.Template.Spec.Containers[0]
			// Fail if keystone-api doesn't have the right number of VolumeMounts
			// entries
			Expect(container.VolumeMounts).To(HaveLen(6))
			// Inspect VolumeMounts and make sure we have the Foo MountPath
			// provided through extraMounts
			th.AssertVolumeMountPathExists("foo",
				keystoneExtraMountsPath, "", container.VolumeMounts)
		})
	})
	When("A KeystoneAPI is created with a federatedRealmConfig", func() {
		const (
			inputSecretName  = "federation-test-secret"
			mountPath        = "/var/lib/config-data/default/multirealm-federation"
			multiRealmSecret = "keystone-multirealm-federation-secret"
		)

		BeforeEach(func() {
			raw := `{
          "idp1.conf": "CONTENT_%ONE%",
          "idp2.conf": "CONTENT-TWO"
        }`
			th.CreateSecret(
				types.NamespacedName{Namespace: namespace, Name: inputSecretName},
				map[string][]byte{"federation-config.json": []byte(raw)},
			)

			spec := GetDefaultKeystoneAPISpec()
			spec["federatedRealmConfig"] = inputSecretName
			spec["federationMountPath"] = mountPath
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

		It("should build the multi-realm Secret with the two files intact", func() {
			multi := th.GetSecret(types.NamespacedName{Namespace: namespace, Name: multiRealmSecret})
			Expect(multi).NotTo(BeNil(), "expected Secret %s to exist", multiRealmSecret)

			Expect(multi.Data).To(HaveKey("_filenames.json"))
			Expect(multi.Data).To(HaveKey("0"))
			Expect(multi.Data).To(HaveKey("1"))

			var content1, content2 string
			Expect(json.Unmarshal(multi.Data["0"], &content1)).To(Succeed(), "key '0' should be valid JSON")
			Expect(json.Unmarshal(multi.Data["1"], &content2)).To(Succeed(), "key '1' should be valid JSON")

			Expect(content1).To(Equal("CONTENT_%ONE%"))
			Expect(content2).To(Equal("CONTENT-TWO"))

			var files []string
			Expect(json.Unmarshal(multi.Data["_filenames.json"], &files)).To(Succeed())
			Expect(files).To(Equal([]string{"idp1.conf", "idp2.conf"}))

			d := th.GetDeployment(deploymentName)
			container := d.Spec.Template.Spec.Containers[0]
			for idx, filename := range []string{"idp1.conf", "idp2.conf"} {
				expectedPath := filepath.Join(mountPath, filename)
				th.AssertVolumeMountPathExists(
					fmt.Sprintf("federation-realm-volume%d", idx),
					expectedPath, "", container.VolumeMounts,
				)
			}
		})
	})

	When("A KeystoneAPI is created with quorum queues disabled", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, infra.CreateTransportURLSecret(namespace, "rabbitmq-secret", false))
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

		It("should not include quorum queue configuration in keystone.conf", func() {
			scrt := th.GetSecret(keystoneAPIConfigDataName)
			configData := string(scrt.Data["keystone.conf"])
			Expect(configData).NotTo(ContainSubstring("rabbit_quorum_queue"))
			Expect(configData).NotTo(ContainSubstring("rabbit_transient_quorum_queue"))
			Expect(configData).NotTo(ContainSubstring("amqp_durable_queues"))
		})
	})

	When("A KeystoneAPI is created with quorum queues enabled", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, infra.CreateTransportURLSecret(namespace, "rabbitmq-secret", true))
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

		It("should include quorum queue configuration in keystone.conf", func() {
			scrt := th.GetSecret(keystoneAPIConfigDataName)
			configData := string(scrt.Data["keystone.conf"])
			Expect(configData).To(ContainSubstring("rabbit_quorum_queue=true"))
			Expect(configData).To(ContainSubstring("rabbit_transient_quorum_queue=true"))
			Expect(configData).To(ContainSubstring("amqp_durable_queues=true"))
		})
	})

	When("A KeystoneAPI is created with quorum queues disabled and then updated to enable them", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()))
			DeferCleanup(
				k8sClient.Delete, ctx, infra.CreateTransportURLSecret(namespace, "rabbitmq-secret", false))
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

		It("should correctly update keystone.conf when quorum queues are enabled", func() {
			// First, verify the initial state - quorum queues disabled
			scrt := th.GetSecret(keystoneAPIConfigDataName)
			configData := string(scrt.Data["keystone.conf"])
			Expect(configData).NotTo(ContainSubstring("rabbit_quorum_queue"))
			Expect(configData).NotTo(ContainSubstring("rabbit_transient_quorum_queue"))
			Expect(configData).NotTo(ContainSubstring("amqp_durable_queues"))

			// Update the RabbitMQ secret to enable quorum queues
			rabbitmqSecret := th.GetSecret(types.NamespacedName{
				Name:      "rabbitmq-secret",
				Namespace: namespace,
			})

			// Update the quorum queue setting in the secret
			Eventually(func(g Gomega) {
				rabbitmqSecret.Data["quorumqueues"] = []byte("true")
				g.Expect(k8sClient.Update(ctx, &rabbitmqSecret)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			// Wait for the configuration to be updated in keystone.conf
			Eventually(func(g Gomega) {
				scrt = th.GetSecret(keystoneAPIConfigDataName)
				configData = string(scrt.Data["keystone.conf"])
				g.Expect(configData).To(ContainSubstring("rabbit_quorum_queue=true"))
				g.Expect(configData).To(ContainSubstring("rabbit_transient_quorum_queue=true"))
				g.Expect(configData).To(ContainSubstring("amqp_durable_queues=true"))
			}, timeout, interval).Should(Succeed())
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

	mariadbSuite.RunURLAssertSuite(func(_ types.NamespacedName, username string, password string) {
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

	When("Keystone is configured for MTLS memcached auth", func() {
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

			// Create Memcached with MTLS auth
			memcachedSpec := infra.GetDefaultMemcachedSpec()
			DeferCleanup(infra.DeleteMemcached, infra.CreateMTLSMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMTLSMemcachedReady(types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			})

			th.SimulateJobSuccess(dbSyncJobName)
			th.SimulateJobSuccess(bootstrapJobName)
			th.SimulateDeploymentReplicaReady(deploymentName)
		})

		It("should complete dbsync, bootstrap and deployment with MTLS configuration", func() {
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
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
				condition.BootstrapReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

			// Verify keystone configuration contains MTLS certificate paths
			scrt := th.GetSecret(keystoneAPIConfigDataName)
			configData := string(scrt.Data["keystone.conf"])
			Expect(configData).To(ContainSubstring("tls_certfile=/etc/pki/tls/certs/mtls.crt"))
			Expect(configData).To(ContainSubstring("tls_keyfile=/etc/pki/tls/private/mtls.key"))
			Expect(configData).To(ContainSubstring("tls_cafile=/etc/pki/tls/certs/mtls-ca.crt"))
		})
	})

	When("A KeystoneAPI is created with region specified", func() {
		BeforeEach(func() {
			spec := GetDefaultKeystoneAPISpec()
			spec["region"] = "test-region"
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

		It("should set region in status", func() {
			Eventually(func(g Gomega) {
				instance := keystone.GetKeystoneAPI(keystoneAPIName)
				g.Expect(instance).NotTo(BeNil())
				g.Expect(instance.Status.Region).To(Equal("test-region"))
				g.Expect(instance.Spec.Region).To(Equal("test-region"))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("A KeystoneAPI is created with externalKeystoneAPI enabled", func() {
		BeforeEach(func() {
			spec := GetDefaultKeystoneAPISpec()
			spec["externalKeystoneAPI"] = true
			spec["region"] = "external-region"
			spec["override"] = map[string]any{
				"service": map[string]any{
					"public": map[string]any{
						"endpointURL": "https://external-keystone.example.com:5000",
					},
					"internal": map[string]any{
						"endpointURL": "https://internal-keystone.example.com:5000",
					},
				},
			}
			DeferCleanup(
				k8sClient.Delete, ctx, CreateKeystoneAPISecret(namespace, SecretName))
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, spec))
		})

		It("should set region and endpoints in status without deploying resources", func() {
			Eventually(func(g Gomega) {
				instance := keystone.GetKeystoneAPI(keystoneAPIName)
				g.Expect(instance).NotTo(BeNil())
				g.Expect(instance.Spec.ExternalKeystoneAPI).To(BeTrue())
				g.Expect(instance.Status.Region).To(Equal("external-region"))
				g.Expect(instance.Status.APIEndpoints).To(HaveKeyWithValue("public", "https://external-keystone.example.com:5000"))
				g.Expect(instance.Status.APIEndpoints).To(HaveKeyWithValue("internal", "https://internal-keystone.example.com:5000"))
				g.Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
			}, timeout, interval).Should(Succeed())

			// Verify ReadyCondition is set for external keystone
			// Note: DBReadyCondition and DeploymentReadyCondition are not set for external Keystone API
			// as they are not relevant when using an external service
			th.ExpectCondition(
				keystoneAPIName,
				ConditionGetterFunc(KeystoneConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// Verify no deployment is created
			Eventually(func() bool {
				err := k8sClient.Get(ctx, deploymentName, &appsv1.Deployment{})
				return k8s_errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	When("an ApplicationCredential CR is created", func() {
		BeforeEach(func() {
			acName = types.NamespacedName{Name: "ac-test-service", Namespace: namespace}
			serviceUserSecret = types.NamespacedName{Name: "osp-secret", Namespace: namespace}

			th.CreateSecret(serviceUserSecret,
				map[string][]byte{"ServicePassword": []byte("service-password")})

			raw := map[string]interface{}{
				"apiVersion": "keystone.openstack.org/v1beta1",
				"kind":       "KeystoneApplicationCredential",
				"metadata": map[string]interface{}{
					"name":      acName.Name,
					"namespace": acName.Namespace,
				},
				"spec": map[string]interface{}{
					"userName":         "test-service",
					"secret":           serviceUserSecret.Name,
					"passwordSelector": "ServicePassword",
					"expirationDays":   14,
					"gracePeriodDays":  7,
					"roles":            []string{"admin", "service"},
					"unrestricted":     false,
				},
			}
			th.CreateUnstructured(raw)
		})

		It("should be recognized by the controller, add a finalizer and initialize Conditions", func() {
			Eventually(func(g Gomega) {
				ac := &keystonev1.KeystoneApplicationCredential{}
				g.Expect(k8sClient.Get(ctx, acName, ac)).To(Succeed())

				g.Expect(ac.Finalizers).To(ContainElement("openstack.org/applicationcredential"))

				g.Expect(ac.Status.Conditions).NotTo(BeNil())
				found := false
				for _, c := range ac.Status.Conditions {
					if c.Type == keystonev1.KeystoneAPIReadyCondition {
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})

	When("a multiple ApplicationCredential CRs are created", func() {
		It("should initialize properly with various configurations and handle missing KeystoneAPI", func() {
			serviceUserSecret := types.NamespacedName{Name: "osp-secret", Namespace: namespace}
			th.CreateSecret(serviceUserSecret, map[string][]byte{
				"ServicePassword": []byte("service-password"),
			})

			basicACName := types.NamespacedName{Name: "ac-basic-config", Namespace: namespace}
			CreateACWithSpec(basicACName, map[string]interface{}{
				"userName":         "test-service",
				"secret":           serviceUserSecret.Name,
				"passwordSelector": "ServicePassword",
				"expirationDays":   30,
				"gracePeriodDays":  7,
				"roles":            []string{"service"},
				"unrestricted":     false,
			})

			rulesACName := types.NamespacedName{Name: "ac-with-rules", Namespace: namespace}
			CreateACWithSpec(rulesACName, map[string]interface{}{
				"userName":         "test-service",
				"secret":           serviceUserSecret.Name,
				"passwordSelector": "ServicePassword",
				"expirationDays":   60,
				"gracePeriodDays":  14,
				"roles":            []string{"admin", "service"},
				"unrestricted":     false,
				"accessRules": []map[string]interface{}{
					{
						"service": "compute",
						"path":    "/servers",
						"method":  "GET",
					},
					{
						"service": "image",
						"path":    "/images",
						"method":  "GET",
					},
				},
			})

			unrestrictedACName := types.NamespacedName{Name: "ac-unrestricted", Namespace: namespace}
			CreateACWithSpec(unrestrictedACName, map[string]interface{}{
				"userName":         "test-service",
				"secret":           serviceUserSecret.Name,
				"passwordSelector": "ServicePassword",
				"expirationDays":   90,
				"gracePeriodDays":  30,
				"roles":            []string{"admin"},
				"unrestricted":     true,
			})

			Eventually(func(g Gomega) {
				basicAC := GetApplicationCredential(basicACName)
				g.Expect(basicAC.Finalizers).To(ContainElement("openstack.org/applicationcredential"))
				g.Expect(basicAC.Status.Conditions).NotTo(BeNil())
				g.Expect(basicAC.Spec.ExpirationDays).To(Equal(30))
				g.Expect(basicAC.Spec.GracePeriodDays).To(Equal(7))
				g.Expect(basicAC.Spec.Roles).To(Equal([]string{"service"}))
				g.Expect(basicAC.Spec.Unrestricted).To(BeFalse())

				rulesAC := GetApplicationCredential(rulesACName)
				g.Expect(rulesAC.Finalizers).To(ContainElement("openstack.org/applicationcredential"))
				g.Expect(rulesAC.Status.Conditions).NotTo(BeNil())
				g.Expect(rulesAC.Spec.AccessRules).To(HaveLen(2))
				g.Expect(rulesAC.Spec.AccessRules[0].Service).To(Equal("compute"))
				g.Expect(rulesAC.Spec.AccessRules[1].Service).To(Equal("image"))
				g.Expect(rulesAC.Spec.Roles).To(ContainElements("admin", "service"))

				unrestrictedAC := GetApplicationCredential(unrestrictedACName)
				g.Expect(unrestrictedAC.Finalizers).To(ContainElement("openstack.org/applicationcredential"))
				g.Expect(unrestrictedAC.Status.Conditions).NotTo(BeNil())
				g.Expect(unrestrictedAC.Spec.Unrestricted).To(BeTrue())
				g.Expect(unrestrictedAC.Spec.ExpirationDays).To(Equal(90))
				g.Expect(unrestrictedAC.Spec.GracePeriodDays).To(Equal(30))

				for _, acName := range []types.NamespacedName{basicACName, rulesACName, unrestrictedACName} {
					ac := GetApplicationCredential(acName)
					keystoneCondition := ac.Status.Conditions.Get(keystonev1.KeystoneAPIReadyCondition)
					g.Expect(keystoneCondition).NotTo(BeNil())
					g.Expect(keystoneCondition.Status).NotTo(Equal(corev1.ConditionTrue), "Should wait for KeystoneAPI")
				}
			}, timeout, interval).Should(Succeed())
		})

		It("should handle deletion with finalizer cleanup", func() {
			serviceUserSecret := types.NamespacedName{Name: "osp-secret", Namespace: namespace}
			th.CreateSecret(serviceUserSecret, map[string][]byte{
				"ServicePassword": []byte("service-password"),
			})

			deleteACName := types.NamespacedName{Name: "ac-delete-test", Namespace: namespace}
			CreateACWithSpec(deleteACName, map[string]interface{}{
				"userName":         "test-service",
				"secret":           serviceUserSecret.Name,
				"passwordSelector": "ServicePassword",
				"expirationDays":   30,
				"gracePeriodDays":  7,
				"roles":            []string{"service"},
			})

			Eventually(func(g Gomega) {
				ac := GetApplicationCredential(deleteACName)
				g.Expect(ac.Finalizers).To(ContainElement("openstack.org/applicationcredential"))
			}, timeout, interval).Should(Succeed())

			acInstance := GetApplicationCredential(deleteACName)
			err := k8sClient.Delete(ctx, acInstance)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				ac := &keystonev1.KeystoneApplicationCredential{}
				err := k8sClient.Get(ctx, deleteACName, ac)
				if k8s_errors.IsNotFound(err) {
					return
				}
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ac.DeletionTimestamp).NotTo(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("should validate expiration vs grace period constraints", func() {
			serviceUserSecret := types.NamespacedName{Name: "osp-secret", Namespace: namespace}
			th.CreateSecret(serviceUserSecret, map[string][]byte{
				"ServicePassword": []byte("service-password"),
			})

			validACName := types.NamespacedName{Name: "ac-valid-periods", Namespace: namespace}
			CreateACWithSpec(validACName, map[string]interface{}{
				"userName":         "test-service",
				"secret":           serviceUserSecret.Name,
				"passwordSelector": "ServicePassword",
				"expirationDays":   30,
				"gracePeriodDays":  7,
				"roles":            []string{"service"},
			})

			Eventually(func(g Gomega) {
				ac := GetApplicationCredential(validACName)
				g.Expect(ac.Spec.ExpirationDays).To(Equal(30))
				g.Expect(ac.Spec.GracePeriodDays).To(Equal(7))
				g.Expect(ac.Finalizers).To(ContainElement("openstack.org/applicationcredential"))
			}, timeout, interval).Should(Succeed())

			invalidACName := types.NamespacedName{Name: "ac-invalid-periods", Namespace: namespace}
			invalidSpec := map[string]interface{}{
				"userName":         "test-service",
				"secret":           serviceUserSecret.Name,
				"passwordSelector": "ServicePassword",
				"expirationDays":   7,
				"gracePeriodDays":  10,
				"roles":            []string{"service"},
			}

			raw := map[string]interface{}{
				"apiVersion": "keystone.openstack.org/v1beta1",
				"kind":       "KeystoneApplicationCredential",
				"metadata": map[string]interface{}{
					"name":      invalidACName.Name,
					"namespace": invalidACName.Namespace,
				},
				"spec": invalidSpec,
			}

			obj := &unstructured.Unstructured{}
			obj.SetUnstructuredContent(raw)
			err := k8sClient.Create(ctx, obj)
			Expect(err).To(HaveOccurred(), "Expected creation to fail due to validation constraint")
			Expect(err.Error()).To(ContainSubstring("gracePeriodDays must be smaller than expirationDays"))
		})
	})
})
