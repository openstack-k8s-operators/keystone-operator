package controller

import (
	"github.com/openstack-k8s-operators/keystone-operator/pkg/controller/keystoneservice"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, keystoneservice.Add)
}
