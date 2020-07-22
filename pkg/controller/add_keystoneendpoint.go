package controller

import (
	"github.com/openstack-k8s-operators/keystone-operator/pkg/controller/keystoneendpoint"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, keystoneendpoint.Add)
}
