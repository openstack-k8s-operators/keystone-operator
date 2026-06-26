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
	"fmt"
	"strings"
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
