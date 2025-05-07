/*
Copyright 2025.

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
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/applicationcredentials"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	oko_secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"
)

// TODO: move this to conditions
var ErrAdminServiceClientNotReady = errors.New("admin client not ready")

// ApplicationCredentialReconciler reconciles an ApplicationCredential object
type ApplicationCredentialReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

//+kubebuilder:rbac:groups=keystone.openstack.org,resources=applicationcredentials,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=applicationcredentials/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=applicationcredentials/finalizers,verbs=update;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;create;update;delete;patch

func (r *ApplicationCredentialReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.GetLogger(ctx)

	// Fetch the CR
	instance := &keystonev1.ApplicationCredential{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Setup helper
	helperObj, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		logger,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Initialize conditions if needed
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
	}
	savedConditions := instance.Status.Conditions.DeepCopy()

	// Defer patch logic (skips if we are deleting)
	defer func() {
		if !instance.DeletionTimestamp.IsZero() {
			return
		}
		condition.RestoreLastTransitionTimes(&instance.Status.Conditions, savedConditions)
		// Mirror the top-level Ready condition
		if instance.Status.Conditions.IsUnknown(condition.ReadyCondition) {
			instance.Status.Conditions.Set(instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}
		_ = helperObj.PatchInstance(ctx, instance)
	}()

	// Ensure we have an initial ReadyCondition
	condList := condition.CreateList(
		condition.UnknownCondition(condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage),
	)
	instance.Status.Conditions.Init(&condList)

	// Check if marked for deletion
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helperObj)
	}

	// Normal reconcile
	return r.reconcileNormal(ctx, instance, helperObj)
}

func (r *ApplicationCredentialReconciler) reconcileNormal(
	ctx context.Context,
	instance *keystonev1.ApplicationCredential,
	helperObj *helper.Helper,
) (ctrl.Result, error) {

	logger := r.GetLogger(ctx)

	// Add finalizer if not present
	const finalizer = "openstack.org/applicationcredential"
	if controllerutil.AddFinalizer(instance, finalizer) {
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		// Return so the next pass sees finalizer
		return ctrl.Result{}, nil
	}

	// Check if KeystoneAPI is ready
	keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, helperObj, instance.Namespace, nil)
	if err != nil {
		logger.Info("KeystoneAPI not found, requeue", "error", err)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if !keystoneAPI.IsReady() {
		logger.Info("KeystoneAPI not ready, requeue")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Decide if we need to create (or rotate)
	needsRotation := false
	if instance.Status.ACID == "" {
		needsRotation = true
		logger.Info("AC does not exist, creating")
	} else if r.shouldRotateNow(instance) {
		needsRotation = true
		logger.Info("AC is within grace period, rotating")
	}

	if needsRotation {
		// Find user ID
		userID, err := r.findUserIDAsAdmin(ctx, helperObj, keystoneAPI, instance.Spec.UserName)
		if err != nil {
			logger.Error(err, "Failed to find user ID")
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.ReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				"User lookup failed %s",
				err.Error(),
			))
			return ctrl.Result{}, err
		}
		logger.Info("Found user ID", "userName", instance.Spec.UserName, "userID", userID)

		// Build user-scoped client
		userOS, userClientRes, userClientErr := keystonev1.GetUserServiceClient(ctx, helperObj, keystoneAPI, instance.Spec.UserName)
		if userClientErr != nil {
			return userClientRes, userClientErr
		}
		if userClientRes != (ctrl.Result{}) {
			// Requeue if we got a partial result
			return userClientRes, nil
		}

		// Create new AC in Keystone
		newACName := fmt.Sprintf("%s-%s", instance.Name, randomSuffix(5))
		newID, newSecret, expiresAt, err := r.createACWithName(logger, userOS.GetOSClient(), userID, instance, newACName)
		if err != nil {
			logger.Error(err, "Could not create AC in Keystone")
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.ReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				"Failed to create AC %s",
				err.Error(),
			))
			return ctrl.Result{}, err
		}
		logger.Info("Created new AC in Keystone", "ACID", newID, "expiresAt", expiresAt)

		// Store it in a Secret
		secretName := fmt.Sprintf("%s-secret", instance.Name)
		if err := r.storeACSecret(ctx, helperObj, instance, secretName, newID, newSecret); err != nil {
			return ctrl.Result{}, err
		}

		// Update status
		instance.Status.ACID = newID
		instance.Status.SecretName = secretName
		instance.Status.CreatedAt = &metav1.Time{Time: time.Now().UTC()}
		instance.Status.ExpiresAt = &metav1.Time{Time: expiresAt}

		instance.Status.Conditions.MarkTrue(condition.ReadyCondition, "AC is ready")

		// Immediate patch
		if patchErr := helperObj.PatchInstance(ctx, instance); patchErr != nil {
			return ctrl.Result{}, patchErr
		}

		// Requeue
		nextCheck := r.computeNextRequeue(instance)
		logger.Info("Rotated AC, requeuing to check again", "nextCheck", nextCheck)
		return ctrl.Result{RequeueAfter: nextCheck}, nil
	}

	logger.Info("AC is up to date", "ACID", instance.Status.ACID)
	instance.Status.Conditions.MarkTrue(condition.ReadyCondition, "Up to date")

	// Requeue to check again before next expiry
	nextCheck := r.computeNextRequeue(instance)
	return ctrl.Result{RequeueAfter: nextCheck}, nil
}

func (r *ApplicationCredentialReconciler) reconcileDelete(
	ctx context.Context,
	instance *keystonev1.ApplicationCredential,
	helperObj *helper.Helper,
) (ctrl.Result, error) {

	logger := r.GetLogger(ctx)

	// Only if user explicitly does "oc delete" do we revoke the AC
	if instance.Status.ACID != "" {
		keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, helperObj, instance.Namespace, nil)
		if err == nil && keystoneAPI.IsReady() {
			userID, userErr := r.findUserIDAsAdmin(ctx, helperObj, keystoneAPI, instance.Spec.UserName)
			if userErr == nil {
				userOS, userRes, userErr2 := keystonev1.GetUserServiceClient(ctx, helperObj, keystoneAPI, instance.Spec.UserName)
				if userErr2 == nil && userRes == (ctrl.Result{}) && userOS != nil {
					delErr := applicationcredentials.Delete(userOS.GetOSClient(), userID, instance.Status.ACID).ExtractErr()
					if delErr != nil {
						logger.Error(delErr, "Failed to delete AC from Keystone", "ACID", instance.Status.ACID)
					} else {
						logger.Info("Deleted AC from Keystone", "ACID", instance.Status.ACID)
					}
				}
			}
		}
	}

	// Remove finalizer
	const finalizer = "openstack.org/applicationcredential"
	if controllerutil.RemoveFinalizer(instance, finalizer) {
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// createACWithName creates a new AC in Keystone
func (r *ApplicationCredentialReconciler) createACWithName(
	logger logr.Logger,
	identClient *gophercloud.ServiceClient,
	userID string,
	ac *keystonev1.ApplicationCredential,
	newACName string,
) (string, string, time.Time, error) {
	createOpts := applicationcredentials.CreateOpts{
		Name:         newACName,
		Description:  fmt.Sprintf("Created by keystone-operator for user %s", ac.Spec.UserName),
		Unrestricted: false,

		// Always assign these roles:
		Roles: []applicationcredentials.Role{
			{Name: "admin"},
			{Name: "service"},
		},
	}

	if ac.Spec.ExpirationDays > 0 {
		expires := time.Now().Add(time.Duration(ac.Spec.ExpirationDays) * 24 * time.Hour)
		createOpts.ExpiresAt = &expires
	}

	created, err := applicationcredentials.Create(identClient, userID, createOpts).Extract()
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("failed to create AC for user %s: %w", ac.Spec.UserName, err)
	}

	actualExpires := created.ExpiresAt
	logger.Info("Created AC in Keystone",
		"ACID", created.ID,
		"userID", userID,
		"expiresAt", actualExpires)
	return created.ID, created.Secret, actualExpires, nil
}

func (r *ApplicationCredentialReconciler) storeACSecret(
	ctx context.Context,
	helperObj *helper.Helper,
	ac *keystonev1.ApplicationCredential,
	secretName, newID, newSecret string,
) error {

	data := map[string]string{
		"AC_ID":     newID,
		"AC_SECRET": newSecret,
	}
	tmpl := []util.Template{{
		Name:       secretName,
		Namespace:  ac.Namespace,
		Type:       util.TemplateTypeNone,
		CustomData: data,
	}}
	return oko_secret.EnsureSecrets(ctx, helperObj, ac, tmpl, nil)
}

// findUserIDAsAdmin uses admin-scoped token to look up user ID
func (r *ApplicationCredentialReconciler) findUserIDAsAdmin(
	ctx context.Context,
	helperObj *helper.Helper,
	keystoneAPI *keystonev1.KeystoneAPI,
	userName string,
) (string, error) {

	logger := r.GetLogger(ctx)
	adminOS, ctrlResult, err := keystonev1.GetAdminServiceClient(ctx, helperObj, keystoneAPI)
	if err != nil {
		return "", err
	}
	if ctrlResult != (ctrl.Result{}) {
		return "", ErrAdminServiceClientNotReady
	}

	userObj, err := adminOS.GetUser(logger, userName, "Default")
	if err != nil {
		return "", fmt.Errorf("cannot find user %q: %w", userName, err)
	}
	return userObj.ID, nil
}

// shouldRotateNow returns true if we are already within GracePeriodDays
func (r *ApplicationCredentialReconciler) shouldRotateNow(ac *keystonev1.ApplicationCredential) bool {
	if ac.Status.ExpiresAt == nil || ac.Status.ExpiresAt.IsZero() {
		return false
	}

	expiry := ac.Status.ExpiresAt.Time
	grace := time.Duration(ac.Spec.GracePeriodDays) * 24 * time.Hour
	rotateAt := expiry.Add(-grace)
	return time.Now().After(rotateAt)
}

// computeNextRequeue schedules the next check to happen at "expiry - grace"
func (r *ApplicationCredentialReconciler) computeNextRequeue(ac *keystonev1.ApplicationCredential) time.Duration {
	// default requeue is 24h as minimal grace period is 1 day
	defaultRequeue := 24 * time.Hour
	if ac.Status.ExpiresAt == nil || ac.Status.ExpiresAt.IsZero() {
		return defaultRequeue
	}

	expiry := ac.Status.ExpiresAt.Time

	grace := time.Duration(ac.Spec.GracePeriodDays) * 24 * time.Hour
	rotateAt := expiry.Add(-grace)

	if rotateAt.Before(time.Now()) {
		// Already within the grace window, so trigger rotation immediately
		return 0
	}

	// Otherwise check again in 24 hours
	return defaultRequeue
}

// randomSuffix generates a random string suffix
func randomSuffix(n int) string {
	b := make([]byte, (n+1)/2)
	_, err := rand.Read(b)
	if err != nil {
		return "fallback" // placeholder for generating failure
	}
	s := hex.EncodeToString(b)
	return s[:n]
}

func (r *ApplicationCredentialReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keystonev1.ApplicationCredential{}).
		Complete(r)
}

func (r *ApplicationCredentialReconciler) GetLogger(ctx context.Context) logr.Logger {
	return ctrlLog.FromContext(ctx).WithName("ApplicationCredentialReconciler")
}
