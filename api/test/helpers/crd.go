/*
Copyright 2023 Red Hat
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

package helpers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/onsi/gomega"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	base "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
)

// TestHelper is a collection of helpers for testing operators. It extends the
// generic TestHelper from modules/test.
type TestHelper struct {
	*base.TestHelper
}

// NewTestHelper returns a TestHelper
func NewTestHelper(
	ctx context.Context,
	k8sClient client.Client,
	timeout time.Duration,
	interval time.Duration,
	logger logr.Logger,
) *TestHelper {
	helper := &TestHelper{}
	helper.TestHelper = base.NewTestHelper(ctx, k8sClient, timeout, interval, logger)
	return helper
}

// CreateKeystoneAPI creates a new KeystoneAPI instance with the specified namespace in the Kubernetes cluster.
//
// Example usage:
//
//	keystoneAPI := th.CreateKeystoneAPI(namespace)
//	DeferCleanup(th.DeleteKeystoneAPI, keystoneAPI)
func (th *TestHelper) CreateKeystoneAPI(namespace string) types.NamespacedName {
	keystone := &keystonev1.KeystoneAPI{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keystone.openstack.org/v1beta1",
			Kind:       "KeystoneAPI",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "keystone-" + uuid.New().String(),
			Namespace: namespace,
		},
		Spec: keystonev1.KeystoneAPISpec{},
	}

	gomega.Expect(th.K8sClient.Create(th.Ctx, keystone.DeepCopy())).Should(gomega.Succeed())
	name := types.NamespacedName{Namespace: namespace, Name: keystone.Name}

	// the Status field needs to be written via a separate client
	keystone = th.GetKeystoneAPI(name)
	keystone.Status = keystonev1.KeystoneAPIStatus{
		APIEndpoints: map[string]string{
			"public":   "http://keystone-public-openstack.testing",
			"internal": "http://keystone-internal.openstack.svc:5000",
		},
	}
	gomega.Expect(th.K8sClient.Status().Update(th.Ctx, keystone.DeepCopy())).Should(gomega.Succeed())

	th.Logger.Info("KeystoneAPI created", "KeystoneAPI", name)
	return name
}

// CreateKeystoneAPIWithFixture creates a KeystoneAPI CR and configures
// its endpoints to point to the KeystoneAPIFixture that simulate the
// keystone-api behavior.
func (th *TestHelper) CreateKeystoneAPIWithFixture(
	namespace string, fixture *KeystoneAPIFixture,
) types.NamespacedName {
	n := "keystone-" + uuid.New().String()

	th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: n + "-secret"},
		map[string][]byte{
			"admin-password": []byte("admin-password"),
		},
	)

	keystone := &keystonev1.KeystoneAPI{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keystone.openstack.org/v1beta1",
			Kind:       "KeystoneAPI",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      n,
			Namespace: namespace,
		},
		Spec: keystonev1.KeystoneAPISpec{
			Secret:    n + "-secret",
			AdminUser: "admin",
			PasswordSelectors: keystonev1.PasswordSelector{
				Admin: "admin-password",
			},
		},
	}

	gomega.Expect(th.K8sClient.Create(th.Ctx, keystone.DeepCopy())).Should(gomega.Succeed())
	name := types.NamespacedName{Namespace: namespace, Name: keystone.Name}

	// the Status field needs to be written via a separate client
	keystone = th.GetKeystoneAPI(name)
	keystone.Status = keystonev1.KeystoneAPIStatus{
		APIEndpoints: map[string]string{
			"public":   fixture.Endpoint(),
			"internal": "http://keystone-internal.openstack.svc:5000",
		},
	}
	gomega.Expect(th.K8sClient.Status().Update(th.Ctx, keystone.DeepCopy())).Should(gomega.Succeed())

	th.Logger.Info("KeystoneAPI created", "KeystoneAPI", name)
	return name
}

// DeleteKeystoneAPI deletes a KeystoneAPI resource from the Kubernetes cluster.
//
// # After the deletion, the function checks again if the KeystoneAPI is successfully deleted
//
// Example usage:
//
//	keystoneAPI := th.CreateKeystoneAPI(namespace)
//	DeferCleanup(th.DeleteKeystoneAPI, keystoneAPI)
func (th *TestHelper) DeleteKeystoneAPI(name types.NamespacedName) {
	gomega.Eventually(func(g gomega.Gomega) {
		keystone := &keystonev1.KeystoneAPI{}
		err := th.K8sClient.Get(th.Ctx, name, keystone)
		// if it is already gone that is OK
		if k8s_errors.IsNotFound(err) {
			return
		}
		g.Expect(err).NotTo(gomega.HaveOccurred())

		g.Expect(th.K8sClient.Delete(th.Ctx, keystone)).Should(gomega.Succeed())

		err = th.K8sClient.Get(th.Ctx, name, keystone)
		g.Expect(k8s_errors.IsNotFound(err)).To(gomega.BeTrue())
	}, th.Timeout, th.Interval).Should(gomega.Succeed())
}

// GetKeystoneAPI retrieves a KeystoneAPI resource.
//
// The function returns a pointer to the retrieved KeystoneAPI resource.
// example usage:
//
//	  keystoneAPIName := th.CreateKeystoneAPI(novaNames.NovaName.Namespace)
//		 DeferCleanup(th.DeleteKeystoneAPI, keystoneAPIName)
//		 keystoneAPI := th.GetKeystoneAPI(keystoneAPIName)
func (th *TestHelper) GetKeystoneAPI(name types.NamespacedName) *keystonev1.KeystoneAPI {
	instance := &keystonev1.KeystoneAPI{}
	gomega.Eventually(func(g gomega.Gomega) {
		g.Expect(th.K8sClient.Get(th.Ctx, name, instance)).Should(gomega.Succeed())
	}, th.Timeout, th.Interval).Should(gomega.Succeed())
	return instance
}

// SimulateKeystoneAPIReady simulates the readiness of a KeystoneAPI
// resource by setting the Ready condition of the KeystoneAPI to true
//
// Example usage:
// th.SimulateKeystoneAPIReady(keystoneAPIName)
func (th *TestHelper) SimulateKeystoneAPIReady(name types.NamespacedName) {
	gomega.Eventually(func(g gomega.Gomega) {
		service := th.GetKeystoneAPI(name)
		service.Status.Conditions.MarkTrue(condition.ReadyCondition, "Ready")
		g.Expect(th.K8sClient.Status().Update(th.Ctx, service)).To(gomega.Succeed())
	}, th.Timeout, th.Interval).Should(gomega.Succeed())
	th.Logger.Info("Simulated GetKeystoneAPI ready", "on", name)
}

// GetKeystoneService function retrieves and returns the KeystoneService resource
//
// Example usage:
//
//	keystoneServiceName := th.CreateKeystoneService(namespace)
func (th *TestHelper) GetKeystoneService(name types.NamespacedName) *keystonev1.KeystoneService {
	instance := &keystonev1.KeystoneService{}
	gomega.Eventually(func(g gomega.Gomega) {
		g.Expect(th.K8sClient.Get(th.Ctx, name, instance)).Should(gomega.Succeed())
	}, th.Timeout, th.Interval).Should(gomega.Succeed())
	return instance
}

// SimulateKeystoneServiceReady simulates the readiness of a KeystoneService
// resource by setting the Ready condition of the KeystoneService to true
//
// Example usage:
// keystoneServiceName := th.CreateKeystoneService(namespace)
func (th *TestHelper) SimulateKeystoneServiceReady(name types.NamespacedName) {
	gomega.Eventually(func(g gomega.Gomega) {
		service := th.GetKeystoneService(name)
		service.Status.Conditions.MarkTrue(condition.ReadyCondition, "Ready")
		g.Expect(th.K8sClient.Status().Update(th.Ctx, service)).To(gomega.Succeed())
	}, th.Timeout, th.Interval).Should(gomega.Succeed())
	th.Logger.Info("Simulated KeystoneService ready", "on", name)
}

// AssertKeystoneServiceDoesNotExist ensures the KeystoneService resource does not exist in a k8s cluster.
func (th *TestHelper) AssertKeystoneServiceDoesNotExist(name types.NamespacedName) {
	instance := &keystonev1.KeystoneService{}
	gomega.Eventually(func(g gomega.Gomega) {
		err := th.K8sClient.Get(th.Ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(gomega.BeTrue())
	}, th.Timeout, th.Interval).Should(gomega.Succeed())
}

// GetKeystoneEndpoint retrieves a KeystoneEndpoint resource from the Kubernetes cluster.
//
// Example usage:
//
//	keystoneEndpointName := th.CreateKeystoneEndpoint(namespace)
func (th *TestHelper) GetKeystoneEndpoint(name types.NamespacedName) *keystonev1.KeystoneEndpoint {
	instance := &keystonev1.KeystoneEndpoint{}
	gomega.Eventually(func(g gomega.Gomega) {
		g.Expect(th.K8sClient.Get(th.Ctx, name, instance)).Should(gomega.Succeed())
	}, th.Timeout, th.Interval).Should(gomega.Succeed())
	return instance
}

// SimulateKeystoneEndpointReady function retrieves the KeystoneEndpoint resource and
// simulates a KeystoneEndpoint resource being marked as ready.
//
// Example usage:
//
//	keystoneEndpointName := th.CreateKeystoneEndpoint(namespace)
//	th.SimulateKeystoneEndpointReady(keystoneEndpointName)
func (th *TestHelper) SimulateKeystoneEndpointReady(name types.NamespacedName) {
	gomega.Eventually(func(g gomega.Gomega) {
		endpoint := th.GetKeystoneEndpoint(name)
		endpoint.Status.Conditions.MarkTrue(condition.ReadyCondition, "Ready")
		g.Expect(th.K8sClient.Status().Update(th.Ctx, endpoint)).To(gomega.Succeed())
	}, th.Timeout, th.Interval).Should(gomega.Succeed())
	th.Logger.Info("Simulated KeystoneEndpoint ready", "on", name)
}

// AssertKeystoneEndpointDoesNotExist ensures the KeystoneEndpoint resource does not exist in a k8s cluster.
func (th *TestHelper) AssertKeystoneEndpointDoesNotExist(name types.NamespacedName) {
	instance := &keystonev1.KeystoneEndpoint{}
	gomega.Eventually(func(g gomega.Gomega) {
		err := th.K8sClient.Get(th.Ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(gomega.BeTrue())
	}, th.Timeout, th.Interval).Should(gomega.Succeed())
}
