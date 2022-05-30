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
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	gophercloud "github.com/gophercloud/gophercloud"
	openstack "github.com/gophercloud/gophercloud/openstack"
	endpoints "github.com/gophercloud/gophercloud/openstack/identity/v3/endpoints"
	projects "github.com/gophercloud/gophercloud/openstack/identity/v3/projects"
	roles "github.com/gophercloud/gophercloud/openstack/identity/v3/roles"
	services "github.com/gophercloud/gophercloud/openstack/identity/v3/services"
	users "github.com/gophercloud/gophercloud/openstack/identity/v3/users"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	keystone "github.com/openstack-k8s-operators/keystone-operator/pkg/keystone"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// KeystoneServiceReconciler reconciles a KeystoneService object
type KeystoneServiceReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch

// Reconcile keystone service requests
func (r *KeystoneServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("keystoneservice", req.NamespacedName)

	// Select the 1st KeystoneAPI instance
	var keystoneAPIInstance keystonev1.KeystoneAPI
	keystoneAPIList := &keystonev1.KeystoneAPIList{}
	listOpts := []client.ListOption{
		client.InNamespace(req.Namespace),
	}

	if err := r.Client.List(ctx, keystoneAPIList, listOpts...); err != nil {
		return ctrl.Result{}, err
	}

	if len(keystoneAPIList.Items) == 1 {
		keystoneAPIInstance = keystoneAPIList.Items[0]
	} else if len(keystoneAPIList.Items) > 1 {
		r.Log.Info("Multiple KeystoneAPI instances found.")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	} else {
		r.Log.Info("No KeystoneAPI instances found")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	if hash, ok := keystoneAPIInstance.Status.Hash[keystonev1.BootstrapHash]; !ok {
		r.Log.Info("KeystoneAPI bootstrap not complete.", "BootstrapHash", hash)
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}
	r.Log.Info("KeystoneAPI bootstrap complete.", "BootstrapHash", keystoneAPIInstance.Status.Hash[keystonev1.BootstrapHash])

	// Fetch the KeystoneService instance
	instance := &keystonev1.KeystoneService{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
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

	openStackConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openstack-config",
			Namespace: instance.Namespace,
		},
	}
	err = r.Client.Get(ctx, types.NamespacedName{Name: openStackConfigMap.Name, Namespace: instance.Namespace}, openStackConfigMap)
	if err != nil {
		return ctrl.Result{}, err
	}

	oscm := keystone.OpenStackConfig{}
	err = yaml.Unmarshal([]byte(openStackConfigMap.Data["clouds.yaml"]), &oscm)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.Log.Info("openStackConfigMap binary data", "binary", openStackConfigMap.BinaryData)
	r.Log.Info("openStackConfigMap data", "data", openStackConfigMap.Data)
	r.Log.Info("oscm", "oscm", oscm)

	openStackConfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openstack-config-secret",
			Namespace: instance.Namespace,
		},
	}
	err = r.Client.Get(
		ctx,
		types.NamespacedName{
			Name:      openStackConfigSecret.Name,
			Namespace: instance.Namespace},
		openStackConfigSecret)
	if err != nil {
		return ctrl.Result{}, err
	}

	oscmSecret := keystone.OpenStackConfigSecret{}
	sec, err := r.Kclient.CoreV1().Secrets(instance.Namespace).Get(ctx, openStackConfigSecret.Name, metav1.GetOptions{})
	if err != nil {
		return ctrl.Result{}, err
	}
	r.Log.Info("sec", "sec", string(sec.Data["secure.yaml"]))

	err = yaml.Unmarshal([]byte(string(sec.Data["secure.yaml"])), &oscmSecret)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.Log.Info("oscmSecret", "secret", oscmSecret)

	opts := gophercloud.AuthOptions{
		IdentityEndpoint: oscm.Clouds.Default.Auth.AuthURL,
		Username:         oscm.Clouds.Default.Auth.UserName,
		Password:         oscmSecret.Clouds.Default.Auth.Password,
		TenantName:       oscm.Clouds.Default.Auth.ProjectName,
		DomainName:       oscm.Clouds.Default.Auth.UserDomainName,
	}

	provider, err := openstack.AuthenticatedClient(opts)
	if err != nil {
		return ctrl.Result{}, err
	}
	endpointOpts := gophercloud.EndpointOpts{Type: "identity", Region: instance.Spec.Region}
	identityClient, err := openstack.NewIdentityV3(provider, endpointOpts)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create new service if ServiceID is not already set
	if instance.Status.ServiceID == "" {
		createOpts := services.CreateOpts{
			Type:    instance.Spec.ServiceType,
			Enabled: &instance.Spec.Enabled,
			Extra: map[string]interface{}{
				"name":        instance.Spec.ServiceName,
				"description": instance.Spec.ServiceDescription,
			},
		}

		service, err := services.Create(identityClient, createOpts).Extract()
		if err != nil {
			r.Log.Error(err, "error")
			return ctrl.Result{}, err
		}

		// Set ServiceID in the status
		r.Log.Info("instance.Status.ServiceID", "ServiceID", instance.Status.ServiceID)
		r.Log.Info("service.ID", "service.ID", service.ID)
		if instance.Status.ServiceID != service.ID {
			instance.Status.ServiceID = service.ID
			if err := r.Client.Status().Update(ctx, instance); err != nil {
				r.Log.Error(err, "error")
				return ctrl.Result{}, err
			}
		}
	} else {
		// ServiceID is already set, update the service
		updateOpts := services.UpdateOpts{
			Type:    instance.Spec.ServiceType,
			Enabled: &instance.Spec.Enabled,
			Extra: map[string]interface{}{
				"name":        instance.Spec.ServiceName,
				"description": instance.Spec.ServiceDescription,
			},
		}
		_, err := services.Update(identityClient, instance.Status.ServiceID, updateOpts).Extract()
		if err != nil {
			r.Log.Error(err, "error")
			return ctrl.Result{}, err
		}
	}

	serviceID := instance.Status.ServiceID
	err = reconcileEndpoint(identityClient, serviceID, instance.Spec.ServiceName, instance.Spec.Region, "admin", instance.Spec.AdminURL)
	if err != nil {
		return ctrl.Result{}, err
	}
	err = reconcileEndpoint(identityClient, serviceID, instance.Spec.ServiceName, instance.Spec.Region, "internal", instance.Spec.InternalURL)
	if err != nil {
		return ctrl.Result{}, err
	}
	err = reconcileEndpoint(identityClient, serviceID, instance.Spec.ServiceName, instance.Spec.Region, "public", instance.Spec.PublicURL)
	if err != nil {
		return ctrl.Result{}, err
	}

	var username string
	if instance.Spec.Username == "" {
		username = instance.Spec.ServiceName
	} else {
		username = instance.Spec.Username
	}

	var password string
	if instance.Spec.Password == "" {
		// FIXME: use our default password for now until we have generation.
		password = "foobar123"
	} else {
		password = instance.Spec.Password
	}

	err = reconcileUser(r.Log, identityClient, username, password)
	if err != nil {
		r.Log.Error(err, "error")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager x
func (r *KeystoneServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keystonev1.KeystoneService{}).
		Complete(r)
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
		return fmt.Errorf("endpoint interface %s not known", endpointInterface)
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

func reconcileUser(reqLogger logr.Logger, client *gophercloud.ServiceClient, username string, password string) error {
	reqLogger.Info("Reconciling User.", "Username", username)

	var serviceProjectID string
	allPages, err := projects.List(client, projects.ListOpts{Name: "service"}).AllPages()
	if err != nil {
		return err
	}
	allProjects, err := projects.ExtractProjects(allPages)
	if err != nil {
		return err
	}
	if len(allProjects) == 1 {
		serviceProjectID = allProjects[0].ID
	} else if len(allProjects) == 0 {
		createOpts := projects.CreateOpts{
			Name:        "service",
			Description: "service",
		}
		reqLogger.Info("Creating service project")
		project, err := projects.Create(client, createOpts).Extract()
		if err != nil {
			return err
		}
		serviceProjectID = project.ID
	} else {
		return errors.New("multiple projects named \"service\" found")
	}

	allPages, err = users.List(client, users.ListOpts{Name: username}).AllPages()
	if err != nil {
		return err
	}
	allUsers, err := users.ExtractUsers(allPages)
	if err != nil {
		return err
	}
	var userID string
	if len(allUsers) == 1 {
		userID = allUsers[0].ID
	} else {
		createOpts := users.CreateOpts{
			Name:             username,
			DefaultProjectID: serviceProjectID,
			Password:         password,
		}
		user, err := users.Create(client, createOpts).Extract()
		if err != nil {
			return err
		}
		reqLogger.Info("User Created", "Username", user.Name, "User ID", user.ID)
		userID = user.ID
	}

	var adminRoleID string
	allPages, err = roles.List(client, roles.ListOpts{
		Name: "admin"}).AllPages()
	if err != nil {
		return err
	}
	allRoles, err := roles.ExtractRoles(allPages)
	if err != nil {
		return err
	}
	if len(allRoles) == 1 {
		adminRoleID = allRoles[0].ID
	} else {
		return errors.New("could not lookup admin role ID")
	}

	err = roles.Assign(client, adminRoleID, roles.AssignOpts{
		UserID:    userID,
		ProjectID: serviceProjectID}).ExtractErr()
	if err != nil {
		return err
	}

	return nil
}
