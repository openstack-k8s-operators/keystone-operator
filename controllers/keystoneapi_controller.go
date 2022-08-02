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
	"time"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	keystone "github.com/openstack-k8s-operators/keystone-operator/pkg/keystone"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	configmap "github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	deployment "github.com/openstack-k8s-operators/lib-common/modules/common/deployment"
	endpoint "github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	job "github.com/openstack-k8s-operators/lib-common/modules/common/job"
	labels "github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	oko_secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	database "github.com/openstack-k8s-operators/lib-common/modules/database"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"

	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// GetClient -
func (r *KeystoneAPIReconciler) GetClient() client.Client {
	return r.Client
}

// GetKClient -
func (r *KeystoneAPIReconciler) GetKClient() kubernetes.Interface {
	return r.Kclient
}

// GetLogger -
func (r *KeystoneAPIReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetScheme -
func (r *KeystoneAPIReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// KeystoneAPIReconciler reconciles a KeystoneAPI object
type KeystoneAPIReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete;

// Reconcile reconcile keystone API requests
func (r *KeystoneAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("keystoneapi", req.NamespacedName)

	// Fetch the KeystoneAPI instance
	instance := &keystonev1.KeystoneAPI{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
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

	//
	// initialize status
	//
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.List{}
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.APIEndpoints == nil {
		instance.Status.APIEndpoints = map[string]string{}
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

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper)
}

// SetupWithManager -
func (r *KeystoneAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keystonev1.KeystoneAPI{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Owns(&routev1.Route{}).
		Complete(r)
}

func (r *KeystoneAPIReconciler) reconcileDelete(ctx context.Context, instance *keystonev1.KeystoneAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service delete")

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	r.Log.Info("Reconciled Service delete successfully")
	if err := r.Update(ctx, instance); err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileInit(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service init")

	//
	// create service DB instance
	//
	db := database.NewDatabase(
		instance.Name,
		instance.Spec.DatabaseUser,
		instance.Spec.Secret,
		map[string]string{
			"dbName": instance.Spec.DatabaseInstance,
		},
	)
	// create or patch the DB
	ctrlResult, err := db.CreateOrPatchDB(
		ctx,
		helper,
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// wait for the DB to be setup
	ctrlResult, err = db.WaitForDBCreated(ctx, helper)
	if err != nil {
		return ctrlResult, err
	}
	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}
	// update Status.DatabaseHostname, used to bootstrap/config the service
	instance.Status.DatabaseHostname = db.GetDatabaseHostname()
	// create service DB - end

	//
	// run keystone db sync
	//
	dbSyncHash := instance.Status.Hash[keystonev1.DbSyncHash]
	jobDef := keystone.DbSyncJob(instance, serviceLabels)
	dbSyncjob := job.NewJob(
		jobDef,
		keystonev1.DbSyncHash,
		instance.Spec.PreserveJobs,
		5,
		dbSyncHash,
	)
	ctrlResult, err = dbSyncjob.DoJob(
		ctx,
		helper,
	)
	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if dbSyncjob.HasChanged() {
		instance.Status.Hash[keystonev1.DbSyncHash] = dbSyncjob.GetHash()
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		r.Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.Hash[keystonev1.DbSyncHash]))
	}
	// run keystone db sync - end

	//
	// expose the service (create service, route and return the created endpoint URLs)
	//
	var keystonePorts = map[endpoint.Endpoint]endpoint.EndpointData{
		endpoint.EndpointAdmin: endpoint.EndpointData{
			Port: keystone.KeystoneAdminPort,
		},
		endpoint.EndpointPublic: endpoint.EndpointData{
			Port: keystone.KeystonePublicPort,
		},
		endpoint.EndpointInternal: endpoint.EndpointData{
			Port: keystone.KeystoneInternalPort,
		},
	}

	apiEndpoints, ctrlResult, err := endpoint.ExposeEndpoints(
		ctx,
		helper,
		keystone.ServiceName,
		serviceLabels,
		keystonePorts,
	)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// Update instance status with service endpoint url from route host information
	//
	// TODO: need to support https default here
	if instance.Status.APIEndpoints == nil {
		instance.Status.APIEndpoints = map[string]string{}
	}
	instance.Status.APIEndpoints = apiEndpoints

	// expose service - end

	//
	// BootStrap Job
	//
	jobDef = keystone.BootstrapJob(instance, serviceLabels, instance.Status.APIEndpoints)
	bootstrapjob := job.NewJob(
		jobDef,
		keystonev1.BootstrapHash,
		instance.Spec.PreserveJobs,
		5,
		instance.Status.Hash[keystonev1.BootstrapHash],
	)
	ctrlResult, err = bootstrapjob.DoJob(
		ctx,
		helper,
	)
	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if bootstrapjob.HasChanged() {
		instance.Status.Hash[keystonev1.BootstrapHash] = bootstrapjob.GetHash()
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		r.Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.Hash[keystonev1.BootstrapHash]))
	}
	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}
	// run keystone bootstrap - end

	r.Log.Info("Reconciled Service init successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileUpdate(ctx context.Context, instance *keystonev1.KeystoneAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled Service update successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileUpgrade(ctx context.Context, instance *keystonev1.KeystoneAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service upgrade")

	// TODO: should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled Service upgrade successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileNormal(ctx context.Context, instance *keystonev1.KeystoneAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service")

	// If the service object doesn't have our finalizer, add it.
	controllerutil.AddFinalizer(instance, helper.GetFinalizer())
	// Register the finalizer immediately to avoid orphaning resources on delete
	if err := r.Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	// ConfigMap
	configMapVars := make(map[string]env.Setter)

	//
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	//
	ospSecret, hash, err := oko_secret.GetSecret(ctx, helper, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: time.Second * 10}, fmt.Errorf("OpenStack secret %s not found", instance.Spec.Secret)
		}
		return ctrl.Result{}, err
	}
	configMapVars[ospSecret.Name] = env.SetValue(hash)
	// run check OpenStack secret - end

	//
	// Create ConfigMaps and Secrets required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create Configmap required for keystone input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal keystone config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the OpenStack secret via the init container
	//
	err = r.generateServiceConfigMaps(ctx, instance, helper, &configMapVars)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// Create secret holding fernet keys
	//
	// TODO key rotation
	err = r.ensureFernetKeys(ctx, instance, helper, &configMapVars)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// create hash over all the different input resources to identify if any those changed
	// and a restart/recreate is required.
	//
	inputHash, err := r.createHashOfInputHashes(ctx, instance, configMapVars)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Create ConfigMaps and Secrets - end

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	serviceLabels := map[string]string{
		common.AppSelector: keystone.ServiceName,
	}

	// Handle service init
	ctrlResult, err := r.reconcileInit(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service update
	ctrlResult, err = r.reconcileUpdate(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// normal reconcile tasks
	//

	// Define a new Deployment object
	depl := deployment.NewDeployment(
		keystone.Deployment(instance, inputHash, serviceLabels),
		5,
	)

	ctrlResult, err = depl.CreateOrPatch(ctx, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}
	instance.Status.ReadyCount = depl.GetDeployment().Status.ReadyReplicas
	// create Deployment - end

	//
	// create OpenStackClient config
	//
	err = r.reconcileConfigMap(ctx, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.Log.Info("Reconciled Service successfully")
	return ctrl.Result{}, nil
}

//
// generateServiceConfigMaps - create create configmaps which hold scripts and service configuration
// TODO add DefaultConfigOverwrite
//
func (r *KeystoneAPIReconciler) generateServiceConfigMaps(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	h *helper.Helper,
	envVars *map[string]env.Setter,
) error {
	//
	// create Configmap/Secret required for keystone input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal keystone config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the ospSecret via the init container
	//

	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(keystone.ServiceName), map[string]string{})

	// customData hold any customization for the service.
	// custom.conf is going to /etc/<service>/<service>.conf.d
	// all other files get placed into /etc/<service> to allow overwrite of e.g. logging.conf or policy.json
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{common.CustomServiceConfigFileName: instance.Spec.CustomServiceConfig}
	for key, data := range instance.Spec.DefaultConfigOverwrite {
		customData[key] = data
	}

	templateParameters := make(map[string]interface{})

	cms := []util.Template{
		// ScriptsConfigMap
		{
			Name:               fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:          instance.Namespace,
			Type:               util.TemplateTypeScripts,
			InstanceType:       instance.Kind,
			AdditionalTemplate: map[string]string{"common.sh": "/common/common.sh"},
			Labels:             cmLabels,
		},
		// ConfigMap
		{
			Name:          fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeConfig,
			InstanceType:  instance.Kind,
			CustomData:    customData,
			ConfigOptions: templateParameters,
			Labels:        cmLabels,
		},
	}
	err := configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
	if err != nil {
		return nil
	}

	return nil
}

//
// reconcileConfigMap -  creates clouds.yaml
// TODO: most likely should be part of the higher openstack operator
//
func (r *KeystoneAPIReconciler) reconcileConfigMap(ctx context.Context, instance *keystonev1.KeystoneAPI) error {

	configMapName := "openstack-config"
	var openStackConfig keystone.OpenStackConfig
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
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: instance.Namespace,
		},
	}

	r.Log.Info("Reconciling ConfigMap", "ConfigMap.Namespace", instance.Namespace, "configMap.Name", configMapName)
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.TypeMeta = metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		}
		cm.ObjectMeta = metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: instance.Namespace,
		}
		cm.Data = map[string]string{
			"clouds.yaml": string(cloudsYamlVal),
			"OS_CLOUD":    "default",
		}
		return nil
	})
	if err != nil {
		return err
	}

	keystoneSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Spec.Secret,
			Namespace: instance.Namespace,
		},
		Type: "Opaque",
	}

	err = r.Client.Get(ctx, types.NamespacedName{Name: keystoneSecret.Name, Namespace: instance.Namespace}, keystoneSecret)
	if err != nil {
		return err
	}

	secretName := "openstack-config-secret"
	var openStackConfigSecret keystone.OpenStackConfigSecret
	openStackConfigSecret.Clouds.Default.Auth.Password = string(keystoneSecret.Data[instance.Spec.PasswordSelectors.Admin])

	secretVal, err := yaml.Marshal(&openStackConfigSecret)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: instance.Namespace,
		},
	}
	if err != nil {
		return err
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		secret.TypeMeta = metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		}
		secret.ObjectMeta = metav1.ObjectMeta{
			Name:      secretName,
			Namespace: instance.Namespace,
		}
		secret.StringData = map[string]string{
			"secure.yaml": string(secretVal),
		}
		return nil
	})

	return err
}

//
// ensureFernetKeys - creates secret with fernet keys
//
func (r *KeystoneAPIReconciler) ensureFernetKeys(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	helper *helper.Helper,
	envVars *map[string]env.Setter,
) error {
	labels := labels.GetLabels(instance, labels.GetGroupLabel(keystone.ServiceName), map[string]string{})

	//
	// check if secret already exist
	//
	secret, hash, err := oko_secret.GetSecret(ctx, helper, keystone.ServiceName, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	} else if k8s_errors.IsNotFound(err) {
		fernetKeys := map[string]string{
			"0": keystone.GenerateFernetKey(),
			"1": keystone.GenerateFernetKey(),
		}

		tmpl := []util.Template{
			{
				Name:       keystone.ServiceName,
				Namespace:  instance.Namespace,
				Type:       util.TemplateTypeNone,
				CustomData: fernetKeys,
				Labels:     labels,
			},
		}
		err := oko_secret.EnsureSecrets(ctx, helper, instance, tmpl, envVars)
		if err != nil {
			return nil
		}

		return fmt.Errorf("OpenStack secret %s not found", instance.Spec.Secret)
	}

	// TODO: fernet key rotation

	// add hash to envVars
	(*envVars)[secret.Name] = env.SetValue(hash)

	return nil
}

//
// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
//
func (r *KeystoneAPIReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	envVars map[string]env.Setter,
) (string, error) {
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, err
	}
	if hashMap, changed := util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return hash, err
		}
		r.Log.Info(fmt.Sprintf("Input maps hash %s - %s", common.InputHashName, hash))
	}
	return hash, nil
}
