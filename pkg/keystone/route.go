package keystone

import (
	routev1 "github.com/openshift/api/route/v1"
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

// Route func
func Route(cr *keystonev1beta1.KeystoneAPI, cmName string) *routev1.Route {

	labels := map[string]string{
		"app": "keystone-api",
	}
	serviceRef := routev1.RouteTargetReference{
		Kind: "Service",
		Name: "keystone",
	}
	routePort := &routev1.RoutePort{
		TargetPort: intstr.FromString("api"),
	}
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: routev1.RouteSpec{
			To:   serviceRef,
			Port: routePort,
		},
	}
	return route
}
