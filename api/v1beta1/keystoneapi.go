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
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openstack "github.com/openstack-k8s-operators/lib-common/modules/openstack"
	appsv1 "k8s.io/api/apps/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

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

// GetAdminServiceClient - get a system scoped admin serviceClient for the keystoneAPI instance
func GetAdminServiceClient(
	ctx context.Context,
	h *helper.Helper,
	keystoneAPI *KeystoneAPI,
) (*openstack.OpenStack, ctrl.Result, error) {
	os, ctrlResult, err := GetScopedAdminServiceClient(
		ctx,
		h,
		keystoneAPI,
		&gophercloud.AuthScope{
			System: true,
		},
	)
	if err != nil {
		return nil, ctrlResult, err
	}

	return os, ctrlResult, nil
}

// GetUserServiceClient - returns an *openstack.OpenStack object scoped as the given service user
func GetUserServiceClient(
	ctx context.Context,
	h *helper.Helper,
	keystoneAPI *KeystoneAPI,
	userName string,
) (*openstack.OpenStack, ctrl.Result, error) {

	authURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointInternal)
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
			tls.InternalCABundleKey)
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

	password, err := getPasswordFromOSPSecret(ctx, h, userName)
	if err != nil {
		return nil, ctrl.Result{}, fmt.Errorf("failed to get password from osp-secret for user %q: %w", userName, err)
	}

	scope := &gophercloud.AuthScope{
		ProjectName: "service",
		DomainName:  "Default",
	}

	osClient, err := openstack.NewOpenStack(
		h.GetLogger(),
		openstack.AuthOpts{
			AuthURL:    authURL,
			Username:   userName,
			Password:   password,
			TenantName: "service",
			DomainName: "Default",
			Region:     keystoneAPI.Spec.Region,
			TLS:        tlsConfig,
			Scope:      scope,
		},
	)
	if err != nil {
		return nil, ctrl.Result{}, err
	}

	// DEBUG
	logger := h.GetLogger()
	logger.Info("GetUserServiceClient debug",
		"authURL", authURL,
		"region", keystoneAPI.Spec.Region,
		"userName", userName,
	)

	return osClient, ctrl.Result{}, nil
}

func getPasswordFromOSPSecret(
	ctx context.Context,
	h *helper.Helper,
	userName string,
) (string, error) {

	// The name of the Secret containing the service passwords
	const ospSecretName = "osp-secret"

	secretObj, _, err := secret.GetSecret(ctx, h, ospSecretName, h.GetBeforeObject().GetNamespace())
	if err != nil {
		return "", fmt.Errorf("failed to get Secret/%s: %w", ospSecretName, err)
	}

	key := capitalizeFirst(userName) + "Password"

	val, ok := secretObj.Data[key]
	if !ok {
		return "", fmt.Errorf("%q not in Secret/%s", key, ospSecretName)
	}

	return string(val), nil
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// GetScopedAdminServiceClient - get a scoped admin serviceClient for the keystoneAPI instance
func GetScopedAdminServiceClient(
	ctx context.Context,
	h *helper.Helper,
	keystoneAPI *KeystoneAPI,
	scope *gophercloud.AuthScope,
) (*openstack.OpenStack, ctrl.Result, error) {
	// get public endpoint as authurl from keystone instance
	authURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointInternal)
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
			tls.InternalCABundleKey)
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
