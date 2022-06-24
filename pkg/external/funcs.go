/*

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

package external

import (
	"context"
	"fmt"

	"github.com/openstack-k8s-operators/lib-common/pkg/helper"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
)

//
// GetKeystoneAPI - get keystoneAPI object in namespace
//
func GetKeystoneAPI(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
	labelSelector map[string]string,
) (*keystonev1.KeystoneAPI, error) {
	keystoneList := &keystonev1.KeystoneAPIList{}

	listOpts := []client.ListOption{
		client.InNamespace(namespace),
	}

	if len(labelSelector) > 0 {
		labels := client.MatchingLabels(labelSelector)
		listOpts = append(listOpts, labels)
	}

	err := h.GetClient().List(ctx, keystoneList, listOpts...)
	if err != nil {
		return nil, err
	}

	if len(keystoneList.Items) > 1 {
		return nil, fmt.Errorf("more then one KeystoneAPI object found in namespace %s", namespace)
	}

	return &keystoneList.Items[0], nil
}
