package keystone

import (
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type keystoneConfigOptions struct {
	DatabaseHostname string
}

// ConfigMap custom keystone config map
func ConfigMap(cr *keystonev1beta1.KeystoneAPI, cmName string) *corev1.ConfigMap {
	opts := keystoneConfigOptions{cr.Spec.DatabaseHostname}

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
			"keystone.conf":       util.ExecuteTemplateFile("keystone.conf", &opts),
			"httpd.conf":          util.ExecuteTemplateFile("httpd.conf", nil),
			"config.json":         util.ExecuteTemplateFile("kolla_config.json", nil),
			"logging.conf":        util.ExecuteTemplateFile("logging.conf", nil),
			"db-sync-config.json": util.ExecuteTemplateFile("db-sync-config.json", nil),
		},
	}

	return cm
}
