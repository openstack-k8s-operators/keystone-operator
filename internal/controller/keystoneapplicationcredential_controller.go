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

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/applicationcredentials"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	oko_secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"
)

const acSecretFinalizer = "openstack.org/ac-secret-protection" // #nosec G101
const finalizer = "openstack.org/applicationcredential"        // #nosec G101

// ApplicationCredentialReconciler reconciles an ApplicationCredential object
type ApplicationCredentialReconciler struct {
	client.Client
	Kclient       kubernetes.Interface
	Log           logr.Logger
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
}

//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapplicationcredentials,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapplicationcredentials/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapplicationcredentials/finalizers,verbs=update;patch
//+kubebuilder:rbac:groups=core,resources=secrets/finalizers,verbs=get;list;create;update;delete;patch
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile reconciles a KeystoneApplicationCredential resource.
func (r *ApplicationCredentialReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.GetLogger(ctx)

	// Fetch the ApplicationCredential CR
	instance := &keystonev1.KeystoneApplicationCredential{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Add finalizer only if not deleting and not already present
	if instance.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(instance, finalizer) {
		base := instance.DeepCopy()
		controllerutil.AddFinalizer(instance, finalizer)
		if err := r.Patch(ctx, instance, client.MergeFrom(base)); err != nil {
			if k8s_errors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize conditions on first pass
	if instance.Status.Conditions == nil {
		cl := condition.CreateList(
			condition.UnknownCondition(keystonev1.KeystoneAPIReadyCondition, condition.InitReason, keystonev1.KeystoneAPIReadyInitMessage),
			condition.UnknownCondition(keystonev1.KeystoneApplicationCredentialReadyCondition, condition.InitReason, keystonev1.KeystoneApplicationCredentialReadyInitMessage),
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
			// ApplicationCredentialReady being True implies KeystoneAPIReady is True,
			// so when ApplicationCredential is ready, setup is complete
			if instance.Status.Conditions.IsTrue(keystonev1.KeystoneApplicationCredentialReadyCondition) {
				instance.Status.Conditions.MarkTrue(condition.ReadyCondition, keystonev1.KeystoneApplicationCredentialReadyMessage)
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
			"KeystoneAPI not found: %v",
			err,
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
		keystonev1.KeystoneAPIReadyMessage,
	)

	// If KeystoneAPI is being deleted, just drop finalizer
	if !instance.DeletionTimestamp.IsZero() && !keystoneAPI.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(instance, finalizer) {
			base := instance.DeepCopy()
			controllerutil.RemoveFinalizer(instance, finalizer)
			if err := r.Patch(ctx, instance, client.MergeFrom(base)); err != nil {
				if k8s_errors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
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

	// Inspect current Secret status
	if instance.Status.SecretName != "" {
		secret := &corev1.Secret{}
		key := types.NamespacedName{Namespace: instance.Namespace, Name: instance.Status.SecretName}
		if err := r.Get(ctx, key, secret); err != nil {
			if k8s_errors.IsNotFound(err) {
				doRotate = true
				msg = "ApplicationCredential secret missing, rotating"
			} else {
				return ctrl.Result{}, err
			}
		}
	}

	if doRotate {
		logger.Info(msg)

		// Determine if this is a rotation (existing ApplicationCredential) vs initial creation
		isRotation := instance.Status.ACID != ""

		// Build a user-scoped client
		userOS, userRes, userErr := keystonev1.GetUserServiceClient(
			ctx, helperObj, keystoneAPI,
			instance.Spec.UserName,
			instance.Spec.Secret,
			instance.Spec.PasswordSelector,
		)
		if userErr != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				keystonev1.KeystoneApplicationCredentialReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				keystonev1.KeystoneApplicationCredentialReadyErrorMessage,
				userErr.Error(),
			))
			return userRes, userErr
		}
		if userRes != (ctrl.Result{}) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				keystonev1.KeystoneApplicationCredentialReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				"Waiting for secret %q to be available",
				instance.Spec.Secret,
			))
			return userRes, nil
		}

		// Extract user ID from the authenticated token
		userID, err := r.getUserIDFromToken(ctx, userOS.GetOSClient(), instance.Spec.UserName)
		if err != nil {
			logger.Error(err, "Could not get user ID from token")
			instance.Status.Conditions.Set(condition.FalseCondition(
				keystonev1.KeystoneApplicationCredentialReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				keystonev1.KeystoneApplicationCredentialReadyErrorMessage,
				fmt.Sprintf("Failed to get user ID from token: %s", err.Error()),
			))
			return ctrl.Result{}, err
		}
		logger.Info("Using Keystone user", "userName", instance.Spec.UserName, "userID", userID)

		// Create new ApplicationCredential in Keystone
		newACName := fmt.Sprintf("%s-%s", instance.Name, rand.String(5))
		newID, newSecret, expiresAt, err := r.createACWithName(
			ctx, userOS.GetOSClient(), userID, instance, newACName,
		)
		if err != nil {
			logger.Error(err, "Could not create ApplicationCredential in Keystone")
			instance.Status.Conditions.Set(condition.FalseCondition(
				keystonev1.KeystoneApplicationCredentialReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				keystonev1.KeystoneApplicationCredentialReadyErrorMessage,
				fmt.Sprintf("Failed to create ApplicationCredential: %s", err.Error()),
			))
			return ctrl.Result{}, err
		}

		// Store it in a Secret (create or update)
		secretName := fmt.Sprintf("%s-secret", instance.Name)
		if err := r.storeACSecret(ctx, helperObj, instance, secretName, newID, newSecret); err != nil {
			if strings.HasPrefix(err.Error(), "requeue: ") {
				return ctrl.Result{RequeueAfter: 500 * time.Millisecond}, nil
			}
			return ctrl.Result{}, err
		}

		// Capture previous expiration before updating status (for rotation event)
		var previousExpiresAt string
		if isRotation && instance.Status.ExpiresAt != nil {
			previousExpiresAt = instance.Status.ExpiresAt.Format(time.RFC3339)
		}

		// Update status
		instance.Status.ACID = newID
		instance.Status.SecretName = secretName
		instance.Status.CreatedAt = &metav1.Time{Time: time.Now().UTC()}
		instance.Status.ExpiresAt = &metav1.Time{Time: expiresAt}

		// Calculate rotation eligible time (start of grace period window)
		graceDuration := time.Duration(instance.Spec.GracePeriodDays) * 24 * time.Hour
		rotationEligibleAt := expiresAt.Add(-graceDuration)
		instance.Status.RotationEligibleAt = &metav1.Time{Time: rotationEligibleAt}

		instance.Status.Conditions.MarkTrue(keystonev1.KeystoneApplicationCredentialReadyCondition, keystonev1.KeystoneApplicationCredentialReadyMessage)

		// Set LastRotated and emit event if this was a rotation
		if isRotation {
			now := metav1.Now()
			instance.Status.LastRotated = &now

			// Emit event for EDPM visibility
			r.EventRecorder.Event(
				instance,
				corev1.EventTypeNormal,
				"ApplicationCredentialRotated",
				fmt.Sprintf("ApplicationCredential '%s' (user: %s) rotated - EDPM nodes may need credential updates. Previous expiration: %s, New expiration: %s",
					instance.Name, instance.Spec.UserName, previousExpiresAt, expiresAt.Format(time.RFC3339)),
			)

			logger.Info("ApplicationCredential rotated", "serviceName", instance.Spec.UserName)
		} else {
			logger.Info("ApplicationCredential created", "serviceName", instance.Spec.UserName)
		}

		// patch status immediately
		if err := helperObj.PatchInstance(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}

		// requeue to check again before next rotate
		nextCheck := r.computeNextRequeue(instance)
		logger.Info("ApplicationCredential ready", "secret", secretName, "ACID", newID, "expiresAt", expiresAt, "nextCheck", nextCheck)
		return ctrl.Result{RequeueAfter: nextCheck}, nil
	}

	// ApplicationCredential already exists and is valid
	// Update RotationEligibleAt in case GracePeriodDays changed
	if instance.Status.ExpiresAt != nil {
		graceDuration := time.Duration(instance.Spec.GracePeriodDays) * 24 * time.Hour
		rotationEligibleAt := instance.Status.ExpiresAt.Add(-graceDuration)
		instance.Status.RotationEligibleAt = &metav1.Time{Time: rotationEligibleAt}
	}

	instance.Status.Conditions.MarkTrue(keystonev1.KeystoneApplicationCredentialReadyCondition, keystonev1.KeystoneApplicationCredentialReadyMessage)
	nextCheck := r.computeNextRequeue(instance)
	return ctrl.Result{RequeueAfter: nextCheck}, nil
}

func (r *ApplicationCredentialReconciler) reconcileDelete(
	ctx context.Context,
	instance *keystonev1.KeystoneApplicationCredential,
) (ctrl.Result, error) {

	// Before removing our CR finalizer, allow ApplicationCredential Secret to be deleted by removing its protection finalizer
	if instance.Status.SecretName != "" {
		key := types.NamespacedName{Namespace: instance.Namespace, Name: instance.Status.SecretName}
		secret := &corev1.Secret{}
		if err := r.Get(ctx, key, secret); err == nil {
			base := secret.DeepCopy()
			controllerutil.RemoveFinalizer(secret, acSecretFinalizer)
			if err := r.Patch(ctx, secret, client.MergeFrom(base)); err != nil {
				if k8s_errors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, err
			}
		}
	}

	// Remove finalizer from the ApplicationCredential CR
	base := instance.DeepCopy()
	controllerutil.RemoveFinalizer(instance, finalizer)
	if err := r.Patch(ctx, instance, client.MergeFrom(base)); err != nil {
		if k8s_errors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// createACWithName creates a new ApplicationCredential in Keystone
func (r *ApplicationCredentialReconciler) createACWithName(
	ctx context.Context,
	identClient *gophercloud.ServiceClient,
	userID string,
	ac *keystonev1.KeystoneApplicationCredential,
	newACName string,
) (string, string, time.Time, error) {
	logger := r.GetLogger(ctx)
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

	created, err := applicationcredentials.Create(ctx, identClient, userID, createOpts).Extract()
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("failed to create ApplicationCredential for user %s: %w", ac.Spec.UserName, err)
	}

	logger.Info("Created ApplicationCredential in Keystone",
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

	// Add application-credentials label
	labels := map[string]string{
		"application-credentials": "true",
	}

	tmpl := []util.Template{{
		Name:       secretName,
		Namespace:  ac.Namespace,
		Type:       util.TemplateTypeNone,
		CustomData: data,
		Labels:     labels,
	}}
	if err := oko_secret.EnsureSecrets(ctx, helperObj, ac, tmpl, nil); err != nil {
		return err
	}

	// Ensure protection finalizer is present on the Secret
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: ac.Namespace, Name: secretName}
	if err := helperObj.GetClient().Get(ctx, key, secret); err != nil {
		return err
	}
	base := secret.DeepCopy()
	if controllerutil.AddFinalizer(secret, acSecretFinalizer) {
		if err := helperObj.GetClient().Patch(ctx, secret, client.MergeFrom(base)); err != nil {
			if k8s_errors.IsConflict(err) {
				return fmt.Errorf("%w: requeue: patch conflict on %s", err, secretName)
			}
			return err
		}
	}

	return nil
}

// getUserIDFromToken extracts the user ID from the authenticated token
func (r *ApplicationCredentialReconciler) getUserIDFromToken(ctx context.Context, identClient *gophercloud.ServiceClient, username string) (string, error) {

	// Get the authenticated user from the token
	user, err := tokens.Get(ctx, identClient, identClient.TokenID).ExtractUser()
	if err != nil {
		return "", fmt.Errorf("failed to extract user %s from token: %w", username, err)
	}

	if user.ID == "" {
		return "", fmt.Errorf("%w: user ID for user %s", util.ErrNotFound, username)
	}

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

// needsRotation determines if an ApplicationCredential needs rotation.
// It checks if the ApplicationCredential exists and if it's within the grace period before expiration.
// Returns: shouldRotate: true if rotation is needed, false otherwise, logMessage: message to log
func needsRotation(ac *keystonev1.KeystoneApplicationCredential) (bool, string) {
	if ac.Status.ACID == "" {
		return true, "ApplicationCredential does not exist, creating"
	}
	expiry := ac.Status.ExpiresAt
	if expiry != nil && !expiry.IsZero() {
		// compute grace window
		rotateAt := expiry.Add(-time.Duration(ac.Spec.GracePeriodDays) * 24 * time.Hour)
		if time.Now().After(rotateAt) {
			return true, "ApplicationCredential is within grace period, rotating"
		}
	}
	return false, ""
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationCredentialReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keystonev1.KeystoneApplicationCredential{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

// GetLogger returns a logger configured for this controller.
func (r *ApplicationCredentialReconciler) GetLogger(ctx context.Context) logr.Logger {
	return ctrlLog.FromContext(ctx).WithName("ApplicationCredentialReconciler")
}
