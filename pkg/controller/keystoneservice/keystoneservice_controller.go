package keystoneservice

import (
	"context"

	gophercloud "github.com/gophercloud/gophercloud"
	openstack "github.com/gophercloud/gophercloud/openstack"
	services "github.com/gophercloud/gophercloud/openstack/identity/v3/services"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/pkg/apis/keystone/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_keystoneservice")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new KeystoneService Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileKeystoneService{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("keystoneservice-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource KeystoneService
	err = c.Watch(&source.Kind{Type: &keystonev1.KeystoneService{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner KeystoneService
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &keystonev1.KeystoneService{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileKeystoneService implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileKeystoneService{}

// ReconcileKeystoneService reconciles a KeystoneService object
type ReconcileKeystoneService struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a KeystoneService object and makes changes based on the state read
// and what is in the KeystoneService.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileKeystoneService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling KeystoneService")

	// Fetch the KeystoneService instance
	instance := &keystonev1.KeystoneService{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	opts := gophercloud.AuthOptions{
		IdentityEndpoint: instance.Spec.AuthURL,
		Username:         instance.Spec.Username,
		Password:         instance.Spec.Password,
		TenantName:       instance.Spec.Project,
		DomainName:       instance.Spec.DomainName,
	}

	provider, err := openstack.AuthenticatedClient(opts)
	if err != nil {
		return reconcile.Result{}, err
	}
	endpointOpts := gophercloud.EndpointOpts{Type: "identity", Region: instance.Spec.Region}
	identityClient, err := openstack.NewIdentityV3(provider, endpointOpts)

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
			reqLogger.Error(err, "error")
			return reconcile.Result{}, err
		}

		// Set ServiceID in the status
		reqLogger.Info("instance.Status.ServiceID", "ServiceID", instance.Status.ServiceID)
		reqLogger.Info("service.ID", "service.ID", service.ID)
		if instance.Status.ServiceID != service.ID {
			instance.Status.ServiceID = service.ID
			if err := r.client.Status().Update(context.TODO(), instance); err != nil {
				reqLogger.Error(err, "error")
				return reconcile.Result{}, err
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
			reqLogger.Error(err, "error")
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}
