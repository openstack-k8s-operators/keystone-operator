package keystoneendpoint

import (
	"context"
	"fmt"
	gophercloud "github.com/gophercloud/gophercloud"
	openstack "github.com/gophercloud/gophercloud/openstack"
	endpoints "github.com/gophercloud/gophercloud/openstack/identity/v3/endpoints"
	services "github.com/gophercloud/gophercloud/openstack/identity/v3/services"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/pkg/apis/keystone/v1"
	keystone "github.com/openstack-k8s-operators/keystone-operator/pkg/keystone"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

var log = logf.Log.WithName("controller_keystoneendpoint")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new KeystoneEndpoint Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileKeystoneEndpoint{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("keystoneendpoint-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource KeystoneEndpoint
	err = c.Watch(&source.Kind{Type: &keystonev1.KeystoneEndpoint{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner KeystoneEndpoint
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &keystonev1.KeystoneEndpoint{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileKeystoneEndpoint implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileKeystoneEndpoint{}

// ReconcileKeystoneEndpoint reconciles a KeystoneEndpoint object
type ReconcileKeystoneEndpoint struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a KeystoneEndpoint object and makes changes based on the state read
// and what is in the KeystoneEndpoint.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileKeystoneEndpoint) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling KeystoneEndpoint")

	keystoneAPI := keystone.API(request.Namespace, "keystone")
	objectKey, err := client.ObjectKeyFromObject(keystoneAPI)
	err = r.client.Get(context.TODO(), objectKey, keystoneAPI)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// No KeystoneAPI instance running, return error
			reqLogger.Error(err, "KeystoneAPI instance not found")
			return reconcile.Result{}, err
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if keystoneAPI.Status.BootstrapHash == "" {
		reqLogger.Info("KeystoneAPI bootstrap not complete.", "BootstrapHash", keystoneAPI.Status.BootstrapHash)
		return reconcile.Result{RequeueAfter: time.Second * 5}, err
	}
	reqLogger.Info("KeystoneAPI bootstrap complete.", "BootstrapHash", keystoneAPI.Status.BootstrapHash)

	// Fetch the KeystoneEndpoint instance
	instance := &keystonev1.KeystoneEndpoint{}
	err = r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	reqLogger.Info("authurl", "authurl", instance.Spec.AuthURL)
	reqLogger.Info("username", "username", instance.Spec.Username)
	reqLogger.Info("password", "password", instance.Spec.Password)
	reqLogger.Info("project", "project", instance.Spec.Project)
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: instance.Spec.AuthURL,
		Username:         instance.Spec.Username,
		Password:         instance.Spec.Password,
		TenantName:       instance.Spec.Project,
		DomainName:       instance.Spec.DomainName,
	}

	provider, err := openstack.AuthenticatedClient(opts)
	if err != nil {
		reqLogger.Error(err, "error")
		return reconcile.Result{}, err
	}
	endpointOpts := gophercloud.EndpointOpts{Type: "identity", Region: instance.Spec.Region}
	reqLogger.Info("provider type", "%t", provider)
	reqLogger.Info("IdentityEndpoint", "provider.IdentityEndpoint", provider.IdentityEndpoint)
	reqLogger.Info("IdentityBase", "provider.IdentityBase", provider.IdentityBase)
	identityClient, err := openstack.NewIdentityV3(provider, endpointOpts)

	// Fetch the service ID
	listOpts := services.ListOpts{
		ServiceType: instance.Spec.ServiceType,
	}
	allPages, err := services.List(identityClient, listOpts).AllPages()
	if err != nil {
		return reconcile.Result{}, err
	}
	allServices, err := services.ExtractServices(allPages)
	reqLogger.Info("allServices", "allServices", fmt.Sprintf("%v", allServices))
	if err != nil {
		return reconcile.Result{}, err
	}
	var serviceID string
	for _, service := range allServices {
		if service.Type == instance.Spec.ServiceType {
			serviceID = service.ID
		}
	}
	if serviceID == "" {
		err := fmt.Errorf("Service %s not found", instance.Spec.ServiceType)
		return reconcile.Result{}, err
	}

	reqLogger.Info("Service Id", "Service Id", serviceID)

	reconcileEndpoint(identityClient, serviceID, instance.Spec.ServiceName, instance.Spec.Region, "admin", instance.Spec.AdminURL)
	reconcileEndpoint(identityClient, serviceID, instance.Spec.ServiceName, instance.Spec.Region, "internal", instance.Spec.InternalURL)
	reconcileEndpoint(identityClient, serviceID, instance.Spec.ServiceName, instance.Spec.Region, "public", instance.Spec.PublicURL)

	return reconcile.Result{}, nil
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
