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
	Scheme  *runtime.Scheme
}

//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints/finalizers,verbs=update
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;update;patch
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis/finalizers,verbs=update
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;update;patch
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices/finalizers,verbs=update

// Reconcile keystone endpoint requests
func (r *KeystoneEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	l := GetLog(ctx)

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

	helper, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		//TODO remove later, log used here as to not break the helper struct signiture.
		l,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() {
		// update the Ready condition based on the sub conditions
		if instance.Status.Conditions.AllSubConditionIsTrue() {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, condition.ReadyMessage)
		} else {
			// something is not ready so reset the Ready condition
			instance.Status.Conditions.MarkUnknown(
				condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage)
			// and recalculate it based on the state of the rest of the conditions
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}
		err := helper.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	// If we're not deleting this and the service object doesn't have our finalizer, add it.
	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, helper.GetFinalizer()) {
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
		return ctrl.Result{}, nil
	}

	//
	// Validate that keystoneAPI is up
	//
	keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, helper, instance.Namespace, map[string]string{})
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// If this KeystoneEndpoint CR is being deleted and it has not registered any actual
			// endpoints on the OpenStack side, just redirect execution to the "reconcileDelete()"
			// logic to avoid potentially hanging on waiting for a KeystoneAPI to appear (which
			// is not needed anyhow, since there is nothing to clean-up on the OpenStack side)
			if !instance.DeletionTimestamp.IsZero() && len(instance.Status.EndpointIDs) == 0 {
				return r.reconcileDelete(ctx, instance, helper, nil, nil)
			}

			instance.Status.Conditions.Set(condition.FalseCondition(
				keystonev1.KeystoneAPIReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				keystonev1.KeystoneAPIReadyNotFoundMessage,
			))
			l.Info("KeystoneAPI not found!")

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

	// If this KeystoneEndpoint CR is being deleted and it has not registered any actual
	// endpoints on the OpenStack side, just redirect execution to the "reconcileDelete()"
	// logic to avoid potentially hanging on waiting for the KeystoneAPI to be ready
	// (which is not needed anyhow, since there is nothing to clean-up on the OpenStack
	// side)
	if !instance.DeletionTimestamp.IsZero() && len(instance.Status.EndpointIDs) == 0 {
		return r.reconcileDelete(ctx, instance, helper, nil, keystoneAPI)
	}

	//
	// Add a finalizer to the KeystoneAPI for this endpoint instance, as we do not want
	// the KeystoneAPI to disappear before this endpoint in the case where this endpoint
	// is deleted
	//
	if controllerutil.AddFinalizer(keystoneAPI, fmt.Sprintf("%s-%s", helper.GetFinalizer(), instance.Name)) {
		err := r.Update(ctx, keystoneAPI)

		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if !keystoneAPI.IsReady() {
		instance.Status.Conditions.Set(condition.FalseCondition(
			keystonev1.KeystoneAPIReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			keystonev1.KeystoneAPIReadyWaitingMessage))
		l.Info("KeystoneAPI not yet ready!")

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

	// Handle normal endpoint delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper, os, keystoneAPI)
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
	keystoneAPI *keystonev1.KeystoneAPI,
) (ctrl.Result, error) {
	l := GetLog(ctx)

	l.Info("Reconciling Endpoint delete")

	// We might not have an OpenStack backend to use in certain situations
	if os != nil {
		// Delete Endpoints -  it is ok to call delete on non existing Endpoints
		// therefore always call delete for the spec.
		for endpointType := range instance.Spec.Endpoints {
			// get the gopher availability mapping for the endpointInterface
			availability, err := openstack.GetAvailability(endpointType)
			if err != nil {
				return ctrl.Result{}, err
			}

			err = os.DeleteEndpoint(
				l,
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
	}

	ksSvc, err := keystonev1.GetKeystoneServiceWithName(ctx, helper, instance.Spec.ServiceName, instance.Namespace)
	if err == nil {
		// Remove the finalizer for this endpoint from the Service
		if controllerutil.RemoveFinalizer(ksSvc, fmt.Sprintf("%s-%s", helper.GetFinalizer(), instance.Name)) {
			err := r.Update(ctx, ksSvc)

			if err != nil {
				return ctrl.Result{}, err
			}
		}
	} else if !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// There are certain deletion scenarios where we might not have the keystoneAPI
	if keystoneAPI != nil {
		// Remove the finalizer for this endpoint from the KeystoneAPI
		if controllerutil.RemoveFinalizer(keystoneAPI, fmt.Sprintf("%s-%s", helper.GetFinalizer(), instance.Name)) {
			err := r.Update(ctx, keystoneAPI)

			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// Endpoints are deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	l.Info("Reconciled Endpoint delete successfully")

	return ctrl.Result{}, nil
}

func (r *KeystoneEndpointReconciler) reconcileNormal(
	ctx context.Context,
	instance *keystonev1.KeystoneEndpoint,
	helper *helper.Helper,
	os *openstack.OpenStack,
) (ctrl.Result, error) {
	l := GetLog(ctx)
	l.Info("Reconciling Endpoint normal")

	//
	// Wait for KeystoneService is Ready and get the ServiceID from the object
	//
	ksSvc, err := keystonev1.GetKeystoneServiceWithName(ctx, helper, instance.Spec.ServiceName, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			l.Info("KeystoneService not found", "KeystoneService", instance.Spec.ServiceName)
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
		l.Info("KeystoneService not ready, waiting to create endpoints", "KeystoneService", instance.Spec.ServiceName)

		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	instance.Status.ServiceID = ksSvc.Status.ServiceID

	//
	// Add a finalizer to KeystoneService, because KeystoneEndpoint is dependent on
	// the service entry created by KeystoneService
	//
	if controllerutil.AddFinalizer(ksSvc, fmt.Sprintf("%s-%s", helper.GetFinalizer(), instance.Name)) {
		err := r.Update(ctx, ksSvc)

		if err != nil {
			return ctrl.Result{}, err
		}
	}

	//
	// create/update endpoints
	//
	err = r.reconcileEndpoints(
		ctx,
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

	l.Info("Reconciled Endpoint normal successfully")

	return ctrl.Result{}, nil
}

func (r *KeystoneEndpointReconciler) reconcileEndpoints(
	ctx context.Context,
	instance *keystonev1.KeystoneEndpoint,
	helper *helper.Helper,
	os *openstack.OpenStack,
) error {
	l := GetLog(ctx)
	l.Info("Reconciling Endpoints")

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
					l,
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
			l,
			instance.Status.ServiceID,
			endpointType)
		if err != nil {
			return err
		}

		endpointID := ""
		if len(allEndpoints) == 0 {
			// Create the endpoint
			endpointID, err = os.CreateEndpoint(
				l,
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
					l,
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

	l.Info("Reconciled Endpoints successfully")

	return nil
}
