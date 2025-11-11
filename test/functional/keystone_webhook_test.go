/*
Copyright 2025.

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

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("KeystoneAPI webhook", func() {
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
				ContainSubstring("rabbitMqClusterName is deprecated and cannot be changed"))
			g.Expect(statusError.ErrStatus.Message).To(
				ContainSubstring("Please use notificationsBus.cluster instead"))
		}, timeout, interval).Should(Succeed())
	})
})
