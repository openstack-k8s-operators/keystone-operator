package keystone

import (
	comv1 "github.com/openstack-k8s-operators/keystone-operator/pkg/apis/keystone/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// API func
func API(nameSpace string, name string) *comv1.KeystoneAPI {

	keystoneAPI := &comv1.KeystoneAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: nameSpace,
		},
	}
	return keystoneAPI
}
