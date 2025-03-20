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

	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
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

const (
	// ACIDSecretKey is the key for the ApplicationCredential ID in the Secret
	ACIDSecretKey = "AC_ID"
	// ACSecretSecretKey is the key for the ApplicationCredential secret in the Secret
	ACSecretSecretKey = "AC_SECRET"
)

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

	acID, okID := secret.Data[ACIDSecretKey]
	if !okID || len(acID) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrACIDMissing, key.String())
	}
	acSecret, okSecret := secret.Data[ACSecretSecretKey]
	if !okSecret || len(acSecret) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrACSecretMissing, key.String())
	}

	return &ApplicationCredentialData{ID: string(acID), Secret: string(acSecret)}, nil
}
