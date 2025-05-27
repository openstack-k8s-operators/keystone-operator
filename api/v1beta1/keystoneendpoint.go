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
	"sort"
	"time"

	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"golang.org/x/exp/slices"
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
		endpoint.Labels = util.MergeStringMaps(endpoint.Labels, ke.labels)

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

// ValidateGeneration - returns true if metadata.generation matches status.observedgeneration
func (ke *KeystoneEndpointHelper) ValidateGeneration() bool {
	return ke.endpoint.Generation == ke.endpoint.Status.ObservedGeneration
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

	endpointurls := filterVisibility(ke.Items, visibility)

	return endpointurls, nil
}

// GetKeystoneEndpointUrlsForServices returns all URLs currently registered. Visibility can be admin, public or internal.
// If it is nil, all URLs are returned. Optional services parameter can be provided to only return the values for
// those services matching the `service` (common.AppSelector) label key.
func GetKeystoneEndpointUrlsForServices(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
	visibility *string,
	servicesToKeep []string,
) ([]string, error) {
	var keystoneEndpoints []KeystoneEndpoint

	ke, err := GetKeystoneEndpointList(ctx, h, namespace)
	if err != nil {
		return nil, err
	}

	if ke != nil {
		keystoneEndpoints = ke.Items
		if len(servicesToKeep) > 0 {
			keystoneEndpoints = filterServices(keystoneEndpoints, servicesToKeep)
		}
	}

	endpointurls := filterVisibility(keystoneEndpoints, visibility)

	return endpointurls, nil
}

// GetHashforKeystoneEndpointUrlsForServices returns a hash for a list of endpoint URLs currently registered.
// Visibility can be admin, public or internal. If it is nil, all URLs are returned.
// Optional services parameter can be provided to only return the values for those services matching the
// `service` (common.AppSelector) label key.
func GetHashforKeystoneEndpointUrlsForServices(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
	visibility *string,
	servicesToKeep []string,
) (string, error) {
	var hash string
	endpointUrls, err := GetKeystoneEndpointUrlsForServices(
		ctx,
		h,
		namespace,
		visibility,
		servicesToKeep,
	)
	if err != nil {
		return hash, err
	}
	if len(endpointUrls) > 0 {
		hash, err = util.ObjectHash(endpointUrls)
		if err != nil {
			return hash, err
		}
	}

	return hash, nil
}

// filterVisibility returns all URLs from an []KeystoneEndpoint list. Visibility can be used to
// filter based on admin, public or internal. If it is nil, all URLs are returned.
// The returning URLs list gets sorted.
func filterVisibility(
	keystoneEndpoints []KeystoneEndpoint,
	visibility *string,
) []string {
	endpointurls := []string{}
	if visibility != nil {
		for _, endpoint := range keystoneEndpoints {
			if endpt, ok := getEndpointForVisibility(endpoint.Status.Endpoints, *visibility); ok {
				endpointurls = append(endpointurls, endpt.URL)
			}
		}
	} else {
		for _, endpoint := range keystoneEndpoints {
			for _, e := range endpoint.Status.Endpoints {
				endpointurls = append(endpointurls, e.URL)
			}
		}
	}

	// make sure the list of endpoint urls is sorted
	sort.Strings(endpointurls)

	return endpointurls
}

// filterServices returns a list of []KeystoneEndpoint based on that passed in filter for
// the services to retain. Services get filtered based on the value in the common.AppSelector
// label.
func filterServices(
	allKeystoneEndpoints []KeystoneEndpoint,
	servicesToKeep []string,
) []KeystoneEndpoint {
	keystoneEndpoints := []KeystoneEndpoint{}
	// Create a map from the 'servicesToKeep' slice for fast lookups.
	// We only care about the keys.
	toKeepSet := make(map[string]struct{}, len(servicesToKeep))
	for _, svc := range servicesToKeep {
		toKeepSet[svc] = struct{}{}
	}

	for _, endpt := range allKeystoneEndpoints {
		if svc, found := endpt.Labels[common.AppSelector]; found {
			// Check if the current value 'svc' is in our whitelist.
			if _, found := toKeepSet[svc]; found {
				keystoneEndpoints = append(keystoneEndpoints, endpt)
			}
		}
	}

	return keystoneEndpoints
}

// getEndpointForVisibility - returns the endpoint from the list of Endpoints
// for the requested vislibility. If not found, returns false.
func getEndpointForVisibility(
	endpts []Endpoint,
	visibility string,
) (Endpoint, bool) {

	// validate if endpoint is already in the endpoint status list
	f := func(e Endpoint) bool {
		return e.Interface == visibility
	}
	idx := slices.IndexFunc(endpts, f)
	if idx >= 0 {
		return endpts[idx], true
	}

	return Endpoint{}, false
}
