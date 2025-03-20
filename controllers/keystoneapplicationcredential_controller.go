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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/applicationcredentials"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/tokens"
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

// ApplicationCredentialReconciler reconciles an ApplicationCredential object
type ApplicationCredentialReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapplicationcredentials,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapplicationcredentials/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapplicationcredentials/finalizers,verbs=update;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;create;update;delete;patch

func (r *ApplicationCredentialReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.GetLogger(ctx)

	// Fetch the AC CR
	instance := &keystonev1.KeystoneApplicationCredential{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	const finalizer = "openstack.org/applicationcredential"

	if controllerutil.AddFinalizer(instance, finalizer) {
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Initialize conditions on first pass
	if instance.Status.Conditions == nil {
		cl := condition.CreateList(
			condition.UnknownCondition(keystonev1.KeystoneAPIReadyCondition, condition.InitReason, keystonev1.KeystoneAPIReadyInitMessage),
			condition.UnknownCondition(condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage),
		)
		instance.Status.Conditions.Init(&cl)
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Setup helper for use in reconciles
	helperObj, err := helper.NewHelper(instance, r.Client, r.Kclient, r.Scheme, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Save and defer patching all subconditions and overall Ready
	saved := instance.Status.Conditions.DeepCopy()
	defer func() {
		if instance.DeletionTimestamp.IsZero() {
			condition.RestoreLastTransitionTimes(&instance.Status.Conditions, saved)
			if instance.Status.Conditions.AllSubConditionIsTrue() {
				instance.Status.Conditions.MarkTrue(condition.ReadyCondition, condition.ReadyMessage)
			} else {
				instance.Status.Conditions.MarkUnknown(condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage)
				instance.Status.Conditions.Set(instance.Status.Conditions.Mirror(condition.ReadyCondition))
			}
			_ = helperObj.PatchInstance(ctx, instance)
		}
	}()

	// Fetch KeystoneAPI
	keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, helperObj, instance.Namespace, nil)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			keystonev1.KeystoneAPIReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			fmt.Sprintf("KeystoneAPI not found: %v", err),
		))
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if !keystoneAPI.IsReady() {
		instance.Status.Conditions.Set(condition.FalseCondition(
			keystonev1.KeystoneAPIReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			keystonev1.KeystoneAPIReadyWaitingMessage,
		))
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	instance.Status.Conditions.MarkTrue(
		keystonev1.KeystoneAPIReadyCondition,
		condition.ReadyMessage,
	)

	// If KeystoneAPI is being deleted, just drop finalizer
	if !instance.DeletionTimestamp.IsZero() && !keystoneAPI.DeletionTimestamp.IsZero() {
		if controllerutil.RemoveFinalizer(instance, finalizer) {
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance)
	}
	return r.reconcileNormal(ctx, instance, helperObj, keystoneAPI)
}

func (r *ApplicationCredentialReconciler) reconcileNormal(
	ctx context.Context,
	instance *keystonev1.KeystoneApplicationCredential,
	helperObj *helper.Helper,
	keystoneAPI *keystonev1.KeystoneAPI,
) (ctrl.Result, error) {
	logger := r.GetLogger(ctx)

	// Decide if we need to create or rotate
	doRotate, msg := needsRotation(instance)
	if doRotate {
		logger.Info(msg)

		// Build a user-scoped client
		userOS, userRes, userErr := keystonev1.GetUserServiceClient(
			ctx, helperObj, keystoneAPI,
			instance.Spec.UserName,
			instance.Spec.Secret,
			instance.Spec.PasswordSelector,
		)
		if userErr != nil {
			return userRes, userErr
		}
		if userRes != (ctrl.Result{}) {
			return userRes, nil
		}

		// Extract user ID from the authenticated token
		userID, err := r.getUserIDFromToken(logger, userOS.GetOSClient(), instance.Spec.UserName)
		if err != nil {
			logger.Error(err, "Could not get user ID from token")
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.ReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				fmt.Sprintf("Failed to get user ID from token: %s", err.Error()),
			))
			return ctrl.Result{}, err
		}
		logger.Info("Found user ID from token", "userName", instance.Spec.UserName, "userID", userID)

		// Create new AC in Keystone
		newACName := fmt.Sprintf("%s-%s", instance.Name, randomSuffix(5))
		newID, newSecret, expiresAt, err := r.createACWithName(
			logger, userOS.GetOSClient(), userID, instance, newACName,
		)
		if err != nil {
			logger.Error(err, "Could not create AC in Keystone")
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.ReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				fmt.Sprintf("Failed to create AC: %s", err.Error()),
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

		// patch status immediately
		if err := helperObj.PatchInstance(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}

		// requeue to check again before next rotate
		nextCheck := r.computeNextRequeue(instance)
		logger.Info("Rotated AC, requeuing", "nextCheck", nextCheck)
		return ctrl.Result{RequeueAfter: nextCheck}, nil
	}

	logger.Info("AC is up to date", "ACID", instance.Status.ACID)
	instance.Status.Conditions.MarkTrue(condition.ReadyCondition, "Up to date")

	// requeue to check again before next expiry
	nextCheck := r.computeNextRequeue(instance)
	return ctrl.Result{RequeueAfter: nextCheck}, nil
}

func (r *ApplicationCredentialReconciler) reconcileDelete(
	ctx context.Context,
	instance *keystonev1.KeystoneApplicationCredential,
) (ctrl.Result, error) {

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
	ac *keystonev1.KeystoneApplicationCredential,
	newACName string,
) (string, string, time.Time, error) {
	unres := ac.Spec.Unrestricted

	roles := make([]applicationcredentials.Role, len(ac.Spec.Roles))
	for i, rn := range ac.Spec.Roles {
		roles[i] = applicationcredentials.Role{Name: rn}
	}

	// build any user-supplied rules (leave nil if none)
	var rules []applicationcredentials.AccessRule
	for _, ar := range ac.Spec.AccessRules {
		if ar.Service == "" {
			continue
		}
		rule := applicationcredentials.AccessRule{
			Service: ar.Service,
			Path:    ar.Path,
			Method:  ar.Method,
		}
		rules = append(rules, rule)
	}

	createOpts := applicationcredentials.CreateOpts{
		Name:         newACName,
		Description:  fmt.Sprintf("Created by keystone-operator for user %s", ac.Spec.UserName),
		Unrestricted: unres,
		Roles:        roles,
	}

	if len(rules) > 0 {
		createOpts.AccessRules = rules
	}

	if ac.Spec.ExpirationDays > 0 {
		expires := time.Now().Add(time.Duration(ac.Spec.ExpirationDays) * 24 * time.Hour)
		createOpts.ExpiresAt = &expires
	}

	created, err := applicationcredentials.Create(identClient, userID, createOpts).Extract()
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("failed to create AC for user %s: %w", ac.Spec.UserName, err)
	}

	logger.Info("Created AC in Keystone",
		"ACID", created.ID,
		"userID", userID,
		"expiresAt", created.ExpiresAt,
	)
	return created.ID, created.Secret, created.ExpiresAt, nil
}

func (r *ApplicationCredentialReconciler) storeACSecret(
	ctx context.Context,
	helperObj *helper.Helper,
	ac *keystonev1.KeystoneApplicationCredential,
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

// getUserIDFromToken extracts the user ID from the authenticated token
func (r *ApplicationCredentialReconciler) getUserIDFromToken(logger logr.Logger, identClient *gophercloud.ServiceClient, username string) (string, error) {

	// Get the authenticated user from the token
	user, err := tokens.Get(identClient, identClient.TokenID).ExtractUser()
	if err != nil {
		return "", fmt.Errorf("failed to extract user %s from token: %w", username, err)
	}

	if user.ID == "" {
		return "", fmt.Errorf("%w: user ID for user %s", util.ErrNotFound, username)
	}

	logger.Info("Successfully extracted user ID from token", "userID", user.ID)
	return user.ID, nil
}

// computeNextRequeue schedules the next check to happen at "expiry - grace"
func (r *ApplicationCredentialReconciler) computeNextRequeue(ac *keystonev1.KeystoneApplicationCredential) time.Duration {
	// default requeue is 24h as minimal grace period is 1 day
	defaultRequeue := 24 * time.Hour
	if ac.Status.ExpiresAt == nil || ac.Status.ExpiresAt.IsZero() {
		return defaultRequeue
	}

	expiry := ac.Status.ExpiresAt.Time

	grace := time.Duration(ac.Spec.GracePeriodDays) * 24 * time.Hour
	rotateAt := expiry.Add(-grace)

	if rotateAt.Before(time.Now()) {
		// already within the grace window, so trigger rotation immediately
		return 0
	}

	// otherwise check again in 24 hours
	return defaultRequeue
}

// needsRotation returns (shouldRotate, logMessage)
func needsRotation(ac *keystonev1.KeystoneApplicationCredential) (bool, string) {
	if ac.Status.ACID == "" {
		return true, "AC does not exist, creating"
	}
	expiry := ac.Status.ExpiresAt
	if expiry != nil && !expiry.IsZero() {
		// compute grace window
		rotateAt := expiry.Time.Add(-time.Duration(ac.Spec.GracePeriodDays) * 24 * time.Hour)
		if time.Now().After(rotateAt) {
			return true, "AC is within grace period, rotating"
		}
	}
	return false, ""
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
		For(&keystonev1.KeystoneApplicationCredential{}).
		Complete(r)
}

func (r *ApplicationCredentialReconciler) GetLogger(ctx context.Context) logr.Logger {
	return ctrlLog.FromContext(ctx).WithName("ApplicationCredentialReconciler")
}
