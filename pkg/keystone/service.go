package keystone

import (
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Service func
func Service(cr *keystonev1beta1.KeystoneAPI, cmName string, servicePort int32) *corev1.Service {

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
				{Name: "api", Port: servicePort, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	return svc
}
