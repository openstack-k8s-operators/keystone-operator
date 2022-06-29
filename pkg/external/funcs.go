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

	"github.com/openstack-k8s-operators/lib-common/pkg/common"
	"github.com/openstack-k8s-operators/lib-common/pkg/condition"
	"github.com/openstack-k8s-operators/lib-common/pkg/helper"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gophercloud "github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
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

	if len(keystoneList.Items) == 0 {
		return nil, k8s_errors.NewNotFound(
			appsv1.Resource("KeystoneAPI"),
			fmt.Sprintf("No KeystoneAPI object found in namespace %s", namespace),
		)
	}

	return &keystoneList.Items[0], nil
}

//
// GetAdminServiceClient - get an admin serviceClient for the keystoneAPI instance
//
func GetAdminServiceClient(
	ctx context.Context,
	h *helper.Helper,
	keystoneAPI *keystonev1.KeystoneAPI,
) (*gophercloud.ServiceClient, condition.Condition, ctrl.Result, error) {
	// get public endpoint as authurl from keystone instance
	authURL, err := keystoneAPI.GetEndpoint(common.EndpointPublic)
	if err != nil {
		return nil, condition.Condition{}, ctrl.Result{}, err
	}

	// get the password of the admin user from Spec.Secret
	// using PasswordSelectors.Admin
	authPassword, cond, ctrlResult, err := common.GetDataFromSecret(
		ctx,
		h,
		keystoneAPI.Spec.Secret,
		10,
		keystoneAPI.Spec.PasswordSelectors.Admin)
	if err != nil {
		return nil, cond, ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		return nil, cond, ctrlResult, nil
	}

	opts := gophercloud.AuthOptions{
		IdentityEndpoint: authURL,
		Username:         keystoneAPI.Spec.AdminUser,
		Password:         authPassword,
		TenantName:       keystoneAPI.Spec.AdminProject,
		DomainName:       "Default",
	}

	provider, err := openstack.AuthenticatedClient(opts)
	if err != nil {
		return nil, condition.Condition{}, ctrl.Result{}, err
	}
	endpointOpts := gophercloud.EndpointOpts{Type: "identity", Region: keystoneAPI.Spec.Region}
	identityClient, err := openstack.NewIdentityV3(provider, endpointOpts)
	if err != nil {
		return nil, condition.Condition{}, ctrl.Result{}, err
	}

	return identityClient, condition.Condition{}, ctrl.Result{}, nil
}
