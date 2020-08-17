/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	gophercloud "github.com/gophercloud/gophercloud"
	openstack "github.com/gophercloud/gophercloud/openstack"
	endpoints "github.com/gophercloud/gophercloud/openstack/identity/v3/endpoints"
	services "github.com/gophercloud/gophercloud/openstack/identity/v3/services"
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	keystone "github.com/openstack-k8s-operators/keystone-operator/pkg/keystone"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// KeystoneEndpointReconciler reconciles a KeystoneEndpoint object
type KeystoneEndpointReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// Reconcile keystone endpoint requests
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints/status,verbs=get;update;patch
func (r *KeystoneEndpointReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("keystoneendpoint", req.NamespacedName)

	keystoneAPI := keystone.API(req.Namespace, "keystone")
	objectKey, err := client.ObjectKeyFromObject(keystoneAPI)
	err = r.Client.Get(context.TODO(), objectKey, keystoneAPI)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// No KeystoneAPI instance running, return error
			r.Log.Error(err, "KeystoneAPI instance not found")
			return ctrl.Result{}, err
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	if keystoneAPI.Status.BootstrapHash == "" {
		r.Log.Info("KeystoneAPI bootstrap not complete.", "BootstrapHash", keystoneAPI.Status.BootstrapHash)
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}
	r.Log.Info("KeystoneAPI bootstrap complete.", "BootstrapHash", keystoneAPI.Status.BootstrapHash)

	// Fetch the KeystoneEndpoint instance
	instance := &keystonev1beta1.KeystoneEndpoint{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	r.Log.Info("authurl", "authurl", instance.Spec.AuthURL)
	r.Log.Info("username", "username", instance.Spec.Username)
	r.Log.Info("password", "password", instance.Spec.Password)
	r.Log.Info("project", "project", instance.Spec.Project)
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: instance.Spec.AuthURL,
		Username:         instance.Spec.Username,
		Password:         instance.Spec.Password,
		TenantName:       instance.Spec.Project,
		DomainName:       instance.Spec.DomainName,
	}

	provider, err := openstack.AuthenticatedClient(opts)
	if err != nil {
		r.Log.Error(err, "error")
		return ctrl.Result{}, err
	}
	endpointOpts := gophercloud.EndpointOpts{Type: "identity", Region: instance.Spec.Region}
	r.Log.Info("provider type", "%t", provider)
	r.Log.Info("IdentityEndpoint", "provider.IdentityEndpoint", provider.IdentityEndpoint)
	r.Log.Info("IdentityBase", "provider.IdentityBase", provider.IdentityBase)
	identityClient, err := openstack.NewIdentityV3(provider, endpointOpts)

	// Fetch the service ID
	listOpts := services.ListOpts{
		ServiceType: instance.Spec.ServiceType,
	}
	allPages, err := services.List(identityClient, listOpts).AllPages()
	if err != nil {
		return ctrl.Result{}, err
	}
	allServices, err := services.ExtractServices(allPages)
	r.Log.Info("allServices", "allServices", fmt.Sprintf("%v", allServices))
	if err != nil {
		return ctrl.Result{}, err
	}
	var serviceID string
	for _, service := range allServices {
		if service.Type == instance.Spec.ServiceType {
			serviceID = service.ID
		}
	}
	if serviceID == "" {
		err := fmt.Errorf("Service %s not found", instance.Spec.ServiceType)
		return ctrl.Result{}, err
	}

	r.Log.Info("Service Id", "Service Id", serviceID)

	reconcileEndpoint(identityClient, serviceID, instance.Spec.ServiceName, instance.Spec.Region, "admin", instance.Spec.AdminURL)
	reconcileEndpoint(identityClient, serviceID, instance.Spec.ServiceName, instance.Spec.Region, "internal", instance.Spec.InternalURL)
	reconcileEndpoint(identityClient, serviceID, instance.Spec.ServiceName, instance.Spec.Region, "public", instance.Spec.PublicURL)

	return ctrl.Result{}, nil
}

func reconcileEndpoint(client *gophercloud.ServiceClient, serviceID string, serviceName string, region string, endpointInterface string, url string) error {
	// Return if url is empty, likely wasn't specified in the request
	if url == "" {
		return nil
	}

	var availability gophercloud.Availability
	if endpointInterface == "admin" {
		availability = gophercloud.AvailabilityAdmin
	} else if endpointInterface == "internal" {
		availability = gophercloud.AvailabilityInternal
	} else if endpointInterface == "public" {
		availability = gophercloud.AvailabilityPublic
	} else {
		return fmt.Errorf("Endpoint interface %s not known", endpointInterface)
	}

	// Fetch existing endpoint and check it's value if it exists
	listOpts := endpoints.ListOpts{
		ServiceID:    serviceID,
		Availability: availability,
		RegionID:     region,
	}
	allPages, err := endpoints.List(client, listOpts).AllPages()
	if err != nil {
		return err
	}
	allEndpoints, err := endpoints.ExtractEndpoints(allPages)
	if err != nil {
		return err
	}
	if len(allEndpoints) == 1 {
		endpoint := allEndpoints[0]
		if url != endpoint.URL {
			// Update the endpoint
			updateOpts := endpoints.UpdateOpts{
				Availability: availability,
				Name:         serviceName,
				Region:       region,
				ServiceID:    serviceID,
				URL:          url,
			}
			_, err := endpoints.Update(client, endpoint.ID, updateOpts).Extract()
			if err != nil {
				return err
			}
		}
	} else {
		// Create the endpoint
		createOpts := endpoints.CreateOpts{
			Availability: availability,
			Name:         serviceName,
			Region:       region,
			ServiceID:    serviceID,
			URL:          url,
		}
		_, err := endpoints.Create(client, createOpts).Extract()
		if err != nil {
			return err
		}
	}

	return nil

}

// SetupWithManager x
func (r *KeystoneEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keystonev1beta1.KeystoneEndpoint{}).
		Complete(r)
}
