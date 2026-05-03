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
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/applicationcredentials"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/keystone-operator/internal/keystone"
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
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const acSecretFinalizer = "openstack.org/ac-secret-protection" // #nosec G101
const finalizer = "openstack.org/applicationcredential"        // #nosec G101

var errACIDMismatch = fmt.Errorf("AC secret already exists with a different ACID")

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
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=delete
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
				return r.reconcileDelete(ctx, instance, helperObj, nil)
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

	// Deletion skips the KeystoneAPI IsReady() check below, so the AC CR is not blocked on readiness
	// reconcileDelete receives keystoneAPI to attempt best-effort Keystone AC revocation
	// If Keystone is unreachable (e.g. full teardown), revocation is skipped and finalizers are still removed so the CR deletion is never blocked
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helperObj, keystoneAPI)
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

	// If the current secret was deleted (e.g. manual cleanup, accidental removal),
	// fall through to rotation so the controller self-heals.
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

		// Create a new immutable Secret with a unique name
		secretName, err := r.createImmutableACSecret(ctx, helperObj, instance, newID, newSecret)
		if err != nil {
			// The Keystone AC was already created above but its secret cannot be stored.
			// Revoke it so it doesn't become a permanently orphaned credential in Keystone.
			if revokeErr := revokeKeystoneAC(ctx, userOS.GetOSClient(), userID, newID); revokeErr != nil {
				logger.Error(revokeErr, "Failed to revoke orphaned Keystone AC after secret creation failure", "ACID", newID)
			} else {
				logger.Info("Revoked orphaned Keystone AC after secret creation failure", "ACID", newID)
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				keystonev1.KeystoneApplicationCredentialReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				keystonev1.KeystoneApplicationCredentialReadyErrorMessage,
				fmt.Sprintf("Failed to create AC secret: %s", err.Error()),
			))
			return ctrl.Result{}, err
		}

		// Capture previous expiration before updating status (for rotation event)
		var previousExpiresAt string
		if isRotation && instance.Status.ExpiresAt != nil {
			previousExpiresAt = instance.Status.ExpiresAt.Format(time.RFC3339)
		}

		// Update status, capture previous secret before rotation for rollback tracking
		if isRotation && instance.Status.SecretName != "" {
			instance.Status.PreviousSecretName = instance.Status.SecretName
		}
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

		// Return early after creation/rotation so the defer patches the status.
		// The status patch triggers a re-reconcile via the For() watch, at which
		// point the cache is fresh and cleanup of unused rotated secrets can run.
		// RequeueAfter is a safety net in case the watch event is delayed.
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	// Migrate old mutable secrets: add the application-credential-service label
	// if missing, so they become visible to label-based cleanup and deletion queries
	serviceName := strings.TrimPrefix(instance.Name, "ac-")
	for _, sn := range []string{instance.Status.SecretName, instance.Status.PreviousSecretName} {
		if sn != "" {
			if err := r.ensureServiceLabel(ctx, helperObj, sn, instance.Namespace, serviceName); err != nil {
				logger.Info("Could not ensure service label on secret", "secret", sn, "error", err)
			}
		}
	}

	// ApplicationCredential already exists and is valid
	// Update RotationEligibleAt in case GracePeriodDays changed
	if instance.Status.ExpiresAt != nil {
		graceDuration := time.Duration(instance.Spec.GracePeriodDays) * 24 * time.Hour
		rotationEligibleAt := instance.Status.ExpiresAt.Add(-graceDuration)
		instance.Status.RotationEligibleAt = &metav1.Time{Time: rotationEligibleAt}
	}

	// Unused rotated AC secrets (not current/previous, no consumer finalizer), best effort
	// Failures are logged but do not block the AC CR from reaching Ready, since the current credentials
	// are valid regardless. Cleanup will be retried on the next reconcile.
	userOS, userRes, userErr := keystonev1.GetUserServiceClient(
		ctx, helperObj, keystoneAPI,
		instance.Spec.UserName,
		instance.Spec.Secret,
		instance.Spec.PasswordSelector,
	)
	if userErr != nil {
		logger.Info("Could not build Keystone client; skipping unused rotated AC secret cleanup", "error", userErr)
	} else if userRes != (ctrl.Result{}) {
		logger.Info("Keystone client not ready; skipping unused rotated AC secret cleanup")
	} else {
		userID, err := r.getUserIDFromToken(ctx, userOS.GetOSClient(), instance.Spec.UserName)
		if err != nil {
			logger.Info("Could not get user ID; skipping unused rotated AC secret cleanup", "error", err)
		} else {
			if err := r.cleanupUnusedRotatedSecrets(ctx, instance, helperObj, userOS.GetOSClient(), userID); err != nil {
				logger.Error(err, "Unused rotated AC secret cleanup failed, will retry on next reconcile")
			}
		}
	}

	instance.Status.Conditions.MarkTrue(keystonev1.KeystoneApplicationCredentialReadyCondition, keystonev1.KeystoneApplicationCredentialReadyMessage)
	return ctrl.Result{}, nil
}

// reconcileDelete runs when the AC CR is deleted (removed from OpenStackControlPlane or manually)
// Best-effort: try to revoke Keystone ACs, then remove all protection finalizers
func (r *ApplicationCredentialReconciler) reconcileDelete(
	ctx context.Context,
	instance *keystonev1.KeystoneApplicationCredential,
	helperObj *helper.Helper,
	keystoneAPI *keystonev1.KeystoneAPI,
) (ctrl.Result, error) {
	logger := r.GetLogger(ctx)
	logger.Info("Reconciling ApplicationCredential delete")

	serviceName := strings.TrimPrefix(instance.Name, "ac-")

	// Migrate old mutable secrets before the label-based list query
	for _, sn := range []string{instance.Status.SecretName, instance.Status.PreviousSecretName} {
		if sn != "" {
			if err := r.ensureServiceLabel(ctx, helperObj, sn, instance.Namespace, serviceName); err != nil {
				logger.Info("Could not ensure service label on secret during delete", "secret", sn, "error", err)
			}
		}
	}

	acLabels := map[string]string{
		"application-credentials":        "true",
		"application-credential-service": serviceName,
	}
	secretList, err := oko_secret.GetSecrets(ctx, helperObj, instance.Namespace, acLabels)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Build a Keystone client for best-effort revocation (nil if unavailable)
	var identClient *gophercloud.ServiceClient
	var userID string
	if keystoneAPI != nil {
		userOS, userRes, userErr := keystonev1.GetUserServiceClient(
			ctx, helperObj, keystoneAPI,
			instance.Spec.UserName,
			instance.Spec.Secret,
			instance.Spec.PasswordSelector,
		)
		if userErr != nil || userRes != (ctrl.Result{}) {
			logger.Info("Could not build Keystone client, skipping revocation during AC CR delete", "error", userErr)
		} else if uid, err := r.getUserIDFromToken(ctx, userOS.GetOSClient(), instance.Spec.UserName); err != nil {
			logger.Info("Could not get user ID, skipping revocation during AC CR delete", "error", err)
		} else {
			identClient = userOS.GetOSClient()
			userID = uid
		}
	} else {
		logger.Info("KeystoneAPI not present, skipping Keystone revocation during AC CR delete")
	}

	// Single pass: revoke ACs in Keystone (best-effort) and strip protection finalizers
	seen := make(map[string]bool)
	processed := make(map[string]bool, len(secretList.Items))
	for i := range secretList.Items {
		s := &secretList.Items[i]
		processed[s.Name] = true

		if identClient != nil {
			acID := string(s.Data[keystonev1.ACIDSecretKey])
			if acID != "" && !seen[acID] {
				seen[acID] = true
				if err := revokeKeystoneAC(ctx, identClient, userID, acID); err != nil {
					logger.Info("Keystone revocation failed during AC CR delete, continuing", "ACID", acID, "error", err)
				} else {
					logger.Info("Revoked AC in Keystone during AC CR delete", "ACID", acID)
				}
			}
		}

		if controllerutil.RemoveFinalizer(s, acSecretFinalizer) {
			logger.Info("Removing protection finalizer from AC secret", "secret", s.Name)
			if err := helperObj.GetClient().Update(ctx, s); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// Fallback: status-referenced secrets may not appear in the label-based
	// list if ensureServiceLabel just added the label and the cache hasn't
	// synced yet. Process them directly to avoid leaving orphaned finalizers
	for _, sn := range []string{instance.Status.SecretName, instance.Status.PreviousSecretName} {
		if sn == "" || processed[sn] {
			continue
		}
		fallbackSecret, _, err := oko_secret.GetSecret(ctx, helperObj, sn, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				continue
			}
			return ctrl.Result{}, err
		}
		if controllerutil.RemoveFinalizer(fallbackSecret, acSecretFinalizer) {
			logger.Info("Removing protection finalizer from status-referenced AC secret", "secret", sn)
			if err := helperObj.GetClient().Update(ctx, fallbackSecret); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	controllerutil.RemoveFinalizer(instance, finalizer)
	return ctrl.Result{}, nil
}

// revokeKeystoneAC deletes one application credential in Keystone
// 404 is ignored (already revoked)
func revokeKeystoneAC(
	ctx context.Context,
	identClient *gophercloud.ServiceClient,
	userID, acID string,
) error {
	res := applicationcredentials.Delete(ctx, identClient, userID, acID)
	if res.Err != nil && !gophercloud.ResponseCodeIs(res.Err, http.StatusNotFound) {
		return fmt.Errorf("failed to revoke application credential %s in Keystone: %w", acID, res.Err)
	}
	return nil
}

// ensureServiceLabel adds the application-credential-service label to a secret if it's missing
// This migrates old mutable secrets, so they become visible to label-based queries used by cleanup and deletion
func (r *ApplicationCredentialReconciler) ensureServiceLabel(
	ctx context.Context,
	helperObj *helper.Helper,
	secretName, namespace, serviceName string,
) error {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: namespace, Name: secretName}
	if err := helperObj.GetClient().Get(ctx, key, secret); err != nil {
		return client.IgnoreNotFound(err)
	}
	if secret.Labels != nil && secret.Labels["application-credential-service"] == serviceName {
		return nil
	}
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	secret.Labels["application-credential-service"] = serviceName
	return helperObj.GetClient().Update(ctx, secret)
}

// hasConsumerFinalizer returns true if the secret has any finalizer matching the AC consumer convention (suffix: -ac-consumer)
//
// Currently the controlplane service operators (barbican, cinder, etc.) place
// finalizers like "openstack.org/barbican-ac-consumer" on the AC secret they
// are actively using
//
// TODO(OSPRH-28176): EDPM services (nova, ceilometer, aodh) that consume AC
// secrets on dataplane nodes must also place an EDPM-scoped consumer finalizer
// (e.g. "openstack.org/edpm-nova-ac-consumer") on the secret. This will
// prevent the keystone-operator from revoking/deleting a secret that is still
// deployed to dataplane nodes that have not yet been updated. The EDPM
// finalizer should only be removed once all nodes across all NodeSets have
// been redeployed with the new credentials. This depends on per-node secret
// rotation tracking: https://github.com/openstack-k8s-operators/openstack-operator/pull/1781
func hasConsumerFinalizer(secret *corev1.Secret) bool {
	for _, f := range secret.Finalizers {
		if strings.HasPrefix(f, "openstack.org/") && strings.HasSuffix(f, "-ac-consumer") {
			return true
		}
	}
	return false
}

// cleanupUnusedRotatedSecrets runs during reconcileNormal (AC CR is not being deleted)
// Finds rotated secrets that are neither current nor previous and have no service consumer finalizer
// For each: revoke its AC in Keystone, remove the protection finalizer, delete the K8s Secret
func (r *ApplicationCredentialReconciler) cleanupUnusedRotatedSecrets(
	ctx context.Context,
	instance *keystonev1.KeystoneApplicationCredential,
	helperObj *helper.Helper,
	identClient *gophercloud.ServiceClient,
	userID string,
) error {
	logger := r.GetLogger(ctx)
	serviceName := strings.TrimPrefix(instance.Name, "ac-")

	secretList, err := oko_secret.GetSecrets(ctx, helperObj, instance.Namespace, map[string]string{
		"application-credentials":        "true",
		"application-credential-service": serviceName,
	})
	if err != nil {
		return err
	}

	for i := range secretList.Items {
		s := &secretList.Items[i]
		if s.Name == instance.Status.SecretName || s.Name == instance.Status.PreviousSecretName || hasConsumerFinalizer(s) {
			continue
		}

		acID := string(s.Data[keystonev1.ACIDSecretKey])
		if acID != "" {
			if err := revokeKeystoneAC(ctx, identClient, userID, acID); err != nil {
				return err
			}
			logger.Info("Revoked AC in Keystone", "ACID", acID, "secret", s.Name)
		}

		if controllerutil.RemoveFinalizer(s, acSecretFinalizer) {
			if err := helperObj.GetClient().Update(ctx, s); err != nil {
				return fmt.Errorf("failed to remove protection finalizer from %s: %w", s.Name, err)
			}
			logger.Info("Removed protection finalizer from AC secret", "secret", s.Name)
		}
		if err := helperObj.GetClient().Delete(ctx, s); err != nil && !k8s_errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete AC secret %s: %w", s.Name, err)
		}
		logger.Info("Deleted unused rotated AC secret", "secret", s.Name)
	}
	return nil
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

// acSecretName returns the unique K8s Secret name for a given service name and
// Keystone AC ID: ac-<service>-<first5ofACID>-secret.
func acSecretName(serviceName, acID string) string {
	idPrefix := acID
	if len(idPrefix) > 5 {
		idPrefix = idPrefix[:5]
	}
	return fmt.Sprintf("ac-%s-%s-secret", serviceName, idPrefix)
}

// createImmutableACSecret creates a new immutable K8s Secret with a unique name
// derived from the AC CR name and the first 5 characters of the Keystone AC ID
// Each rotation produces a distinct secret; old secrets are retained
// The protection finalizer is set atomically during creation to avoid a
// read-after-write cache race that would occur with a separate Get+Update
func (r *ApplicationCredentialReconciler) createImmutableACSecret(
	ctx context.Context,
	helperObj *helper.Helper,
	ac *keystonev1.KeystoneApplicationCredential,
	newID, newSecret string,
) (string, error) {
	logger := r.GetLogger(ctx)

	serviceName := strings.TrimPrefix(ac.Name, "ac-")
	secretName := acSecretName(serviceName, newID)
	immutable := true

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ac.Namespace,
			Labels: map[string]string{
				"application-credentials":        "true",
				"application-credential-service": serviceName,
			},
			Finalizers: []string{acSecretFinalizer},
		},
		Immutable: &immutable,
		Data: map[string][]byte{
			keystonev1.ACIDSecretKey:     []byte(newID),
			keystonev1.ACSecretSecretKey: []byte(newSecret),
		},
	}
	if err := controllerutil.SetControllerReference(ac, secret, helperObj.GetScheme()); err != nil {
		return "", fmt.Errorf("failed to set controller reference on AC secret %s: %w", secretName, err)
	}

	if err := helperObj.GetClient().Create(ctx, secret); err != nil {
		if k8s_errors.IsAlreadyExists(err) {
			// A secret with this name already exists - most likely a previous reconcile
			// created it but crashed before patching the status. Validate that the
			// existing secret's ACID matches the one we intended to store, a mismatch
			// would indicate an ACID prefix collision (two different Keystone AC IDs
			// whose first 5 characters are identical, producing the same secret name).
			existing, _, getErr := oko_secret.GetSecret(ctx, helperObj, secretName, ac.Namespace)
			if getErr != nil {
				return "", fmt.Errorf("AC secret %s already exists but failed to fetch for validation: %w", secretName, getErr)
			}
			existingACID := string(existing.Data[keystonev1.ACIDSecretKey])
			if existingACID != newID {
				return "", fmt.Errorf("%w: secret=%s existingACID=%s expectedACID=%s", errACIDMismatch, secretName, existingACID, newID)
			}
			logger.Info("Immutable AC secret already exists with matching ACID, proceeding", "secret", secretName, "ACID", newID)
			return secretName, nil
		}
		return "", fmt.Errorf("failed to create immutable AC secret %s: %w", secretName, err)
	}
	logger.Info("Created immutable AC secret", "secret", secretName, "ACID", newID)

	return secretName, nil
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
		// Suppress Create events on owned secrets to prevent a race condition:
		// when a reconcile creates a new immutable secret, the Owns() watch would
		// immediately enqueue another reconcile. That second reconcile could read
		// a stale AC CR from the informer cache (status not yet patched) and
		// duplicate the AC+secret creation. Update and Delete events still trigger reconciles.
		Owns(&corev1.Secret{}, builder.WithPredicates(predicate.Funcs{
			CreateFunc: func(_ event.CreateEvent) bool { return false },
		})).
		Complete(r)
}

// GetLogger returns a logger configured for this controller.
func (r *ApplicationCredentialReconciler) GetLogger(ctx context.Context) logr.Logger {
	return ctrlLog.FromContext(ctx).WithName("Controllers").WithName("KeystoneApplicationCredential")
}
