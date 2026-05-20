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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/object"
)

// ApplicationCredentialData contains AC ID/Secret extracted from a Secret
type ApplicationCredentialData struct {
	ID     string
	Secret string
}

// GetACCRName returns the standard AC CR name for a service
func GetACCRName(serviceName string) string {
	return fmt.Sprintf("ac-%s", serviceName)
}

// GetServiceNameFromACCR extracts the service name from an AC CR name
// by stripping the "ac-" prefix. This is the inverse of GetACCRName.
func GetServiceNameFromACCR(acName string) string {
	return strings.TrimPrefix(acName, "ac-")
}

const (
	// ACIDSecretKey is the key for the ApplicationCredential ID in the Secret
	ACIDSecretKey = "AC_ID"
	// ACSecretSecretKey is the key for the ApplicationCredential secret in the Secret
	ACSecretSecretKey = "AC_SECRET"

	// EDPMServiceAnnotation marks whether an AC CR's credentials are deployed
	// to EDPM nodes. The AC controller gates cleanup and deletion on NodeSet
	// secret hash sync unless this annotation is explicitly set to "false".
	// Missing annotation defaults to EDPM service (as fail safe).
	EDPMServiceAnnotation = "keystone.openstack.org/edpm-service" // #nosec G101
)

// IsEDPMService returns true unless the annotation is explicitly set to "false".
// Missing annotation defaults to true as a safety mechanism: if the annotation
// is accidentally removed, the AC is still protected by EDPM hash sync checks.
func (ac *KeystoneApplicationCredential) IsEDPMService() bool {
	return ac.GetAnnotations()[EDPMServiceAnnotation] != "false"
}

// ManageACSecretFinalizer ensures consumerFinalizer is present on the AC secret
// identified by newSecretName and absent from the one identified by
// oldSecretName. It is a no-op when both names are equal.
func ManageACSecretFinalizer(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
	newSecretName string,
	oldSecretName string,
	consumerFinalizer string,
) error {
	if newSecretName == oldSecretName {
		return nil
	}

	var newObj, oldObj client.Object

	if newSecretName != "" {
		secret := &corev1.Secret{}
		key := types.NamespacedName{Name: newSecretName, Namespace: namespace}
		if err := h.GetClient().Get(ctx, key, secret); err != nil {
			return fmt.Errorf("failed to get new AC secret %s: %w", newSecretName, err)
		}
		newObj = secret
	}

	if oldSecretName != "" {
		secret := &corev1.Secret{}
		key := types.NamespacedName{Name: oldSecretName, Namespace: namespace}
		if err := h.GetClient().Get(ctx, key, secret); err != nil {
			if !k8s_errors.IsNotFound(err) {
				return fmt.Errorf("failed to get old AC secret %s: %w", oldSecretName, err)
			}
		} else {
			oldObj = secret
		}
	}

	return object.ManageConsumerFinalizer(ctx, h, newObj, oldObj, consumerFinalizer)
}

// RemoveACSecretConsumerFinalizer removes consumerFinalizer from the AC secret
// identified by secretName. It is a no-op when secretName is empty or the
// secret no longer exists.
func RemoveACSecretConsumerFinalizer(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
	secretName string,
	consumerFinalizer string,
) error {
	if secretName == "" {
		return nil
	}

	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: secretName, Namespace: namespace}
	if err := h.GetClient().Get(ctx, key, secret); err != nil {
		if k8s_errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return object.RemoveConsumerFinalizer(ctx, h, secret, consumerFinalizer)
}
