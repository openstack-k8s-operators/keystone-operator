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
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// KeystoneEndpointHelper -
type KeystoneEndpointHelper struct {
	endpoint *KeystoneEndpoint
	timeout  time.Duration
	labels   map[string]string
	id       map[string]string
}

// NewKeystoneEndpoint returns an initialized KeystoneEndpoint.
func NewKeystoneEndpoint(
	name string,
	namespace string,
	spec KeystoneEndpointSpec,
	labels map[string]string,
	timeout time.Duration,
) *KeystoneEndpointHelper {
	endpoint := &KeystoneEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}

	return &KeystoneEndpointHelper{
		endpoint: endpoint,
		timeout:  timeout,
		labels:   labels,
	}
}

// CreateOrPatch - creates or patches a KeystoneEndpoint, reconciles after Xs if object won't exist.
func (ke *KeystoneEndpointHelper) CreateOrPatch(
	ctx context.Context,
	h *helper.Helper,
) (ctrl.Result, error) {
	endpoint := &KeystoneEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ke.endpoint.Name,
			Namespace: ke.endpoint.Namespace,
		},
	}

	// add finalizer
	controllerutil.AddFinalizer(endpoint, h.GetFinalizer())

	op, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), endpoint, func() error {
		endpoint.Spec = ke.endpoint.Spec
		endpoint.Labels = util.MergeStringMaps(endpoint.Labels, ke.endpoint.Labels)

		return controllerutil.SetControllerReference(h.GetBeforeObject(), endpoint, h.GetScheme())
	})
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			h.GetLogger().Info(fmt.Sprintf("KeystoneEndpoint %s not found, reconcile in %s", endpoint.Name, ke.timeout))
			return ctrl.Result{RequeueAfter: ke.timeout}, nil
		}
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		h.GetLogger().Info(fmt.Sprintf("KeystoneEndpoint %s - %s", endpoint.Name, op))
	}

	// update the endpoint object of the KeystoneEndpoint type
	ke.endpoint, err = GetKeystoneEndpointWithName(ctx, h, endpoint.GetName(), endpoint.GetNamespace())
	if err != nil {
		return ctrl.Result{}, err
	}

	ke.id = ke.endpoint.Status.EndpointIDs

	return ctrl.Result{}, nil
}

// GetEndpointIDs - returns the openstack endpoint IDs
func (ke *KeystoneEndpointHelper) GetEndpointIDs() map[string]string {
	return ke.id
}

// GetConditions - returns the conditions of the keystone endpoint object
func (ke *KeystoneEndpointHelper) GetConditions() *condition.Conditions {
	return &ke.endpoint.Status.Conditions
}

// Delete - deletes a KeystoneEndpoint if it exists.
func (ke *KeystoneEndpointHelper) Delete(
	ctx context.Context,
	h *helper.Helper,
) error {

	endpoint, err := GetKeystoneEndpointWithName(ctx, h, ke.endpoint.Name, ke.endpoint.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	err = h.GetClient().Delete(ctx, endpoint, &client.DeleteOptions{})
	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}

	// Endpoint is deleted so remove the finalizer.
	if controllerutil.RemoveFinalizer(endpoint, h.GetFinalizer()) {
		err := h.GetClient().Update(ctx, endpoint)
		if err != nil && !k8s_errors.IsNotFound(err) {
			return err
		}
	}

	h.GetLogger().Info(fmt.Sprintf("KeystoneEndpoint %s in namespace %s deleted", endpoint.Name, endpoint.Namespace))

	return nil
}

// GetKeystoneEndpointWithName func
func GetKeystoneEndpointWithName(
	ctx context.Context,
	h *helper.Helper,
	name string,
	namespace string,
) (*KeystoneEndpoint, error) {

	ke := &KeystoneEndpoint{}
	err := h.GetClient().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, ke)
	if err != nil {
		return nil, err
	}

	return ke, nil
}

// DeleteKeystoneEndpointWithName func
func DeleteKeystoneEndpointWithName(
	ctx context.Context,
	h *helper.Helper,
	name string,
	namespace string,
) error {

	ke, err := GetKeystoneEndpointWithName(ctx, h, name, namespace)

	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}

	if ke != nil {
		ksEndptObj := NewKeystoneEndpoint(ke.Name, namespace, ke.Spec, map[string]string{}, 10)

		err = ksEndptObj.Delete(ctx, h)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetKeystoneEndpointList func
func GetKeystoneEndpointList(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
) (*KeystoneEndpointList, error) {

	ke := &KeystoneEndpointList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
	}

	if err := h.GetClient().List(ctx, ke, listOpts...); err != nil {
		err = fmt.Errorf("error listing endpoints for namespace %s: %w", namespace, err)
		return nil, err
	}

	return ke, nil
}

// GetKeystoneEndpointUrls returns all URLs currently registered. Visibility can be admin, public or internal.
// If it is nil, all URLs are returned
func GetKeystoneEndpointUrls(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
	visibility *string,
) ([]string, error) {

	ke, err := GetKeystoneEndpointList(ctx, h, namespace)
	if err != nil {
		return nil, err
	}

	var endpointurls []string

	if visibility != nil {
		for _, endpoint := range ke.Items {
			endpointurls = append(endpointurls, endpoint.Spec.Endpoints[*visibility])
		}
	} else {
		for _, endpoint := range ke.Items {
			endpointurls = append(endpointurls, endpoint.Spec.Endpoints["internal"])
			endpointurls = append(endpointurls, endpoint.Spec.Endpoints["public"])
			endpointurls = append(endpointurls, endpoint.Spec.Endpoints["admin"])
		}
	}


	return endpointurls, nil
}
