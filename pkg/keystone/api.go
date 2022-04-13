package keystone

import (
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// API func
func API(nameSpace string, name string) *keystonev1beta1.KeystoneAPI {

	keystoneAPI := &keystonev1beta1.KeystoneAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: nameSpace,
		},
	}
	return keystoneAPI
}
