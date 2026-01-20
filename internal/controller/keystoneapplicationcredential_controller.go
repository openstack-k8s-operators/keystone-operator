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
	"time"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/applicationcredentials"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/keystone-operator/internal/keystone"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
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
func (r *ApplicationCredentialReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	logger := r.GetLogger(ctx)

	// Fetch the ApplicationCredential CR
	instance := &keystonev1.KeystoneApplicationCredential{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Setup helper for use in reconciles
	helperObj, err := helper.NewHelper(instance, r.Client, r.Kclient, r.Scheme, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// initialize status
	//
	isNewInstance := instance.Status.Conditions == nil
	if isNewInstance {
		instance.Status.Conditions = condition.Conditions{}
	}

	// Save a copy of the condtions so that we can restore the LastTransitionTime
	// when a condition's state doesn't change.
	savedConditions := instance.Status.Conditions.DeepCopy()

	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() {
		if rec := recover(); rec != nil {
			logger.Info(fmt.Sprintf("Panic during reconcile %v\n", rec))
			panic(rec)
		}

		if instance.DeletionTimestamp.IsZero() {
			if instance.Status.Conditions.AllSubConditionIsTrue() {
				instance.Status.Conditions.MarkTrue(condition.ReadyCondition, condition.ReadyMessage)
			} else {
				instance.Status.Conditions.MarkUnknown(condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage)
				instance.Status.Conditions.Set(instance.Status.Conditions.Mirror(condition.ReadyCondition))
			}
			condition.RestoreLastTransitionTimes(&instance.Status.Conditions, savedConditions)
		}

		if err := helperObj.PatchInstance(ctx, instance); err != nil {
			_err = err
			return
		}
	}()

	//
	// Conditions init
	//
	cl := condition.CreateList(
		condition.UnknownCondition(keystonev1.KeystoneAPIReadyCondition, condition.InitReason, keystonev1.KeystoneAPIReadyInitMessage),
		condition.UnknownCondition(keystonev1.KeystoneApplicationCredentialReadyCondition, condition.InitReason, keystonev1.KeystoneApplicationCredentialReadyInitMessage),
		condition.UnknownCondition(condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage),
	)
	instance.Status.Conditions.Init(&cl)
	instance.Status.ObservedGeneration = instance.Generation

	// If we're not deleting this and the service object doesn't have our finalizer, add it.
	if instance.DeletionTimestamp.IsZero() &&
		(controllerutil.AddFinalizer(instance, finalizer) || isNewInstance) {
		return ctrl.Result{}, nil
	}

	//
	// Validate that keystoneAPI is up
	//
	keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, helperObj, instance.Namespace, nil)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// If this KeystoneApplicationCredential CR is being deleted and it has not created
			// any actual ApplicationCredential, just redirect execution to the "reconcileDelete()"
			// logic to avoid potentially hanging on waiting for a KeystoneAPI to appear
			if !instance.DeletionTimestamp.IsZero() {
				return r.reconcileDelete(ctx, instance, helperObj)
			}

			instance.Status.Conditions.Set(condition.FalseCondition(
				keystonev1.KeystoneAPIReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				keystonev1.KeystoneAPIReadyNotFoundMessage,
			))
			logger.Info("KeystoneAPI not found!")

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

	// If this KeystoneApplicationCredential CR is being deleted, perform delete cleanup without waiting for KeystoneAPI readiness
	// NOTE: We don't talk to KeystoneAPI during delete, so KeystoneAPI readiness/deletion check is not needed here
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helperObj)
	}

	if !keystoneAPI.IsReady() {
		instance.Status.Conditions.Set(condition.FalseCondition(
			keystonev1.KeystoneAPIReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			keystonev1.KeystoneAPIReadyWaitingMessage,
		))
		logger.Info("KeystoneAPI not yet ready!")

		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	instance.Status.Conditions.MarkTrue(
		keystonev1.KeystoneAPIReadyCondition,
		keystonev1.KeystoneAPIReadyMessage,
	)

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
	doRotate, msg, err := needsRotation(instance)
	if err != nil {
		logger.Error(err, "Failed to determine rotation need")
		return ctrl.Result{}, err
	}

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
				keystonev1.KeystoneApplicationCredentialWaitingMessage,
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

		// Update security hash to track security-critical fields
		securityHash, err := keystone.ComputeSecurityHash(instance.Spec)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to compute security hash: %w", err)
		}
		instance.Status.SecurityHash = securityHash

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

		logger.Info("ApplicationCredential ready", "secret", secretName, "ACID", newID, "expiresAt", expiresAt)
		return ctrl.Result{}, nil
	}

	// ApplicationCredential already exists and is valid
	// Update RotationEligibleAt in case GracePeriodDays changed
	if instance.Status.ExpiresAt != nil {
		graceDuration := time.Duration(instance.Spec.GracePeriodDays) * 24 * time.Hour
		rotationEligibleAt := instance.Status.ExpiresAt.Add(-graceDuration)
		instance.Status.RotationEligibleAt = &metav1.Time{Time: rotationEligibleAt}
	}

	instance.Status.Conditions.MarkTrue(keystonev1.KeystoneApplicationCredentialReadyCondition, keystonev1.KeystoneApplicationCredentialReadyMessage)
	return ctrl.Result{}, nil
}

func (r *ApplicationCredentialReconciler) reconcileDelete(
	ctx context.Context,
	instance *keystonev1.KeystoneApplicationCredential,
	helperObj *helper.Helper,
) (ctrl.Result, error) {
	logger := r.GetLogger(ctx)
	logger.Info("Reconciling ApplicationCredential delete")

	// NOTE: We intentionally do NOT delete the ApplicationCredential from Keystone.
	// This prevents breaking services (especially EDPM nodes) that are actively using these credentials.
	// The AC will expire naturally based on its expiration time. If immediate cleanup is needed,
	// operators can manually delete the AC from Keystone using: openstack application credential delete <ac-id>

	// Before removing our CR finalizer, allow ApplicationCredential Secret to be deleted by removing its protection finalizer
	if instance.Status.SecretName != "" {
		key := types.NamespacedName{Namespace: instance.Namespace, Name: instance.Status.SecretName}
		secret := &corev1.Secret{}
		if err := helperObj.GetClient().Get(ctx, key, secret); err == nil {
			if controllerutil.RemoveFinalizer(secret, acSecretFinalizer) {
				if err := helperObj.GetClient().Update(ctx, secret); err != nil {
					return ctrl.Result{}, err
				}
			}
		} else if !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Remove finalizer from the ApplicationCredential CR, patching is done in the defer function
	controllerutil.RemoveFinalizer(instance, finalizer)

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
		rule := applicationcredentials.AccessRule{
			Service: ar.Service,
			Path:    ar.Path,
			Method:  ar.Method,
		}
		rules = append(rules, rule)
	}

	createOpts := applicationcredentials.CreateOpts{
		Name:         newACName,
		Description:  fmt.Sprintf("Created by keystone-operator for AC CR %s/%s (user: %s)", ac.Namespace, ac.Name, ac.Spec.UserName),
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
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ac.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, helperObj.GetClient(), secret, func() error {
		secret.Labels = map[string]string{
			"application-credentials": "true",
		}
		secret.Data = map[string][]byte{
			keystonev1.ACIDSecretKey:     []byte(newID),
			keystonev1.ACSecretSecretKey: []byte(newSecret),
		}
		// Add protection finalizer
		controllerutil.AddFinalizer(secret, acSecretFinalizer)
		// Set owner reference
		return controllerutil.SetControllerReference(ac, secret, helperObj.GetScheme())
	})
	if err != nil {
		return err
	}

	if op != controllerutil.OperationResultNone {
		r.GetLogger(ctx).Info("Secret operation completed", "secret", secretName, "operation", op)
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

// needsRotation determines if an ApplicationCredential needs rotation.
// It checks if the ApplicationCredential exists, if security-critical fields changed,
// and if it's within the grace period before expiration.
func needsRotation(ac *keystonev1.KeystoneApplicationCredential) (bool, string, error) {
	if ac.Status.ACID == "" {
		return true, "ApplicationCredential does not exist, creating", nil
	}

	// Check if security-critical fields (roles, accessRules, unrestricted) changed
	currentSecurityHash, err := keystone.ComputeSecurityHash(ac.Spec)
	if err != nil {
		return false, "", fmt.Errorf("failed to compute security hash: %w", err)
	}

	if ac.Status.SecurityHash != "" && currentSecurityHash != ac.Status.SecurityHash {
		return true, "Security fields changed, rotating", nil
	}

	expiry := ac.Status.ExpiresAt
	if expiry != nil && !expiry.IsZero() {
		// compute grace window
		rotateAt := expiry.Add(-time.Duration(ac.Spec.GracePeriodDays) * 24 * time.Hour)
		if time.Now().After(rotateAt) {
			return true, "ApplicationCredential is within grace period, rotating", nil
		}
	}
	return false, "", nil
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
	return ctrlLog.FromContext(ctx).WithName("Controllers").WithName("KeystoneApplicationCredential")
}
