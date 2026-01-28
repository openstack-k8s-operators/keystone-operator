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

// Package controller implements the keystone-operator Kubernetes controllers.
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"time"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	keystone "github.com/openstack-k8s-operators/keystone-operator/internal/keystone"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	configmap "github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	cronjob "github.com/openstack-k8s-operators/lib-common/modules/common/cronjob"
	deployment "github.com/openstack-k8s-operators/lib-common/modules/common/deployment"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	job "github.com/openstack-k8s-operators/lib-common/modules/common/job"
	labels "github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	nad "github.com/openstack-k8s-operators/lib-common/modules/common/networkattachment"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	oko_secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"

	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/utils/ptr"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// GetClient -
func (r *KeystoneAPIReconciler) GetClient() client.Client {
	return r.Client
}

// GetKClient -
func (r *KeystoneAPIReconciler) GetKClient() kubernetes.Interface {
	return r.Kclient
}

// GetScheme -
func (r *KeystoneAPIReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// GetLogger returns a logger object with a logging prefix of "controller.name" and additional controller context fields
func (r *KeystoneAPIReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("KeystoneAPI")
}

// KeystoneAPIReconciler reconciles a KeystoneAPI object
type KeystoneAPIReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Scheme  *runtime.Scheme
}

// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=transporturls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=topology.openstack.org,resources=topologies,verbs=get;list;watch;update

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch
// keystone service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch

// Reconcile reconcile keystone API requests
func (r *KeystoneAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)
	// Fetch the KeystoneAPI instance
	instance := &keystonev1.KeystoneAPI{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
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
		Log,
	)
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
		// Don't update the status, if Reconciler Panics
		if r := recover(); r != nil {
			Log.Info(fmt.Sprintf("Panic during reconcile %v\n", r))
			panic(r)
		}
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
		condition.RestoreLastTransitionTimes(&instance.Status.Conditions, savedConditions)
		err := helper.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	//
	// Conditions init
	//
	// Always needed conditions
	cl := condition.CreateList(
		condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
		condition.UnknownCondition(condition.TLSInputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
	)

	// Only for internal Keystone API
	if !instance.Spec.ExternalKeystoneAPI {
		cl.Set(condition.UnknownCondition(condition.DBReadyCondition, condition.InitReason, condition.DBReadyInitMessage))
		cl.Set(condition.UnknownCondition(condition.DBSyncReadyCondition, condition.InitReason, condition.DBSyncReadyInitMessage))
		cl.Set(condition.UnknownCondition(condition.RabbitMqTransportURLReadyCondition, condition.InitReason, condition.RabbitMqTransportURLReadyInitMessage))
		cl.Set(condition.UnknownCondition(condition.MemcachedReadyCondition, condition.InitReason, condition.MemcachedReadyInitMessage))
		cl.Set(condition.UnknownCondition(condition.CreateServiceReadyCondition, condition.InitReason, condition.CreateServiceReadyInitMessage))
		cl.Set(condition.UnknownCondition(condition.BootstrapReadyCondition, condition.InitReason, condition.BootstrapReadyInitMessage))
		cl.Set(condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage))
		cl.Set(condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage))
		cl.Set(condition.UnknownCondition(condition.NetworkAttachmentsReadyCondition, condition.InitReason, condition.NetworkAttachmentsReadyInitMessage))
		cl.Set(condition.UnknownCondition(condition.CronJobReadyCondition, condition.InitReason, condition.CronJobReadyInitMessage))
		// service account, role, rolebinding conditions
		cl.Set(condition.UnknownCondition(condition.ServiceAccountReadyCondition, condition.InitReason, condition.ServiceAccountReadyInitMessage))
		cl.Set(condition.UnknownCondition(condition.RoleReadyCondition, condition.InitReason, condition.RoleReadyInitMessage))
		cl.Set(condition.UnknownCondition(condition.RoleBindingReadyCondition, condition.InitReason, condition.RoleBindingReadyInitMessage))
	}

	// Init Topology condition if there's a reference
	if instance.Spec.TopologyRef != nil {
		cl.Set(condition.UnknownCondition(condition.TopologyReadyCondition, condition.InitReason, condition.TopologyReadyInitMessage))
	}

	instance.Status.Conditions.Init(&cl)
	instance.Status.ObservedGeneration = instance.Generation

	// If we're not deleting this and the service object doesn't have our finalizer, add it.
	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, helper.GetFinalizer()) || isNewInstance {
		return ctrl.Result{}, nil
	}

	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.APIEndpoints == nil {
		instance.Status.APIEndpoints = map[string]string{}
	}
	if instance.Status.NetworkAttachments == nil {
		instance.Status.NetworkAttachments = map[string][]string{}
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	// Check if external Keystone API is configured
	if instance.Spec.ExternalKeystoneAPI {
		return r.reconcileExternalKeystoneAPI(ctx, instance, helper)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper)
}

// fields to index to reconcile when change
const (
	passwordSecretField                 = ".spec.secret"
	caBundleSecretNameField             = ".spec.tls.caBundleSecretName" // #nosec G101
	tlsAPIInternalField                 = ".spec.tls.api.internal.secretName"
	tlsAPIPublicField                   = ".spec.tls.api.public.secretName"
	topologyField                       = ".spec.topologyRef.Name"
	httpdCustomServiceConfigSecretField = ".spec.httpdCustomization.customServiceConfigSecret" // #nosec G101
	federatedRealmConfigField           = ".spec.federatedRealmConfig"                         // #nosec G101
)

var allWatchFields = []string{
	passwordSecretField,
	caBundleSecretNameField,
	tlsAPIInternalField,
	tlsAPIPublicField,
	httpdCustomServiceConfigSecretField,
	federatedRealmConfigField,
	topologyField,
}

// SetupWithManager -
func (r *KeystoneAPIReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	Log := r.GetLogger(ctx)

	// index passwordSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &keystonev1.KeystoneAPI{}, passwordSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*keystonev1.KeystoneAPI)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.Secret}
	}); err != nil {
		return err
	}

	// index caBundleSecretNameField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &keystonev1.KeystoneAPI{}, caBundleSecretNameField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*keystonev1.KeystoneAPI)
		if cr.Spec.TLS.CaBundleSecretName == "" {
			return nil
		}
		return []string{cr.Spec.TLS.CaBundleSecretName}
	}); err != nil {
		return err
	}

	// index tlsAPIInternalField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &keystonev1.KeystoneAPI{}, tlsAPIInternalField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*keystonev1.KeystoneAPI)
		if cr.Spec.TLS.API.Internal.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.API.Internal.SecretName}
	}); err != nil {
		return err
	}

	// index tlsAPIPublicField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &keystonev1.KeystoneAPI{}, tlsAPIPublicField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*keystonev1.KeystoneAPI)
		if cr.Spec.TLS.API.Public.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.API.Public.SecretName}
	}); err != nil {
		return err
	}

	// index httpdOverrideSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &keystonev1.KeystoneAPI{}, httpdCustomServiceConfigSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*keystonev1.KeystoneAPI)
		if cr.Spec.HttpdCustomization.CustomConfigSecret == nil {
			return nil
		}
		return []string{*cr.Spec.HttpdCustomization.CustomConfigSecret}
	}); err != nil {
		return err
	}

	// index federatedRealmConfigField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &keystonev1.KeystoneAPI{}, federatedRealmConfigField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*keystonev1.KeystoneAPI)
		if cr.Spec.FederatedRealmConfig == "" {
			return nil
		}
		return []string{cr.Spec.FederatedRealmConfig}
	}); err != nil {
		return err
	}

	// index topologyField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &keystonev1.KeystoneAPI{}, topologyField, func(rawObj client.Object) []string {
		// Extract the topology name from the spec, if one is provided
		cr := rawObj.(*keystonev1.KeystoneAPI)
		if cr.Spec.TopologyRef == nil {
			return nil
		}
		return []string{cr.Spec.TopologyRef.Name}
	}); err != nil {
		return err
	}

	memcachedFn := func(ctx context.Context, o client.Object) []reconcile.Request {
		result := []reconcile.Request{}

		// get all KeystoneAPI CRs
		keystoneAPIs := &keystonev1.KeystoneAPIList{}
		listOpts := []client.ListOption{
			client.InNamespace(o.GetNamespace()),
		}
		if err := r.List(ctx, keystoneAPIs, listOpts...); err != nil {
			Log.Error(err, "Unable to retrieve KeystoneAPI CRs %w")
			return nil
		}

		for _, cr := range keystoneAPIs.Items {
			if o.GetName() == cr.Spec.MemcachedInstance {
				name := client.ObjectKey{
					Namespace: o.GetNamespace(),
					Name:      cr.Name,
				}
				Log.Info(fmt.Sprintf("Memcached %s is used by KeystoneAPI CR %s", o.GetName(), cr.Name))
				result = append(result, reconcile.Request{NamespacedName: name})
			}
		}
		if len(result) > 0 {
			return result
		}
		return nil
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&keystonev1.KeystoneAPI{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&mariadbv1.MariaDBAccount{}).
		Owns(&batchv1.Job{}).
		Owns(&batchv1.CronJob{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&rabbitmqv1.TransportURL{}).
		Watches(&memcachedv1.Memcached{},
			handler.EnqueueRequestsFromMapFunc(memcachedFn)).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&topologyv1.Topology{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

func (r *KeystoneAPIReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(context.Background())

	for _, field := range allWatchFields {
		crList := &keystonev1.KeystoneAPIList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := r.List(ctx, crList, listOps)
		if err != nil {
			Log.Error(err, fmt.Sprintf("listing %s for field: %s - %s", crList.GroupVersionKind().Kind, field, src.GetNamespace()))
			return requests
		}

		for _, item := range crList.Items {
			Log.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

			requests = append(requests,
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
				},
			)
		}
	}

	return requests
}

func (r *KeystoneAPIReconciler) reconcileDelete(ctx context.Context, instance *keystonev1.KeystoneAPI, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service delete")

	// If using external Keystone API, we don't have any resources to clean up
	if instance.Spec.ExternalKeystoneAPI {
		// Just remove the finalizer
		controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
		Log.Info("Reconciled External Keystone API delete successfully")
		return ctrl.Result{}, nil
	}

	// We need to allow all KeystoneEndpoint and KeystoneService processing to finish
	// in the case of a delete before we remove the finalizers.  For instance, in the
	// case of the Memcached dependency, if Memcached is deleted before all Keystone
	// cleanup has finished, then the Keystone logic will likely hit a 500 error and
	// thus its deletion will hang indefinitely.
	for _, finalizer := range instance.Finalizers {
		// If this finalizer is not our KeystoneAPI finalizer, then it is either
		// a KeystoneService or KeystoneEndpointer finalizer, which indicates that
		// there is more Keystone processing that needs to finish before we can
		// allow our DB and Memcached dependencies to be potentially deleted
		// themselves
		if finalizer != helper.GetFinalizer() {
			return ctrl.Result{}, nil
		}
	}

	// Remove finalizer on the Topology CR
	if ctrlResult, err := topologyv1.EnsureDeletedTopologyRef(
		ctx,
		helper,
		instance.Status.LastAppliedTopology,
		instance.Name,
	); err != nil {
		return ctrlResult, err
	}

	// Remove our finalizer from Memcached
	memcached, err := memcachedv1.GetMemcachedByName(ctx, helper, instance.Spec.MemcachedInstance, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if !k8s_errors.IsNotFound(err) && memcached != nil {
		if controllerutil.RemoveFinalizer(memcached, helper.GetFinalizer()) {
			err := r.Update(ctx, memcached)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// remove db finalizer before the keystone one
	db, err := mariadbv1.GetDatabaseByNameAndAccount(ctx, helper, keystone.DatabaseCRName, instance.Spec.DatabaseAccount, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if !k8s_errors.IsNotFound(err) {
		if err := db.DeleteFinalizer(ctx, helper); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info("Reconciled Service delete successfully")

	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileInit(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	helper *helper.Helper,
	serviceLabels map[string]string,
	serviceAnnotations map[string]string,
	memcached *memcachedv1.Memcached,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service init")
	//
	// Service account, role, binding
	//
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups:     []string{"security.openshift.io"},
			ResourceNames: []string{"anyuid"},
			Resources:     []string{"securitycontextconstraints"},
			Verbs:         []string{"use"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"create", "get", "list", "watch", "update", "patch", "delete"},
		},
	}
	rbacResult, err := common_rbac.ReconcileRbac(ctx, helper, instance, rbacRules)
	if err != nil {
		return rbacResult, err
	} else if (rbacResult != ctrl.Result{}) {
		return rbacResult, nil
	}

	//
	// run keystone db sync
	//
	dbSyncHash := instance.Status.Hash[keystonev1.DbSyncHash]
	jobDef := keystone.DbSyncJob(instance, serviceLabels, serviceAnnotations)
	dbSyncjob := job.NewJob(
		jobDef,
		keystonev1.DbSyncHash,
		instance.Spec.PreserveJobs,
		5*time.Second,
		dbSyncHash,
	)
	ctrlResult, err := dbSyncjob.DoJob(
		ctx,
		helper,
	)
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBSyncReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBSyncReadyRunningMessage))
		return ctrlResult, nil
	}
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBSyncReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBSyncReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if dbSyncjob.HasChanged() {
		instance.Status.Hash[keystonev1.DbSyncHash] = dbSyncjob.GetHash()
		Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.Hash[keystonev1.DbSyncHash]))
	}
	instance.Status.Conditions.MarkTrue(condition.DBSyncReadyCondition, condition.DBSyncReadyMessage)

	// run keystone db sync - end

	//
	// create service/s
	//
	keystoneEndpoints := map[service.Endpoint]endpoint.Data{
		service.EndpointPublic: {
			Port: keystone.KeystonePublicPort,
		},
		service.EndpointInternal: {
			Port: keystone.KeystoneInternalPort,
		},
	}

	apiEndpoints := make(map[string]string)
	for endpointType, data := range keystoneEndpoints {
		endpointTypeStr := string(endpointType)
		endpointName := instance.Name + "-" + endpointTypeStr

		svcOverride := instance.Spec.Override.Service[endpointType]
		if svcOverride.EmbeddedLabelsAnnotations == nil {
			svcOverride.EmbeddedLabelsAnnotations = &service.EmbeddedLabelsAnnotations{}
		}

		exportLabels := util.MergeStringMaps(
			serviceLabels,
			map[string]string{
				service.AnnotationEndpointKey: endpointTypeStr,
			},
		)

		// Create the service
		svc, err := service.NewService(
			service.GenericService(&service.GenericServiceDetails{
				Name:      endpointName,
				Namespace: instance.Namespace,
				Labels:    exportLabels,
				Selector:  serviceLabels,
				Port: service.GenericServicePort{
					Name:     endpointName,
					Port:     data.Port,
					Protocol: corev1.ProtocolTCP,
				},
			}),
			5,
			&svcOverride.OverrideSpec,
		)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.CreateServiceReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.CreateServiceReadyErrorMessage,
				err.Error()))

			return ctrl.Result{}, err
		}

		svc.AddAnnotation(map[string]string{
			service.AnnotationEndpointKey: endpointTypeStr,
		})

		// add Annotation to whether creating an ingress is required or not
		if endpointType == service.EndpointPublic && svc.GetServiceType() == corev1.ServiceTypeClusterIP {
			svc.AddAnnotation(map[string]string{
				service.AnnotationIngressCreateKey: "true",
			})
		} else {
			svc.AddAnnotation(map[string]string{
				service.AnnotationIngressCreateKey: "false",
			})
			if svc.GetServiceType() == corev1.ServiceTypeLoadBalancer {
				svc.AddAnnotation(map[string]string{
					service.AnnotationHostnameKey: svc.GetServiceHostname(), // add annotation to register service name in dnsmasq
				})
			}
		}

		ctrlResult, err := svc.CreateOrPatch(ctx, helper)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.CreateServiceReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.CreateServiceReadyErrorMessage,
				err.Error()))

			return ctrlResult, err
		} else if (ctrlResult != ctrl.Result{}) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.CreateServiceReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.CreateServiceReadyRunningMessage))
			return ctrlResult, nil
		}
		// create service - end

		// if TLS is enabled
		if instance.Spec.TLS.API.Enabled(endpointType) {
			// set endpoint protocol to https
			data.Protocol = ptr.To(service.ProtocolHTTPS)
		}

		apiEndpoints[string(endpointType)], err = svc.GetAPIEndpoint(
			svcOverride.EndpointURL, data.Protocol, data.Path)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	instance.Status.Conditions.MarkTrue(condition.CreateServiceReadyCondition, condition.CreateServiceReadyMessage)

	//
	// Update instance status with service endpoint url from route host information
	//
	instance.Status.APIEndpoints = apiEndpoints

	// expose service - end

	//
	// BootStrap Job
	//
	jobDef = keystone.BootstrapJob(instance, serviceLabels, serviceAnnotations, instance.Status.APIEndpoints, memcached)
	bootstrapjob := job.NewJob(
		jobDef,
		keystonev1.BootstrapHash,
		instance.Spec.PreserveJobs,
		5*time.Second,
		instance.Status.Hash[keystonev1.BootstrapHash],
	)
	ctrlResult, err = bootstrapjob.DoJob(
		ctx,
		helper,
	)
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.BootstrapReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.BootstrapReadyRunningMessage))
		return ctrlResult, nil
	}
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.BootstrapReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.BootstrapReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if bootstrapjob.HasChanged() {
		instance.Status.Hash[keystonev1.BootstrapHash] = bootstrapjob.GetHash()
		Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.Hash[keystonev1.BootstrapHash]))
	}
	instance.Status.Conditions.MarkTrue(condition.BootstrapReadyCondition, condition.BootstrapReadyMessage)

	// run keystone bootstrap - end

	Log.Info("Reconciled Service init successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileExternalKeystoneAPI(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	helper *helper.Helper,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling External Keystone API")

	// When using external Keystone API, we skip all deployment logic
	// and just use the endpoints from the override spec

	configMapVars := make(map[string]env.Setter)

	// Verify secret is available (needed for admin client operations)
	ctrlResult, err := r.verifySecret(ctx, instance, helper, configMapVars)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Verify override spec is valid - both public and internal endpoints must be defined
	if len(instance.Spec.Override.Service) == 0 {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyWaitingMessage))
		return ctrl.Result{}, nil
	}

	// Verify both public and internal endpoints are defined with valid EndpointURL
	hasPublic := false
	hasInternal := false
	publicEndpointURL := ""
	internalEndpointURL := ""

	for endpointType, data := range instance.Spec.Override.Service {
		if endpointType == service.EndpointPublic {
			hasPublic = true
			if data.EndpointURL == nil || *data.EndpointURL == "" {
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.InputReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					"external Keystone API requires endpointURL to be set for public endpoint"))
				return ctrl.Result{}, nil
			}
			publicEndpointURL = *data.EndpointURL
		}
		if endpointType == service.EndpointInternal {
			hasInternal = true
			if data.EndpointURL == nil || *data.EndpointURL == "" {
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.InputReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					"external Keystone API requires endpointURL to be set for internal endpoint"))
				return ctrl.Result{}, nil
			}
			internalEndpointURL = *data.EndpointURL
		}
	}

	if !hasPublic {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			"external Keystone API requires public endpoint to be defined"))
		return ctrl.Result{}, nil
	}

	if !hasInternal {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			"external Keystone API requires internal endpoint to be defined"))
		return ctrl.Result{}, nil
	}

	// Set API endpoints from externalKeystoneAPI spec
	if instance.Status.APIEndpoints == nil {
		instance.Status.APIEndpoints = map[string]string{}
	}
	instance.Status.APIEndpoints[string(service.EndpointPublic)] = publicEndpointURL
	instance.Status.APIEndpoints[string(service.EndpointInternal)] = internalEndpointURL

	// Copy region from spec to status
	instance.Status.Region = instance.Spec.Region

	// Set InputReadyCondition after both secret and endpoints are verified
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	// Set ready count to 0 since we're not deploying anything
	instance.Status.ReadyCount = 0

	// Verify TLS input (CA cert secret if provided)
	err = r.verifyTLSInput(ctx, instance, helper, configMapVars)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Don't return NotFound error to the caller
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(condition.TLSInputReadyCondition, condition.InputReadyMessage)

	//
	// create OpenStackClient config
	//
	err = r.reconcileCloudConfig(ctx, helper, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	Log.Info("Reconciled External Keystone API successfully")
	return ctrl.Result{}, nil
}

// verifySecret verifies the OpenStack secret exists and adds its hash to configMapVars
func (r *KeystoneAPIReconciler) verifySecret(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	helper *helper.Helper,
	configMapVars map[string]env.Setter,
) (ctrl.Result, error) {
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	// NOTE: VerifySecret handles the "not found" error and returns RequeueAfter ctrl.Result if so, so we don't
	//       need to check the error type here
	hash, result, err := oko_secret.VerifySecret(ctx, types.NamespacedName{Name: instance.Spec.Secret, Namespace: instance.Namespace}, []string{"AdminPassword"}, helper.GetClient(), time.Second*10)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	} else if (result != ctrl.Result{}) {
		// This case is "secret not found".  VerifySecret already logs a message for it.
		// We treat this as a warning because it means that the service will not be able to start
		// while we are waiting for the secret to be created manually by the user.
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyWaitingMessage))
		return result, nil
	}

	// Add hash to configMapVars if provided
	if configMapVars != nil {
		configMapVars[instance.Spec.Secret] = env.SetValue(hash)
	}

	return ctrl.Result{}, nil
}
func (r *KeystoneAPIReconciler) verifyTLSInput(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	helper *helper.Helper,
	configMapVars map[string]env.Setter,
) error {
	// Validate the CA cert secret if provided
	if instance.Spec.TLS.CaBundleSecretName != "" {
		hash, err := tls.ValidateCACertSecret(
			ctx,
			helper.GetClient(),
			types.NamespacedName{
				Name:      instance.Spec.TLS.CaBundleSecretName,
				Namespace: instance.Namespace,
			},
		)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				// Since the CA cert secret should have been manually created by the user and provided in the spec,
				// we treat this as a warning because it means that the service will not be able to start.
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.TLSInputReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					condition.TLSInputReadyWaitingMessage,
					instance.Spec.TLS.CaBundleSecretName))
			} else {
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.TLSInputReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					condition.TLSInputErrorMessage,
					err.Error()))
			}
			return err
		}

		if hash != "" {
			if configMapVars != nil {
				configMapVars[tls.CABundleKey] = env.SetValue(hash)
			}
		}
	}

	return nil
}
func (r *KeystoneAPIReconciler) reconcileUpdate(ctx context.Context) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	Log.Info("Reconciled Service update successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileUpgrade(ctx context.Context) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service upgrade")

	// TODO: should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	Log.Info("Reconciled Service upgrade successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileNormal(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	helper *helper.Helper,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service")

	serviceLabels := map[string]string{
		common.AppSelector:   keystone.ServiceName,
		common.OwnerSelector: instance.Name,
	}

	// ConfigMap
	configMapVars := make(map[string]env.Setter)

	//
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	//
	ctrlResult, err := r.verifySecret(ctx, instance, helper, configMapVars)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	// run check OpenStack secret - end

	//
	// create service DB instance
	//
	db, result, err := r.ensureDB(ctx, helper, instance)
	if err != nil {
		return ctrl.Result{}, err
	} else if (result != ctrl.Result{}) {
		return result, nil
	}
	// create service DB - end

	//
	// create RabbitMQ transportURL CR and get the actual URL from the associated secret that is created
	//
	transportURL, op, err := r.transportURLCreateOrUpdate(ctx, instance, serviceLabels)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.RabbitMqTransportURLReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.RabbitMqTransportURLReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("TransportURL %s successfully reconciled - operation: %s", transportURL.Name, string(op)))
	}

	instance.Status.TransportURLSecret = transportURL.Status.SecretName

	if instance.Status.TransportURLSecret == "" {
		// Since the TransportURL secret is automatically created by the Infra operator,
		// we treat this as an info (because the user is not responsible for manually creating it).
		Log.Info(fmt.Sprintf("Waiting for TransportURL %s secret to be created", transportURL.Name))
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.RabbitMqTransportURLReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.RabbitMqTransportURLReadyRunningMessage))
		return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
	}
	Log.Info(fmt.Sprintf("TransportURL secret name %s", transportURL.Status.SecretName))
	instance.Status.Conditions.MarkTrue(condition.RabbitMqTransportURLReadyCondition, condition.RabbitMqTransportURLReadyMessage)
	// run check rabbitmq - end

	//
	// Check for required memcached used for caching
	//
	memcached, err := memcachedv1.GetMemcachedByName(ctx, helper, instance.Spec.MemcachedInstance, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Memcached should be automatically created by the encompassing OpenStackControlPlane,
			// but we don't propagate its name into the "memcachedInstance" field of other sub-resources,
			// so if it is missing at this point, it *could* be because there's a mismatch between the
			// name of the Memcached CR and the name of the Memcached instance referenced by this CR.
			// Since that situation would block further reconciliation, we treat it as a warning.
			Log.Info(fmt.Sprintf("memcached %s not found", instance.Spec.MemcachedInstance))
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.MemcachedReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.MemcachedReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.MemcachedReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.MemcachedReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	// Add finalizer to Memcached to prevent it from being deleted now that we're using it
	if controllerutil.AddFinalizer(memcached, helper.GetFinalizer()) {
		err := r.Update(ctx, memcached)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.MemcachedReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.MemcachedReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}
	}

	if !memcached.IsReady() {
		Log.Info(fmt.Sprintf("memcached %s is not ready", memcached.Name))
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.MemcachedReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.MemcachedReadyWaitingMessage))
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	// Mark the Memcached Service as Ready if we get to this point with no errors
	instance.Status.Conditions.MarkTrue(
		condition.MemcachedReadyCondition, condition.MemcachedReadyMessage)
	// run check memcached - end

	//
	// Create ConfigMaps and Secrets required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create Configmap required for keystone input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal keystone config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the OpenStack secret via the init container
	//
	err = r.generateServiceConfigMaps(ctx, instance, helper, &configMapVars, memcached, db)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	//
	// Create secret holding fernet keys (for token and credential)
	//
	err = r.ensureFernetKeys(ctx, instance, helper, &configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	//
	// Create secret holding federation realm config (for multiple realms)
	//

	federationFilenames, err := r.ensureFederationRealmConfig(ctx, instance, helper, &configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	//
	// TLS input validation
	//
	// Verify TLS input (CA cert secret if provided)
	err = r.verifyTLSInput(ctx, instance, helper, configMapVars)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Don't return NotFound error to the caller
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Validate API service certs secrets
	certsHash, err := instance.Spec.TLS.API.ValidateCertSecrets(ctx, helper, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Since the OpenStackControlPlane creates the API service certs secrets,
			// we treat this as an info (because the user is not responsible for manually creating them).
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.TLSInputReadyWaitingMessage,
				err.Error()))
			return ctrl.Result{}, nil
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.TLSInputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TLSInputErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	configMapVars[tls.TLSHashName] = env.SetValue(certsHash)

	// all cert input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.TLSInputReadyCondition, condition.InputReadyMessage)

	// create hash over all the different input resources to identify if any those changed
	// and a restart/recreate is required.
	inputHash, hashChanged, err := r.createHashOfInputHashes(ctx, instance, configMapVars)
	if err != nil {
		return ctrl.Result{}, err
	} else if hashChanged {
		// Hash changed and instance status should be updated (which will be done by main defer func),
		// so we need to return and reconcile again
		return ctrl.Result{}, nil
	}
	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	// Create ConfigMaps and Secrets - end

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	// networks to attach to
	nadList := []networkv1.NetworkAttachmentDefinition{}
	for _, netAtt := range instance.Spec.NetworkAttachments {
		nad, err := nad.GetNADWithName(ctx, helper, netAtt, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				// Since the net-attach-def CR should have been manually created by the user and referenced in the spec,
				// we treat this as a warning because it means that the service will not be able to start.
				Log.Info(fmt.Sprintf("network-attachment-definition %s not found", netAtt))
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.NetworkAttachmentsReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					condition.NetworkAttachmentsReadyWaitingMessage,
					netAtt))
				return ctrl.Result{RequeueAfter: time.Second * 10}, nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.NetworkAttachmentsReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.NetworkAttachmentsReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}

		if nad != nil {
			nadList = append(nadList, *nad)
		}
	}

	serviceAnnotations, err := nad.EnsureNetworksAnnotation(nadList)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed create network annotation from %s: %w",
			instance.Spec.NetworkAttachments, err)
	}

	// Handle service init
	ctrlResult, err = r.reconcileInit(ctx, instance, helper, serviceLabels, serviceAnnotations, memcached)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service update
	ctrlResult, err = r.reconcileUpdate(ctx)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// Handle Topology
	//
	// Build a defaultLabelSelector
	topology, err := topologyv1.EnsureServiceTopology(
		ctx,
		helper,
		instance.Spec.TopologyRef,
		instance.Status.LastAppliedTopology,
		instance.Name,
		labels.GetLabelSelector(serviceLabels),
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.TopologyReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TopologyReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, fmt.Errorf("waiting for Topology requirements: %w", err)
	}

	// If TopologyRef is present and ensureKeystoneAPITopology returned a valid
	// topology object, set .Status.LastAppliedTopology to the referenced one
	// and mark the condition as true
	if instance.Spec.TopologyRef != nil {
		// update the Status with the last retrieved Topology name
		instance.Status.LastAppliedTopology = instance.Spec.TopologyRef
		// update the TopologyRef associated condition
		instance.Status.Conditions.MarkTrue(condition.TopologyReadyCondition, condition.TopologyReadyMessage)
	} else {
		// remove LastAppliedTopology from the .Status
		instance.Status.LastAppliedTopology = nil
	}

	//
	// normal reconcile tasks
	//

	// Define a new Deployment object
	deplDef, err := keystone.Deployment(instance, inputHash, serviceLabels, serviceAnnotations, topology, federationFilenames, memcached)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	depl := deployment.NewDeployment(
		deplDef,
		5*time.Second,
	)

	ctrlResult, err = depl.CreateOrPatch(ctx, helper)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		return ctrlResult, nil
	}

	deploy := depl.GetDeployment()
	if deploy.Generation == deploy.Status.ObservedGeneration {
		instance.Status.ReadyCount = deploy.Status.ReadyReplicas
		instance.Status.Region = instance.Spec.Region
	}

	// verify if network attachment matches expectations
	networkReady, networkAttachmentStatus, err := nad.VerifyNetworkStatusFromAnnotation(
		ctx, helper, instance.Spec.NetworkAttachments, serviceLabels, instance.Status.ReadyCount,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	instance.Status.NetworkAttachments = networkAttachmentStatus
	if networkReady {
		instance.Status.Conditions.MarkTrue(condition.NetworkAttachmentsReadyCondition, condition.NetworkAttachmentsReadyMessage)
	} else {
		instance.Status.Conditions.Set(
			condition.FalseCondition(
				condition.NetworkAttachmentsReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.NetworkAttachmentsErrorMessage,
				instance.Spec.NetworkAttachments,
			),
		)
		return ctrl.Result{}, err
	}

	// Mark the Deployment as Ready only if the number of Replicas is equals
	// to the Deployed instances (ReadyCount), and the the Status.Replicas
	// match Status.ReadyReplicas. If a deployment update is in progress,
	// Replicas > ReadyReplicas.
	// In addition, make sure the controller sees the last Generation
	// by comparing it with the ObservedGeneration.
	if deployment.IsReady(deploy) {
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	} else {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
	}
	// create Deployment - end

	if instance.Status.ReadyCount == *instance.Spec.Replicas {
		// remove finalizers from unused MariaDBAccount records
		err = mariadbv1.DeleteUnusedMariaDBAccountFinalizers(ctx, helper, keystone.DatabaseName, instance.Spec.DatabaseAccount, instance.Namespace)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// create CronJob
	cronjobDef := keystone.CronJob(instance, serviceLabels, serviceAnnotations, memcached)
	cronjob := cronjob.NewCronJob(
		cronjobDef,
		5*time.Second,
	)

	ctrlResult, err = cronjob.CreateOrPatch(ctx, helper)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.CronJobReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.CronJobReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	}

	instance.Status.Conditions.MarkTrue(condition.CronJobReadyCondition, condition.CronJobReadyMessage)
	// create CronJob - end

	//
	// create OpenStackClient config
	//
	err = r.reconcileCloudConfig(ctx, helper, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	Log.Info("Reconciled Service successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) transportURLCreateOrUpdate(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	serviceLabels map[string]string,
) (*rabbitmqv1.TransportURL, controllerutil.OperationResult, error) {
	transportURL := &rabbitmqv1.TransportURL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-keystone-transport", instance.Name),
			Namespace: instance.Namespace,
			Labels:    serviceLabels,
		},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, transportURL, func() error {
		// Handle nil NotificationsBus gracefully - use default values
		if instance.Spec.NotificationsBus != nil {
			transportURL.Spec.RabbitmqClusterName = instance.Spec.NotificationsBus.Cluster
			// Always set Username and Vhost to allow clearing/resetting them
			// The infra-operator TransportURL controller handles empty values:
			// - Empty Username: uses default cluster admin credentials
			// - Empty Vhost: defaults to "/" vhost
			transportURL.Spec.Username = instance.Spec.NotificationsBus.User
			transportURL.Spec.Vhost = instance.Spec.NotificationsBus.Vhost
		} else {
			// If NotificationsBus is nil, use default rabbitmq cluster
			transportURL.Spec.RabbitmqClusterName = "rabbitmq"
			transportURL.Spec.Username = ""
			transportURL.Spec.Vhost = ""
		}
		return controllerutil.SetControllerReference(instance, transportURL, r.Scheme)
	})

	return transportURL, op, err
}

// generateServiceConfigMaps - create create configmaps which hold scripts and service configuration
// TODO add DefaultConfigOverwrite
func (r *KeystoneAPIReconciler) generateServiceConfigMaps(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	h *helper.Helper,
	envVars *map[string]env.Setter,
	mc *memcachedv1.Memcached,
	db *mariadbv1.Database,
) error {
	//
	// create Configmap/Secret required for keystone input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal keystone config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the ospSecret via the init container
	//

	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(keystone.ServiceName), map[string]string{})

	var tlsCfg *tls.Service
	if instance.Spec.TLS.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}

	// customData hold any customization for the service.
	// custom.conf is going to /etc/<service>/<service>.conf.d
	// all other files get placed into /etc/<service> to allow overwrite of e.g. policy.json
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{
		common.CustomServiceConfigFileName: instance.Spec.CustomServiceConfig,
		"my.cnf":                           db.GetDatabaseClientConfig(tlsCfg), //(mschuppert) for now just get the default my.cnf
	}
	maps.Copy(customData, instance.Spec.DefaultConfigOverwrite)

	transportURLSecret, _, err := oko_secret.GetSecret(ctx, h, instance.Status.TransportURLSecret, instance.Namespace)
	if err != nil {
		return err
	}

	databaseAccount := db.GetAccount()
	dbSecret := db.GetSecret()

	templateParameters := map[string]any{
		"MemcachedServers":         mc.GetMemcachedServerListString(),
		"MemcachedServersWithInet": mc.GetMemcachedServerListWithInetString(),
		"MemcachedTLS":             mc.GetMemcachedTLSSupport(),
		"TransportURL":             string(transportURLSecret.Data["transport_url"]),
		"DatabaseConnection": fmt.Sprintf("mysql+pymysql://%s:%s@%s/%s?read_default_file=/etc/my.cnf",
			databaseAccount.Spec.UserName,
			string(dbSecret.Data[mariadbv1.DatabasePasswordSelector]),
			instance.Status.DatabaseHostname,
			keystone.DatabaseName,
		),
		"ProcessNumber":       instance.Spec.HttpdCustomization.ProcessNumber,
		"EnableSecureRBAC":    instance.Spec.EnableSecureRBAC,
		"FernetMaxActiveKeys": instance.Spec.FernetMaxActiveKeys,
	}

	templateParameters["KeystoneEndpointPublic"], _ = instance.GetEndpoint(endpoint.EndpointPublic)
	templateParameters["KeystoneEndpointInternal"], _ = instance.GetEndpoint(endpoint.EndpointInternal)

	// MTLS params
	if mc.GetMemcachedMTLSSecret() != "" {
		templateParameters["MemcachedAuthCert"] = fmt.Sprint(memcachedv1.CertMountPath())
		templateParameters["MemcachedAuthKey"] = fmt.Sprint(memcachedv1.KeyMountPath())
		templateParameters["MemcachedAuthCa"] = fmt.Sprint(memcachedv1.CaMountPath())
	}

	// Check if Quorum Queues are enabled
	templateParameters["QuorumQueues"] = string(transportURLSecret.Data["quorumqueues"]) == "true"

	httpdOverrideSecret := &corev1.Secret{}
	if instance.Spec.HttpdCustomization.CustomConfigSecret != nil && *instance.Spec.HttpdCustomization.CustomConfigSecret != "" {
		httpdOverrideSecret, _, err = oko_secret.GetSecret(ctx, h, *instance.Spec.HttpdCustomization.CustomConfigSecret, instance.Namespace)
		if err != nil {
			return err
		}
	}

	// create httpd  vhost template parameters
	customTemplates := map[string]string{}
	httpdVhostConfig := map[string]any{}
	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		endptConfig := map[string]any{}
		endptConfig["ServerName"] = fmt.Sprintf("%s-%s.%s.svc", instance.Name, endpt.String(), instance.Namespace)
		endptConfig["TLS"] = false // default TLS to false, and set it bellow to true if enabled
		if instance.Spec.TLS.API.Enabled(endpt) {
			endptConfig["TLS"] = true
			endptConfig["SSLCertificateFile"] = fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String())
			endptConfig["SSLCertificateKeyFile"] = fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String())
		}

		endptConfig["Override"] = false
		if len(httpdOverrideSecret.Data) > 0 {
			endptConfig["Override"] = true
			for key, data := range httpdOverrideSecret.Data {
				if len(data) > 0 {
					customTemplates["httpd_custom_"+endpt.String()+"_"+key] = string(data)
				}
			}
		}
		httpdVhostConfig[endpt.String()] = endptConfig
	}
	templateParameters["VHosts"] = httpdVhostConfig
	templateParameters["TimeOut"] = instance.Spec.APITimeout

	// Marshal the templateParameters map to YAML
	yamlData, err := yaml.Marshal(templateParameters)
	if err != nil {
		return fmt.Errorf("error marshalling to YAML: %w", err)
	}
	customData[common.TemplateParameters] = string(yamlData)

	tmpl := []util.Template{
		// Scripts
		{
			Name:         fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:    instance.Namespace,
			Type:         util.TemplateTypeScripts,
			InstanceType: instance.Kind,
			Labels:       cmLabels,
		},
		// Configs
		{
			Name:           fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:      instance.Namespace,
			Type:           util.TemplateTypeConfig,
			InstanceType:   instance.Kind,
			StringTemplate: customTemplates,
			CustomData:     customData,
			ConfigOptions:  templateParameters,
			Labels:         cmLabels,
		},
	}
	return oko_secret.EnsureSecrets(ctx, h, instance, tmpl, envVars)
}

// reconcileConfigMap -  creates clouds.yaml
// TODO: most likely should be part of the higher openstack operator
func (r *KeystoneAPIReconciler) reconcileCloudConfig(
	ctx context.Context,
	h *helper.Helper,
	instance *keystonev1.KeystoneAPI,
) error {
	// clouds.yaml
	var openStackConfig keystone.OpenStackConfig
	templateParameters := make(map[string]any)
	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(keystone.ServiceName), map[string]string{})

	authURL, err := instance.GetEndpoint(endpoint.EndpointPublic)
	if err != nil {
		return err
	}
	openStackConfig.Clouds.Default.Auth.AuthURL = authURL
	openStackConfig.Clouds.Default.Auth.ProjectName = instance.Spec.AdminProject
	openStackConfig.Clouds.Default.Auth.UserName = instance.Spec.AdminUser
	openStackConfig.Clouds.Default.Auth.UserDomainName = "Default"
	openStackConfig.Clouds.Default.Auth.ProjectDomainName = "Default"
	openStackConfig.Clouds.Default.RegionName = instance.Spec.Region

	cloudsYamlVal, err := yaml.Marshal(&openStackConfig)
	if err != nil {
		return err
	}
	cloudsYaml := map[string]string{
		"clouds.yaml": string(cloudsYamlVal),
		"OS_CLOUD":    "default",
	}

	cms := []util.Template{
		{
			Name:          "openstack-config",
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeNone,
			InstanceType:  instance.Kind,
			CustomData:    cloudsYaml,
			ConfigOptions: templateParameters,
			Labels:        cmLabels,
		},
	}
	err = configmap.EnsureConfigMaps(ctx, h, instance, cms, nil)
	if err != nil {
		return err
	}

	// secure.yaml
	keystoneSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Spec.Secret,
			Namespace: instance.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
	}

	err = r.Get(ctx, types.NamespacedName{Name: keystoneSecret.Name, Namespace: instance.Namespace}, keystoneSecret)
	if err != nil {
		return err
	}

	var openStackConfigSecret keystone.OpenStackConfigSecret
	openStackConfigSecret.Clouds.Default.Auth.Password = string(keystoneSecret.Data[instance.Spec.PasswordSelectors.Admin])

	secretVal, err := yaml.Marshal(&openStackConfigSecret)
	if err != nil {
		return err
	}
	cloudrc := keystone.GenerateCloudrc(&openStackConfigSecret, &openStackConfig)
	secretString := map[string]string{
		"secure.yaml": string(secretVal),
		"cloudrc":     string(cloudrc),
	}

	secrets := []util.Template{
		{
			Name:          "openstack-config-secret",
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeNone,
			InstanceType:  instance.Kind,
			CustomData:    secretString,
			ConfigOptions: templateParameters,
			Labels:        cmLabels,
		},
	}

	return oko_secret.EnsureSecrets(ctx, h, instance, secrets, nil)
}

// ensureFernetKeys - creates secret with fernet keys, rotates the keys
func (r *KeystoneAPIReconciler) ensureFernetKeys(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	helper *helper.Helper,
	envVars *map[string]env.Setter,
) error {
	logger := r.GetLogger(ctx)
	fernetAnnotation := labels.GetGroupLabel(keystone.ServiceName) + "/rotatedat"
	labels := labels.GetLabels(instance, labels.GetGroupLabel(keystone.ServiceName), map[string]string{})
	now := time.Now().UTC()

	//
	// check if secret already exist
	//
	secretName := keystone.ServiceName
	var numberKeys int
	if instance.Spec.FernetMaxActiveKeys == nil {
		numberKeys = keystone.DefaultFernetMaxActiveKeys
	} else {
		numberKeys = int(*instance.Spec.FernetMaxActiveKeys)
	}

	secret, _, err := oko_secret.GetSecret(ctx, helper, secretName, instance.Namespace)

	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	} else if k8s_errors.IsNotFound(err) {
		fernetKeys := map[string]string{
			"CredentialKeys0": keystone.GenerateFernetKey(logger),
			"CredentialKeys1": keystone.GenerateFernetKey(logger),
		}

		for i := 0; i < numberKeys; i++ {
			fernetKeys[fmt.Sprintf("FernetKeys%d", i)] = keystone.GenerateFernetKey(logger)
		}

		annotations := map[string]string{
			fernetAnnotation: now.Format(time.RFC3339)}

		tmpl := []util.Template{
			{
				Name:        secretName,
				Namespace:   instance.Namespace,
				Type:        util.TemplateTypeNone,
				CustomData:  fernetKeys,
				Labels:      labels,
				Annotations: annotations,
			},
		}
		err := oko_secret.EnsureSecrets(ctx, helper, instance, tmpl, envVars)
		if err != nil {
			return err
		}
	} else {
		// DON'T add hash to envVars to prevent pod restarts when keys rotate
		// Keys are mounted directly to /etc/keystone/fernet-keys, so Kubernetes
		// will propagate changes automatically without needing pod recreation

		changedKeys := false

		extraKey := fmt.Sprintf("FernetKeys%d", numberKeys)

		//
		// Fernet Key rotation
		//
		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}
		rotatedAt, err := time.Parse(time.RFC3339, secret.Annotations[fernetAnnotation])

		var duration int
		if instance.Spec.FernetRotationDays == nil {
			duration = keystone.DefaultFernetRotationDays
		} else {
			duration = int(*instance.Spec.FernetRotationDays)
		}

		if err != nil {
			changedKeys = true
		} else if rotatedAt.AddDate(0, 0, duration).Before(now) {
			secret.Data[extraKey] = secret.Data["FernetKeys0"]
			secret.Data["FernetKeys0"] = []byte(keystone.GenerateFernetKey(logger))
		}

		//
		// Remove extra keys when FernetMaxActiveKeys changes
		//
		for {
			_, exists := secret.Data[extraKey]
			if !exists {
				break
			}
			changedKeys = true
			i := 1
			for {
				key := fmt.Sprintf("FernetKeys%d", i)
				i++
				nextKey := fmt.Sprintf("FernetKeys%d", i)
				_, exists = secret.Data[nextKey]
				if !exists {
					break
				}
				secret.Data[key] = secret.Data[nextKey]
				delete(secret.Data, nextKey)
			}
		}

		//
		// Add extra keys when FernetMaxActiveKeys changes
		//
		lastKey := fmt.Sprintf("FernetKeys%d", numberKeys-1)
		for {
			_, exists := secret.Data[lastKey]
			if exists {
				break
			}
			changedKeys = true
			i := 1
			nextKeyValue := []byte(keystone.GenerateFernetKey(logger))
			for {
				key := fmt.Sprintf("FernetKeys%d", i)
				i++
				keyValue, exists := secret.Data[key]
				secret.Data[key] = nextKeyValue
				nextKeyValue = keyValue
				if !exists {
					break
				}
			}
		}

		if !changedKeys {
			return nil
		}

		secret.Annotations[fernetAnnotation] = now.Format(time.RFC3339)

		// use update to apply changes to the secret, since EnsureSecrets
		// does not handle annotation updates, also CreateOrPatchSecret would
		// preserve the existing annotation
		err = helper.GetClient().Update(ctx, secret, &client.UpdateOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

// ensureFederationRealmConfig - create secret with federation realm config
// only used for multiple realm configuration
// returns the array of sorted filenames
func (r *KeystoneAPIReconciler) ensureFederationRealmConfig(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	helper *helper.Helper,
	envVars *map[string]env.Setter,
) ([]string, error) {
	logger := r.GetLogger(ctx)

	if instance.Spec.FederatedRealmConfig == "" {
		return nil, nil
	}

	// Verify that the federation secret object exists
	federationSecret, hash, err := oko_secret.GetSecret(ctx, helper, instance.Spec.FederatedRealmConfig, instance.Namespace)
	if err != nil {
		return nil, err
	}

	// Add the secret hash to envVars for change detection
	(*envVars)[instance.Spec.FederatedRealmConfig] = env.SetValue(hash)

	// Get the data from the federation secret, we expect this to be a JSON dict
	jsonData, ok := federationSecret.Data[keystone.FederationConfigKey]
	if !ok {
		return nil, fmt.Errorf("key %s not found in secret %s: %w", keystone.FederationConfigKey, instance.Spec.FederatedRealmConfig, util.ErrFieldNotFound)
	}

	// Parse the JSON content into a map
	var rawConfigs map[string]json.RawMessage
	err = json.Unmarshal(jsonData, &rawConfigs)
	if err != nil {
		logger.Error(err, "Failed to unmarshal nested JSON from 'federation-config.json'")
		return nil, err
	}

	// Extract and Sort Filenames (Keys)
	var sortedFilenames []string
	for filename := range rawConfigs {
		sortedFilenames = append(sortedFilenames, filename)
	}
	sort.Strings(sortedFilenames)

	// Prepare data for the new Secret
	newSecretData := make(map[string]string)

	// Iterate through the *sorted* filenames to populate the new Secret
	for index, filename := range sortedFilenames {
		// Get the raw content for the current filename
		content, ok := rawConfigs[filename]
		if !ok {
			// logger.Warning("Warning: Content for sorted filename '%s' not found in rawConfigs. Skipping.", filename)
			continue
		}

		newSecretData[strconv.Itoa(index)] = string(content) // Use sorted index as key
	}

	// Store the *sorted* original filenames as a JSON array in a special key
	filenamesJSON, err := json.Marshal(sortedFilenames)
	if err != nil {
		logger.Error(err, "Failed to marshal sorted filenames")
		return nil, err
	}
	newSecretData["_filenames.json"] = string(filenamesJSON)

	templateParameters := make(map[string]any)
	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(keystone.ServiceName), map[string]string{})
	newSecret := []util.Template{
		{
			Name:          keystone.FederationMultiRealmSecret,
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeNone,
			InstanceType:  instance.Kind,
			CustomData:    newSecretData,
			ConfigOptions: templateParameters,
			Labels:        cmLabels,
		},
	}

	err = oko_secret.EnsureSecrets(ctx, helper, instance, newSecret, envVars)
	if err != nil {
		logger.Error(err, "Failed to create new secret")
		return nil, err
	}
	return sortedFilenames, nil
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
//
// returns the hash, whether the hash changed (as a bool) and any error
func (r *KeystoneAPIReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	envVars map[string]env.Setter,
) (string, bool, error) {
	Log := r.GetLogger(ctx)
	var hashMap map[string]string
	changed := false
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, changed, err
	}
	if hashMap, changed = util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		Log.Info(fmt.Sprintf("Input maps hash %s - %s", common.InputHashName, hash))
	}
	return hash, changed, nil
}

func (r *KeystoneAPIReconciler) ensureDB(
	ctx context.Context,
	h *helper.Helper,
	instance *keystonev1.KeystoneAPI,
) (*mariadbv1.Database, ctrl.Result, error) {

	// ensure MariaDBAccount exists.  This account record may be created by
	// openstack-operator or the cloud operator up front without a specific
	// MariaDBDatabase configured yet.   Otherwise, a MariaDBAccount CR is
	// created here with a generated username as well as a secret with
	// generated password.   The MariaDBAccount is created without being
	// yet associated with any MariaDBDatabase.
	_, _, err := mariadbv1.EnsureMariaDBAccount(
		ctx, h, instance.Spec.DatabaseAccount,
		instance.Namespace, false, keystone.DatabaseUsernamePrefix,
	)

	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			mariadbv1.MariaDBAccountReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			mariadbv1.MariaDBAccountNotReadyMessage,
			err.Error()))

		return nil, ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(
		mariadbv1.MariaDBAccountReadyCondition,
		mariadbv1.MariaDBAccountReadyMessage)

	//
	// create service DB instance
	//
	db := mariadbv1.NewDatabaseForAccount(
		instance.Spec.DatabaseInstance, // mariadb/galera service to target
		keystone.DatabaseName,          // name used in CREATE DATABASE in mariadb
		keystone.DatabaseCRName,        // CR name for MariaDBDatabase
		instance.Spec.DatabaseAccount,  // CR name for MariaDBAccount
		instance.Namespace,             // namespace
	)

	// create or patch the DB
	ctrlResult, err := db.CreateOrPatchAll(ctx, h)

	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return db, ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return db, ctrlResult, nil
	}
	// wait for the DB to be setup
	// (ksambor) should we use WaitForDBCreatedWithTimeout instead?
	ctrlResult, err = db.WaitForDBCreated(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return db, ctrlResult, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return db, ctrlResult, nil
	}

	// update Status.DatabaseHostname, used to config the service
	instance.Status.DatabaseHostname = db.GetDatabaseHostname()
	instance.Status.Conditions.MarkTrue(condition.DBReadyCondition, condition.DBReadyMessage)
	return db, ctrlResult, nil
}
