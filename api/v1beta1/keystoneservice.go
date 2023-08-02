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

// KeystoneServiceHelper -
type KeystoneServiceHelper struct {
	service *KeystoneService
	timeout time.Duration
	labels  map[string]string
	id      string
}

// NewKeystoneService returns an initialized KeystoneService.
func NewKeystoneService(
	spec KeystoneServiceSpec,
	namespace string,
	labels map[string]string,
	timeout time.Duration,
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

		return controllerutil.SetControllerReference(h.GetBeforeObject(), service, h.GetScheme())
	})
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			h.GetLogger().Info(fmt.Sprintf("KeystoneService %s not found, reconcile in %s", service.Name, ks.timeout))
			return ctrl.Result{RequeueAfter: ks.timeout}, nil
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
	if controllerutil.RemoveFinalizer(service, h.GetFinalizer()) {
		err := h.GetClient().Update(ctx, service)
		if err != nil && !k8s_errors.IsNotFound(err) {
			return err
		}
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
		return nil, err
	}

	return ks, nil
}

// DeleteKeystoneServiceWithName func
func DeleteKeystoneServiceWithName(
	ctx context.Context,
	h *helper.Helper,
	name string,
	namespace string,
) error {

	ks, err := GetKeystoneServiceWithName(ctx, h, name, namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}

	if ks != nil {
		ksSvcObj := NewKeystoneService(ks.Spec, namespace, map[string]string{}, 10)

		err = ksSvcObj.Delete(ctx, h)
		if err != nil {
			return err
		}
	}

	return nil
}
