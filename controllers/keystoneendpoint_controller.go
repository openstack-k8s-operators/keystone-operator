/*
Copyright 2022.

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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	openstack "github.com/openstack-k8s-operators/lib-common/modules/openstack"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// KeystoneEndpointReconciler reconciles a KeystoneEndpoint object
type KeystoneEndpointReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints/finalizers,verbs=update
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list

// Reconcile keystone endpoint requests
func (r *KeystoneEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// Fetch the KeystoneEndpoint instance
	instance := &keystonev1.KeystoneEndpoint{}
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

	//
	// initialize status
	//
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
		cl := condition.CreateList(
			condition.UnknownCondition(keystonev1.KeystoneAPIReadyCondition, condition.InitReason, keystonev1.KeystoneAPIReadyInitMessage),
			condition.UnknownCondition(keystonev1.AdminServiceClientReadyCondition, condition.InitReason, keystonev1.AdminServiceClientReadyInitMessage),
			condition.UnknownCondition(keystonev1.KeystoneServiceOSEndpointsReadyCondition, condition.InitReason, keystonev1.KeystoneServiceOSEndpointsReadyInitMessage),
			// right now we have no dedicated KeystoneServiceReadyInitMessage
			condition.UnknownCondition(condition.KeystoneServiceReadyCondition, condition.InitReason, ""),
		)
		instance.Status.Conditions.Init(&cl)

		// Register overall status immediately to have an early feedback e.g. in the cli
		if err := r.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
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
		// update the overall status condition if endpoints are ready
		if instance.IsReady() {
			instance.Status.Conditions.MarkTrue(condition.ReadyCondition, condition.ReadyMessage)
		}

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
	keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, helper, instance.Namespace, map[string]string{})
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				keystonev1.KeystoneAPIReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				keystonev1.KeystoneAPIReadyNotFoundMessage,
			))
			util.LogForObject(helper, "KeystoneAPI not found!", instance)

			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			keystonev1.KeystoneAPIReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			keystonev1.KeystoneAPIReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if !keystoneAPI.IsReady() {
		instance.Status.Conditions.Set(condition.FalseCondition(
			keystonev1.KeystoneAPIReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			keystonev1.KeystoneAPIReadyWaitingMessage))
		util.LogForObject(helper, "KeystoneAPI not yet ready!", instance)

		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}
	instance.Status.Conditions.MarkTrue(keystonev1.KeystoneAPIReadyCondition, keystonev1.KeystoneAPIReadyMessage)

	//
	// get admin authentication OpenStack
	//
	os, ctrlResult, err := keystonev1.GetAdminServiceClient(
		ctx,
		helper,
		keystoneAPI,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			keystonev1.AdminServiceClientReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			keystonev1.AdminServiceClientReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			keystonev1.AdminServiceClientReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			keystonev1.AdminServiceClientReadyWaitingMessage))
		return ctrlResult, nil
	}
	instance.Status.Conditions.MarkTrue(keystonev1.AdminServiceClientReadyCondition, keystonev1.AdminServiceClientReadyMessage)

	// update status to save current conditions to object before sub-reconcilation rules start
	if err := r.Status().Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	// Handle endpoint delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper, os)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper, os)
}

// SetupWithManager sets up the controller with the Manager.
func (r *KeystoneEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keystonev1.KeystoneEndpoint{}).
		Complete(r)
}

func (r *KeystoneEndpointReconciler) reconcileDelete(
	ctx context.Context,
	instance *keystonev1.KeystoneEndpoint,
	helper *helper.Helper,
	os *openstack.OpenStack,
) (ctrl.Result, error) {
	util.LogForObject(helper, "Reconciling Endpoint delete", instance)

	// Delete Endpoints -  it is ok to call delete on non existing Endpoints
	// therefore always call delete for the spec.
	for endpointType := range instance.Spec.Endpoints {
		// get the gopher availability mapping for the endpointInterface
		availability, err := openstack.GetAvailability(endpointType)
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

	// Endpoints are deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	util.LogForObject(helper, "Reconciled Endpoint delete successfully", instance)

	if err := r.Update(ctx, instance); err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *KeystoneEndpointReconciler) reconcileNormal(
	ctx context.Context,
	instance *keystonev1.KeystoneEndpoint,
	helper *helper.Helper,
	os *openstack.OpenStack,
) (ctrl.Result, error) {
	util.LogForObject(helper, "Reconciling Endpoint normal", instance)

	if !controllerutil.ContainsFinalizer(instance, helper.GetFinalizer()) {
		// If the service object doesn't have our finalizer, add it.
		controllerutil.AddFinalizer(instance, helper.GetFinalizer())
		// Register the finalizer immediately to avoid orphaning resources on delete
		err := r.Update(ctx, instance)

		return ctrl.Result{}, err
	}

	//
	// Wait for KeystoneService is Ready and get the ServiceID from the object
	//
	ksSvc, err := keystonev1.GetKeystoneServiceWithName(ctx, helper, instance.Spec.ServiceName, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			util.LogForObject(helper, fmt.Sprintf("KeystoneService %s not found", instance.Spec.ServiceName), instance)
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}

		return ctrl.Result{}, err
	}
	// mirror the Status, Reason, Severity and Message of the latest keystoneservice condition
	// into a local condition with the type condition.KeystoneServiceReadyCondition
	c := ksSvc.Status.Conditions.Mirror(condition.KeystoneServiceReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	if !ksSvc.IsReady() {
		util.LogForObject(helper, fmt.Sprintf("KeystoneService %s not ready, waiting to create endpoints", instance.Spec.ServiceName), instance)

		return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
	}

	instance.Status.ServiceID = ksSvc.Status.ServiceID

	//
	// create/update endpoints
	//
	err = r.reconcileEndpoints(
		instance,
		helper,
		os)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			keystonev1.KeystoneServiceOSEndpointsReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			keystonev1.KeystoneServiceOSEndpointsReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(
		keystonev1.KeystoneServiceOSEndpointsReadyCondition,
		keystonev1.KeystoneServiceOSEndpointsReadyMessage,
		instance.Spec.Endpoints,
	)

	util.LogForObject(helper, "Reconciled Endpoint normal successfully", instance)

	return ctrl.Result{}, nil
}

func (r *KeystoneEndpointReconciler) reconcileEndpoints(
	instance *keystonev1.KeystoneEndpoint,
	helper *helper.Helper,
	os *openstack.OpenStack,
) error {
	util.LogForObject(helper, "Reconciling Endpoints", instance)

	// delete endpoint if it does no longer exist in Spec.Endpoints
	// but has a reference in Status.EndpointIDs
	if instance.Status.EndpointIDs != nil {
		for endpointType := range instance.Status.EndpointIDs {
			if _, ok := instance.Spec.Endpoints[endpointType]; !ok {
				// get the gopher availability mapping for the endpointInterface
				availability, err := openstack.GetAvailability(endpointType)
				if err != nil {
					return err
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
					return err
				}

				// remove endpoint reference from status
				delete(instance.Status.EndpointIDs, endpointType)
			}
		}
	}

	// create / update endpoints
	for endpointType, endpointURL := range instance.Spec.Endpoints {

		// get the gopher availability mapping for the endpointType
		availability, err := openstack.GetAvailability(endpointType)
		if err != nil {
			return err
		}

		// get registered endpoints for the service and endpointType
		allEndpoints, err := os.GetEndpoints(
			r.Log,
			instance.Status.ServiceID,
			endpointType)
		if err != nil {
			return err
		}

		endpointID := ""
		if len(allEndpoints) == 0 {
			// Create the endpoint
			endpointID, err = os.CreateEndpoint(
				r.Log,
				openstack.Endpoint{
					Name:         instance.Spec.ServiceName,
					ServiceID:    instance.Status.ServiceID,
					Availability: availability,
					URL:          endpointURL,
				},
			)
			if err != nil {
				return err
			}
		} else if len(allEndpoints) == 1 {
			// Update the endpoint if URL changed
			endpoint := allEndpoints[0]
			if endpointURL != endpoint.URL {
				endpointID, err = os.UpdateEndpoint(
					r.Log,
					openstack.Endpoint{
						Name:         endpoint.Name,
						ServiceID:    endpoint.ServiceID,
						Availability: availability,
						URL:          endpointURL,
					},
					endpoint.ID,
				)
				if err != nil {
					return err
				}
			}
		} else {
			// If there are multiple endpoints for the service and endpoint type log it as an error
			// as manual check is required
			return util.WrapErrorForObject(
				fmt.Sprintf("multiple endpoints registered for service:%s type: %s",
					instance.Spec.ServiceName, endpointType),
				instance, err)
		}

		if instance.Status.EndpointIDs == nil {
			instance.Status.EndpointIDs = map[string]string{}
		}
		if _, ok := instance.Spec.Endpoints[endpointType]; ok && endpointID != "" {
			instance.Status.EndpointIDs[endpointType] = endpointID
		}
	}

	util.LogForObject(helper, "Reconciled Endpoints successfully", instance)

	return nil
}
