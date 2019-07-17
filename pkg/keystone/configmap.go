package keystone

import (
        util "github.com/openstack-k8s-operators/keystone-operator/pkg/util"
        comv1 "github.com/openstack-k8s-operators/keystone-operator/pkg/apis/keystone/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type keystoneConfigOptions struct {
	DatabasePassword string
	DatabaseHostname string
	AdminPassword    string
}

// custom keystone config map
func ConfigMap(cr *comv1.KeystoneApi, cmName string) *corev1.ConfigMap {
	opts := keystoneConfigOptions{cr.Spec.DatabasePassword, cr.Spec.DatabaseHostname, cr.Spec.AdminPassword}

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
		},
		Data: map[string]string{
			"keystone.conf": util.ExecuteTemplateFile("keystone.conf", &opts),
			"httpd.conf":    util.ExecuteTemplateFile("httpd.conf", nil),
			"config.json":   util.ExecuteTemplateFile("kolla_config.json", nil),
		},
	}

	return cm
}
