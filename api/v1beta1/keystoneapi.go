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
	"net/url"
	"reflect"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	openstack "github.com/openstack-k8s-operators/lib-common/modules/openstack"
	appsv1 "k8s.io/api/apps/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// KeystoneAPIStatusChangedPredicate - primary purpose is to return true if
// the KeystoneAPI Status.APIEndpoints has changed.
// In addition also returns true if it gets deleted. Used by service operators
// to watch KeystoneAPI endpoint they depend on.
var KeystoneAPIStatusChangedPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectOld == nil || e.ObjectNew == nil {
			return false
		}
		oldPod, okOld := e.ObjectOld.(*KeystoneAPI)
		newPod, okNew := e.ObjectNew.(*KeystoneAPI)

		if !okOld || !okNew {
			return false // Not a keystonev1.KeystoneEndpoint, or cast error
		}

		// Compare the Status fields of the old and new keystonev1.KeystoneAPI.Status.APIEndpoints.
		statusIsDifferent := !reflect.DeepEqual(oldPod.Status.APIEndpoints, newPod.Status.APIEndpoints)
		return statusIsDifferent
	},
	DeleteFunc: func(_ event.DeleteEvent) bool {
		// By default, we might want to react to deletions of KeystoneAPI.
		return true
	},
}

// GetKeystoneAPI - get keystoneAPI object in namespace
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

// GetKeystoneAPIByName - get keystoneAPI object by name and namespace
func GetKeystoneAPIByName(
	ctx context.Context,
	h *helper.Helper,
	name string,
	namespace string,
) (*KeystoneAPI, error) {
	keystoneAPI := &KeystoneAPI{}
	err := h.GetClient().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, keystoneAPI)
	if err != nil {
		return nil, err
	}

	return keystoneAPI, nil
}

// GetAdminServiceClient - get a system scoped admin serviceClient for the keystoneAPI instance
func GetAdminServiceClient(
	ctx context.Context,
	h *helper.Helper,
	keystoneAPI *KeystoneAPI,
) (*openstack.OpenStack, ctrl.Result, error) {
	// Get the password of the admin user from Spec.Secret using PasswordSelectors.Admin
	authPassword, ctrlResult, err := secret.GetDataFromSecret(
		ctx,
		h,
		keystoneAPI.Spec.Secret,
		10*time.Second,
		keystoneAPI.Spec.PasswordSelectors.Admin)
	if err != nil {
		return nil, ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		return nil, ctrlResult, fmt.Errorf("password for user %s not found", keystoneAPI.Spec.PasswordSelectors.Admin)
	}

	// get endpoint as authurl from keystone instance
	// default to internal endpoint if not specified
	epInterface := endpoint.EndpointInternal
	if keystoneAPI.Spec.ExternalKeystoneAPI {
		epInterface = endpoint.EndpointPublic
	}

	return GetScopedClient(
		ctx,
		h,
		keystoneAPI,
		keystoneAPI.Spec.AdminUser,
		authPassword,
		keystoneAPI.Spec.AdminProject,
		"Default",
		keystoneAPI.Spec.Region,
		epInterface,
		&gophercloud.AuthScope{
			System: true,
		},
	)
}

// GetScopedClient - returns an *openstack.OpenStack object with the specified credentials and scope
func GetScopedClient(
	ctx context.Context,
	h *helper.Helper,
	keystoneAPI *KeystoneAPI,
	username string,
	password string,
	tenantName string,
	domainName string,
	region string,
	endpointType endpoint.Endpoint,
	scope *gophercloud.AuthScope,
) (*openstack.OpenStack, ctrl.Result, error) {

	// Use keystoneAPI region as default if not specified
	if region == "" {
		region = keystoneAPI.Spec.Region
	}

	authURL, err := keystoneAPI.GetEndpoint(endpointType)
	if err != nil {
		return nil, ctrl.Result{}, err
	}

	parsedAuthURL, err := url.Parse(authURL)
	if err != nil {
		return nil, ctrl.Result{}, err
	}

	tlsConfig := &openstack.TLSConfig{}
	if parsedAuthURL.Scheme == "https" && keystoneAPI.Spec.TLS.CaBundleSecretName != "" {
		caCert, ctrlResult, err := secret.GetDataFromSecret(
			ctx,
			h,
			keystoneAPI.Spec.TLS.CaBundleSecretName,
			10*time.Second,
			interfaceBundleKeys[endpointType])
		if err != nil {
			return nil, ctrlResult, err
		}
		if (ctrlResult != ctrl.Result{}) {
			return nil, ctrlResult,
				fmt.Errorf("CABundleSecret %s not found",
					keystoneAPI.Spec.TLS.CaBundleSecretName)
		}

		tlsConfig = &openstack.TLSConfig{
			CACerts: []string{caCert},
		}
	}

	osClient, err := openstack.NewOpenStack(
		ctx,
		h.GetLogger(),
		openstack.AuthOpts{
			AuthURL:    authURL,
			Username:   username,
			Password:   password,
			TenantName: tenantName,
			DomainName: domainName,
			Region:     region,
			TLS:        tlsConfig,
			Scope:      scope,
		},
	)
	if err != nil {
		return nil, ctrl.Result{}, err
	}

	return osClient, ctrl.Result{}, nil
}

// GetUserServiceClient - returns an *openstack.OpenStack object scoped as the given service user
func GetUserServiceClient(
	ctx context.Context,
	h *helper.Helper,
	keystoneAPI *KeystoneAPI,
	userName string,
	secretName string,
	passwordSelector string,
) (*openstack.OpenStack, ctrl.Result, error) {

	password, res, err := getPasswordFromOSPSecret(ctx, h, secretName, passwordSelector)
	if err != nil {
		return nil, ctrl.Result{}, fmt.Errorf("failed to get password from osp-secret for user %q: %w", userName, err)
	}
	if res != (ctrl.Result{}) {
		return nil, res, nil
	}

	// get endpoint as authurl from keystone instance
	// default to internal endpoint if not specified
	epInterface := endpoint.EndpointInternal
	if keystoneAPI.Spec.ExternalKeystoneAPI {
		epInterface = endpoint.EndpointPublic
	}

	return GetScopedClient(
		ctx,
		h,
		keystoneAPI,
		userName,
		password,
		"service",
		"Default",
		keystoneAPI.Spec.Region,
		epInterface,
		&gophercloud.AuthScope{
			ProjectName: "service",
			DomainName:  "Default",
		},
	)
}

func getPasswordFromOSPSecret(
	ctx context.Context,
	h *helper.Helper,
	ospSecretName, passwordSelector string,
) (string, ctrl.Result, error) {
	data, res, err := secret.GetDataFromSecret(
		ctx,
		h,
		ospSecretName,
		10*time.Second,
		passwordSelector,
	)
	if err != nil {
		return "", ctrl.Result{}, err
	}
	if res != (ctrl.Result{}) {
		return "", res, nil
	}
	return data, ctrl.Result{}, nil
}

// GetScopedAdminServiceClient - get a scoped admin serviceClient for the keystoneAPI instance
//
// Deprecated: Use GetScopedClient instead. This function will be removed in a future release.
func GetScopedAdminServiceClient(
	ctx context.Context,
	h *helper.Helper,
	keystoneAPI *KeystoneAPI,
	scope *gophercloud.AuthScope,
) (*openstack.OpenStack, ctrl.Result, error) {
	// get endpoint as authurl from keystone instance
	// default to internal endpoint if not specified
	epInterface := endpoint.EndpointInternal
	if keystoneAPI.Spec.ExternalKeystoneAPI {
		epInterface = endpoint.Endpoint(endpoint.EndpointPublic)
	}
	authURL, err := keystoneAPI.GetEndpoint(epInterface)
	if err != nil {
		return nil, ctrl.Result{}, err
	}

	parsedAuthURL, err := url.Parse(authURL)
	if err != nil {
		return nil, ctrl.Result{}, err
	}

	tlsConfig := &openstack.TLSConfig{}
	if parsedAuthURL.Scheme == "https" {
		caCert, ctrlResult, err := secret.GetDataFromSecret(
			ctx,
			h,
			keystoneAPI.Spec.TLS.CaBundleSecretName,
			10*time.Second,
			interfaceBundleKeys[epInterface])
		if err != nil {
			return nil, ctrl.Result{}, err
		}
		if (ctrlResult != ctrl.Result{}) {
			return nil, ctrl.Result{}, fmt.Errorf("the CABundleSecret %s not found", keystoneAPI.Spec.TLS.CaBundleSecretName)
		}

		tlsConfig = &openstack.TLSConfig{
			CACerts: []string{
				caCert,
			},
		}
	}

	// get the password of the admin user from Spec.Secret
	// using PasswordSelectors.Admin
	authPassword, ctrlResult, err := secret.GetDataFromSecret(
		ctx,
		h,
		keystoneAPI.Spec.Secret,
		10*time.Second,
		keystoneAPI.Spec.PasswordSelectors.Admin)
	if err != nil {
		return nil, ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		return nil, ctrlResult, fmt.Errorf("password for user %s not found", keystoneAPI.Spec.PasswordSelectors.Admin)
	}

	os, err := openstack.NewOpenStack(
		ctx,
		h.GetLogger(),
		openstack.AuthOpts{
			AuthURL:    authURL,
			Username:   keystoneAPI.Spec.AdminUser,
			Password:   authPassword,
			TenantName: keystoneAPI.Spec.AdminProject,
			DomainName: "Default",
			Region:     keystoneAPI.Spec.Region,
			TLS:        tlsConfig,
			Scope:      scope,
		})
	if err != nil {
		return nil, ctrl.Result{}, err
	}

	return os, ctrl.Result{}, nil
}
