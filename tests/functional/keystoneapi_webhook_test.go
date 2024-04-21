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
	"os"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports
	"k8s.io/apimachinery/pkg/types"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
)

var _ = Describe("KeystoneAPI Webhook", func() {

	var keystoneAPIName types.NamespacedName

	BeforeEach(func() {

		keystoneAPIName = types.NamespacedName{
			Name:      "keystone",
			Namespace: namespace,
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
			keystoneAPISpec := GetDefaultKeystoneAPISpec()
			keystoneAPISpec["containerImage"] = "api-container-image"
			DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, keystoneAPISpec))
		})

		It("should use the given values", func() {
			KeystoneAPI := GetKeystoneAPI(keystoneAPIName)
			Expect(KeystoneAPI.Spec.ContainerImage).Should(Equal(
				"api-container-image",
			))
		})
	})
})
