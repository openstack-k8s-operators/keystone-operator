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
	"strings"
	"time"

	"github.com/go-logr/logr"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	external "github.com/openstack-k8s-operators/keystone-operator/pkg/external"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"

	openstack "github.com/openstack-k8s-operators/lib-common/modules/openstack"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// GetClient -
func (r *KeystoneServiceReconciler) GetClient() client.Client {
	return r.Client
}

// GetKClient -
func (r *KeystoneServiceReconciler) GetKClient() kubernetes.Interface {
	return r.Kclient
}

// GetLogger -
func (r *KeystoneServiceReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetScheme -
func (r *KeystoneServiceReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

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

	helper, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		r.Log,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() {
		if err := helper.SetAfter(instance); err != nil {
			util.LogErrorForObject(helper, err, "Set after and calc patch/diff", instance)
		}

		if changed := helper.GetChanges()["status"]; changed {
			patch := client.MergeFrom(helper.GetBeforeObject())

			if err := r.Status().Patch(ctx, instance, patch); err != nil && !k8s_errors.IsNotFound(err) {
				util.LogErrorForObject(helper, err, "Update status", instance)
			}
		}
	}()

	//
	// Validate that keystoneAPI is up
	//
	keystoneAPI, err := external.GetKeystoneAPI(ctx, helper, instance.Namespace, map[string]string{})
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			r.Log.Info("KeystoneAPI not found!")
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
		return ctrl.Result{}, err
	}

	if !keystoneAPI.IsReady() {
		r.Log.Info("KeystoneAPI not yet ready")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	//
	// get admin authentication OpenStack
	//
	os, cond, ctrlResult, err := external.GetAdminServiceClient(
		ctx,
		helper,
		keystoneAPI,
	)
	instance.Status.Conditions.UpdateCurrentCondition(cond)
	if err != nil {
		return ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper, os)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper, os)

}

// SetupWithManager x
func (r *KeystoneServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keystonev1.KeystoneService{}).
		Complete(r)
}

func (r *KeystoneServiceReconciler) reconcileDelete(
	ctx context.Context,
	instance *keystonev1.KeystoneService,
	helper *helper.Helper,
	os *openstack.OpenStack,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service delete")

	// only cleanup the service if there is the ServiceID reference in the
	// object status
	if instance.Status.ServiceID != "" {
		// Delete Endpoints
		for endpointInterface := range instance.Spec.APIEndpoints {
			// get the gopher availability mapping for the endpointInterface
			availability, err := openstack.GetAvailability(endpointInterface)
			if err != nil {
				return ctrl.Result{}, err
			}

			err = os.DeleteEndpoint(
				r.Log,
				openstack.Endpoint{
					Name:         instance.Spec.ServiceName,
					ServiceID:    instance.Status.ServiceID,
					Availability: availability,
				},
			)
			if err != nil {
				return ctrl.Result{}, err
			}
		}

		// Delete User
		err := os.DeleteUser(
			r.Log,
			instance.Spec.ServiceUser)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Delete Service
		err = os.DeleteService(
			r.Log,
			instance.Status.ServiceID)
		if err != nil {
			r.Log.Info(err.Error())
			return ctrl.Result{}, err
		}

	} else {
		r.Log.Info(fmt.Sprintf("Not deleting service %s as there is no stores service ID", instance.Spec.ServiceName))
	}

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	r.Log.Info("Reconciled Service delete successfully")
	if err := r.Update(ctx, instance); err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *KeystoneServiceReconciler) reconcileNormal(
	ctx context.Context,
	instance *keystonev1.KeystoneService,
	helper *helper.Helper,
	os *openstack.OpenStack,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service")

	// If the service object doesn't have our finalizer, add it.
	controllerutil.AddFinalizer(instance, helper.GetFinalizer())
	// Register the finalizer immediately to avoid orphaning resources on delete
	if err := r.Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	//
	// Create new service if ServiceID is not already set
	//
	err := r.reconcileService(instance, os)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// create/update/delete endpoint
	//
	err = r.reconcileEndpoints(
		instance,
		os)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// create/update service user
	//
	ctrlResult, err := r.reconcileUser(
		ctx,
		helper,
		instance,
		os)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	r.Log.Info("Reconciled Service successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneServiceReconciler) reconcileService(
	instance *keystonev1.KeystoneService,
	os *openstack.OpenStack,
) error {
	r.Log.Info(fmt.Sprintf("Reconciling Service %s", instance.Spec.ServiceName))

	// Create new service if ServiceID is not already set
	if instance.Status.ServiceID == "" {

		// verify if there is still an existing service in keystone for
		// type and name, if so register the ID
		service, err := os.GetService(
			r.Log,
			instance.Spec.ServiceType,
			instance.Spec.ServiceName,
		)
		// If the service is not found, don't count that as an error here
		if err != nil && !strings.Contains(err.Error(), "service not found in keystone") {
			return err
		}

		// if there is already a service registered use it
		if service != nil && instance.Status.ServiceID != service.ID {
			instance.Status.ServiceID = service.ID
		} else {
			instance.Status.ServiceID, err = os.CreateService(
				r.Log,
				openstack.Service{
					Name:        instance.Spec.ServiceName,
					Type:        instance.Spec.ServiceType,
					Description: instance.Spec.ServiceDescription,
					Enabled:     instance.Spec.Enabled,
				})
			if err != nil {
				return err
			}
		}
	} else {
		// ServiceID is already set, update the service
		err := os.UpdateService(
			r.Log,
			openstack.Service{
				Name:        instance.Spec.ServiceName,
				Type:        instance.Spec.ServiceType,
				Description: instance.Spec.ServiceDescription,
				Enabled:     instance.Spec.Enabled,
			},
			instance.Status.ServiceID)
		if err != nil {
			return err
		}
	}

	r.Log.Info("Reconciled Service successfully")
	return nil
}

func (r *KeystoneServiceReconciler) reconcileEndpoints(
	instance *keystonev1.KeystoneService,
	os *openstack.OpenStack,
) error {
	r.Log.Info("Reconciling Endpoints")

	// create / update endpoints
	for endpointInterface, url := range instance.Spec.APIEndpoints {

		// get the gopher availability mapping for the endpointInterface
		availability, err := openstack.GetAvailability(endpointInterface)
		if err != nil {
			return err
		}

		// get registered endpoints for the service and endpointInterface
		allEndpoints, err := os.GetEndpoints(
			r.Log,
			instance.Status.ServiceID,
			endpointInterface)
		if err != nil {
			return err
		}

		if len(allEndpoints) == 1 {
			endpoint := allEndpoints[0]
			if url != endpoint.URL {
				// Update the endpoint
				_, err := os.UpdateEndpoint(
					r.Log,
					openstack.Endpoint{
						Name:         endpoint.Name,
						ServiceID:    endpoint.ServiceID,
						Availability: availability,
						URL:          url,
					},
					endpoint.ID,
				)
				if err != nil {
					return err
				}
			}
		} else {
			// Create the endpoint
			_, err := os.CreateEndpoint(
				r.Log,
				openstack.Endpoint{
					Name:         instance.Spec.ServiceName,
					ServiceID:    instance.Status.ServiceID,
					Availability: availability,
					URL:          url,
				},
			)
			if err != nil {
				return err
			}
		}
	}

	r.Log.Info("Reconciled Endpoints successfully")
	return nil
}

func (r *KeystoneServiceReconciler) reconcileUser(
	ctx context.Context,
	h *helper.Helper,
	instance *keystonev1.KeystoneService,
	os *openstack.OpenStack,
) (reconcile.Result, error) {
	r.Log.Info(fmt.Sprintf("Reconciling User %s", instance.Spec.ServiceUser))
	roleName := "admin"

	// get the password of the service user from the secret
	password, _, ctrlResult, err := secret.GetDataFromSecret(
		ctx,
		h,
		instance.Spec.Secret,
		10,
		instance.Spec.PasswordSelector)
	if err != nil {
		return ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	//  create service project if it does not exist
	//
	serviceProjectID, err := os.CreateProject(
		r.Log,
		openstack.Project{
			Name:        "service",
			Description: "service",
		})
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	//  create role if it does not exist
	//
	_, err = os.CreateRole(
		r.Log,
		roleName)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// create user if it does not exist
	//
	userID, err := os.CreateUser(
		r.Log,
		openstack.User{
			Name:      instance.Spec.ServiceUser,
			Password:  password,
			ProjectID: serviceProjectID,
		})
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// add user to admin role
	//
	err = os.AssignUserRole(
		r.Log,
		roleName,
		userID,
		serviceProjectID)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.Log.Info("Reconciled User successfully")
	return ctrl.Result{}, nil
}
