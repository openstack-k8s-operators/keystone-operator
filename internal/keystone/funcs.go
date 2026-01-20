package keystone

import (
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// baseSecurityContext - currently used to make sure we don't run cronJob and Log
// Pods as root user, and we drop privileges and Capabilities we don't need
func baseSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		RunAsUser:                ptr.To(KeystoneUID),
		RunAsGroup:               ptr.To(KeystoneUID),
		RunAsNonRoot:             ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				"ALL",
			},
		},
	}
}

// dbSyncSecurityContext - currently used to make sure we don't run db-sync as
// root user
func dbSyncSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		RunAsUser:  ptr.To(KeystoneUID),
		RunAsGroup: ptr.To(KeystoneUID),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				"MKNOD",
			},
		},
	}
}

// httpdSecurityContext -
func httpdSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				"MKNOD",
			},
		},
		RunAsUser:  ptr.To(KeystoneUID),
		RunAsGroup: ptr.To(KeystoneUID),
	}
}

// ComputeSecurityHash computes a hash of security-critical spec fields
// (roles, accessRules, unrestricted). Used to detect changes that require immediate rotation.
func ComputeSecurityHash(spec keystonev1.KeystoneApplicationCredentialSpec) (string, error) {
	securityFields := struct {
		Roles        []string
		AccessRules  []keystonev1.ACRule
		Unrestricted bool
	}{
		Roles:        spec.Roles,
		AccessRules:  spec.AccessRules,
		Unrestricted: spec.Unrestricted,
	}
	return util.ObjectHash(securityFields)
}
