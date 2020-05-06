package keystone

import (
	comv1 "github.com/openstack-k8s-operators/keystone-operator/pkg/apis/keystone/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Service func
func Service(cr *comv1.KeystoneAPI, cmName string) *corev1.Service {

	labels := map[string]string{
		"app": "keystone-api",
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{Name: "api", Port: 5000, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	return svc
}
