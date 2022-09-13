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

package v1beta1

import (
	"context"
	"fmt"
	"time"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openstack "github.com/openstack-k8s-operators/lib-common/modules/openstack"
	appsv1 "k8s.io/api/apps/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
) (*KeystoneAPI, error) {
	keystoneList := &KeystoneAPIList{}

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
	keystoneAPI *KeystoneAPI,
) (*openstack.OpenStack, ctrl.Result, error) {
	// get public endpoint as authurl from keystone instance
	authURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointPublic)
	if err != nil {
		return nil, ctrl.Result{}, err
	}

	// get the password of the admin user from Spec.Secret
	// using PasswordSelectors.Admin
	authPassword, ctrlResult, err := secret.GetDataFromSecret(
		ctx,
		h,
		keystoneAPI.Spec.Secret,
		10,
		keystoneAPI.Spec.PasswordSelectors.Admin)
	if err != nil {
		return nil, ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		return nil, ctrlResult, nil
	}

	os, err := openstack.NewOpenStack(
		h.GetLogger(),
		openstack.AuthOpts{
			AuthURL:    authURL,
			Username:   keystoneAPI.Spec.AdminUser,
			Password:   authPassword,
			TenantName: keystoneAPI.Spec.AdminProject,
			DomainName: "Default",
			Region:     keystoneAPI.Spec.Region,
		})
	if err != nil {
		return nil, ctrl.Result{}, err
	}

	return os, ctrl.Result{}, nil
}

// NewKeystoneService returns an initialized NewKeystoneService.
func NewKeystoneService(
	spec KeystoneServiceSpec,
	namespace string,
	labels map[string]string,
	timeout int,
) *KeystoneServiceHelper {
	service := &KeystoneService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.ServiceName,
			Namespace: namespace,
		},
		Spec: spec,
	}

	return &KeystoneServiceHelper{
		service: service,
		timeout: timeout,
		labels:  labels,
	}
}

// CreateOrPatch - creates or patches a KeystoneService, reconciles after Xs if object won't exist.
func (ks *KeystoneServiceHelper) CreateOrPatch(
	ctx context.Context,
	h *helper.Helper,
) (ctrl.Result, error) {
	service := &KeystoneService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ks.service.Name,
			Namespace: ks.service.Namespace,
		},
	}

	// add finalizer
	controllerutil.AddFinalizer(service, h.GetFinalizer())

	op, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), service, func() error {
		service.Spec = ks.service.Spec
		service.Labels = util.MergeStringMaps(service.Labels, ks.service.Labels)

		err := controllerutil.SetControllerReference(h.GetBeforeObject(), service, h.GetScheme())
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			h.GetLogger().Info(fmt.Sprintf("KeystoneService %s not found, reconcile in %ds", service.Name, ks.timeout))
			return ctrl.Result{RequeueAfter: time.Duration(ks.timeout) * time.Second}, nil
		}
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		h.GetLogger().Info(fmt.Sprintf("KeystoneService %s - %s", service.Name, op))
	}

	// update the service object of the KeystoneService type
	ks.service, err = GetKeystoneServiceWithName(ctx, h, service.GetName(), service.GetNamespace())
	if err != nil {
		return ctrl.Result{}, err
	}

	ks.id = ks.service.Status.ServiceID

	return ctrl.Result{}, nil
}

// GetServiceID - returns the openstack service ID
func (ks *KeystoneServiceHelper) GetServiceID() string {
	return ks.id
}

// GetConditions - returns the conditions of the keystone service object
func (ks *KeystoneServiceHelper) GetConditions() *condition.Conditions {
	return &ks.service.Status.Conditions
}

// Delete - deletes a KeystoneService if it exists.
func (ks *KeystoneServiceHelper) Delete(
	ctx context.Context,
	h *helper.Helper,
) error {

	service, err := GetKeystoneServiceWithName(ctx, h, ks.service.Name, ks.service.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	err = h.GetClient().Delete(ctx, service, &client.DeleteOptions{})
	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(service, h.GetFinalizer())
	if err := h.GetClient().Update(ctx, service); err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}

	h.GetLogger().Info(fmt.Sprintf("KeystoneService %s in namespace %s deleted", service.Name, service.Namespace))

	return nil
}

// GetKeystoneServiceWithName func
func GetKeystoneServiceWithName(
	ctx context.Context,
	h *helper.Helper,
	name string,
	namespace string,
) (*KeystoneService, error) {

	ks := &KeystoneService{}
	err := h.GetClient().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, ks)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return ks, err
		}
		return ks, err
	}

	return ks, nil
}
