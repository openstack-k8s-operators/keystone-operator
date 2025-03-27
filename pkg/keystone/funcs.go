package keystone

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// BaseSecurityContext - currently used to make sure we don't run cronJob and Log
// Pods as root user, and we drop privileges and Capabilities we don't need
func BaseSecurityContext() *corev1.SecurityContext {
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
