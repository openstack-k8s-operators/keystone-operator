/*
Copyright 2025

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
	"errors"
	"fmt"
	"time"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplicationCredentialData contains AC ID/Secret extracted from a Secret
// Used by service operators to get AC data from the Secret
type ApplicationCredentialData struct {
	ID     string
	Secret string
}

// GetACSecretName returns the standard AC Secret name for a service
func GetACSecretName(serviceName string) string {
	return fmt.Sprintf("ac-%s-secret", serviceName)
}

// GetACCRName returns the standard AC CR name for a service
func GetACCRName(serviceName string) string {
	return fmt.Sprintf("ac-%s", serviceName)
}

var (
	// ErrACIDMissing indicates AC_ID key missing or empty in the Secret
	ErrACIDMissing = errors.New("applicationcredential secret missing AC_ID")
	// ErrACSecretMissing indicates AC_SECRET key missing or empty in the Secret
	ErrACSecretMissing = errors.New("applicationcredential secret missing AC_SECRET")
)

// GetApplicationCredentialFromSecret fetches and validates AC data from the Secret
func GetApplicationCredentialFromSecret(
	ctx context.Context,
	c client.Client,
	namespace string,
	serviceName string,
) (*ApplicationCredentialData, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: namespace, Name: GetACSecretName(serviceName)}
	if err := c.Get(ctx, key, secret); err != nil {
		if k8s_errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get applicationcredential secret %s: %w", key, err)
	}

	acID, okID := secret.Data["AC_ID"]
	if !okID || len(acID) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrACIDMissing, key.String())
	}
	acSecret, okSecret := secret.Data["AC_SECRET"]
	if !okSecret || len(acSecret) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrACSecretMissing, key.String())
	}

	return &ApplicationCredentialData{ID: string(acID), Secret: string(acSecret)}, nil
}

// VerifyApplicationCredentialsForService checks if AC secret exists and adds it to configVars.
// If the AC secret is not found or invalid, it returns ctrl.Result{} without error (AC is optional).
// If the AC secret is valid, it adds the secret hash to configVars for change tracking.
func VerifyApplicationCredentialsForService(
	ctx context.Context,
	c client.Client,
	namespace string,
	serviceName string,
	configVars *map[string]env.Setter,
	timeout time.Duration,
) (ctrl.Result, error) {
	acSecretName := GetACSecretName(serviceName)
	secretKey := types.NamespacedName{Namespace: namespace, Name: acSecretName}

	hash, res, err := secret.VerifySecret(
		ctx,
		secretKey,
		[]string{"AC_ID", "AC_SECRET"},
		c,
		timeout,
	)

	// VerifySecret returns res.RequeueAfter > 0 when secret not found (not an error)
	// For AC, this is optional, so we just skip it instead of requeueing
	if res.RequeueAfter > 0 {
		// AC secret not found - this is OK, service will use password auth
		return ctrl.Result{}, nil
	}

	if err != nil {
		// Actual error (not NotFound) - continue with password auth
		return ctrl.Result{}, nil
	}

	// AC secret exists and is valid - add to configVars for hash tracking
	if configVars != nil {
		(*configVars)["secret-"+acSecretName] = env.SetValue(hash)
	}

	return ctrl.Result{}, nil
}
