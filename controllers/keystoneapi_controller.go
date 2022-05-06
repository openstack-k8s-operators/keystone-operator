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
	routev1 "github.com/openshift/api/route/v1"
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	keystone "github.com/openstack-k8s-operators/keystone-operator/pkg/keystone"
	common "github.com/openstack-k8s-operators/lib-common/pkg/common"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;create;update;delete;

// Reconcile reconcile keystone API requests
func (r *KeystoneAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("keystoneapi", req.NamespacedName)

	// Fetch the KeystoneAPI instance
	instance := &keystonev1beta1.KeystoneAPI{}
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

	// Secret
	secret := keystone.FernetSecret(instance, instance.Name)
	// Check if this Secret already exists
	foundSecret := &corev1.Secret{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, foundSecret)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new Secret", "Secret.Namespace", secret.Namespace, "Job.Name", secret.Name)
		err = r.Client.Create(ctx, secret)
		if err != nil {
			return ctrl.Result{}, err
		}
		if err := controllerutil.SetControllerReference(instance, secret, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
	}

	// ConfigMap
	configMapVars := make(map[string]common.EnvSetter)
	err = r.generateServiceConfigMaps(ctx, instance, &configMapVars)
	if err != nil {
		return ctrl.Result{}, err
	}
	mergedMapVars := common.MergeEnvs([]corev1.EnvVar{}, configMapVars)
	configHash := ""
	for _, hashEnv := range mergedMapVars {
		configHash = configHash + hashEnv.Value
	}

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating configmap hash: %v", err)
	}

	// Create the database (unstructured so we don't explicitly import mariadb-operator code)
	databaseObj, err := keystone.DatabaseObject(instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	foundDatabase := &unstructured.Unstructured{}
	foundDatabase.SetGroupVersionKind(databaseObj.GroupVersionKind())
	err = r.Client.Get(ctx, types.NamespacedName{Name: databaseObj.GetName(), Namespace: databaseObj.GetNamespace()}, foundDatabase)
	if err != nil && k8s_errors.IsNotFound(err) {
		err := r.Client.Create(ctx, &databaseObj)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		completed, _, err := unstructured.NestedBool(foundDatabase.UnstructuredContent(), "status", "completed")
		if !completed {
			r.Log.Info("Waiting on DB to be created...")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	}

	// Define a new Job object
	job, err := keystone.DbSyncJob(instance, instance.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error getting dbSyncJob: %v", err)
	}
	dbSyncHash, err := common.ObjectHash(job)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating DB sync hash: %v", err)
	}

	if instance.Status.DbSyncHash != dbSyncHash {

		op, err := controllerutil.CreateOrPatch(ctx, r.Client, job, func() error {
			err := controllerutil.SetControllerReference(instance, job, r.Scheme)
			if err != nil {
				// FIXME error conditions
				return err

			}

			return nil
		})
		if err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if op != controllerutil.OperationResultNone {
			// FIXME: error conditions
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}

		requeue, err := common.WaitOnJob(ctx, job, r.Client, r.Log)
		r.Log.Info("Running DB Sync")
		if err != nil {
			return ctrl.Result{}, err
		} else if requeue {
			r.Log.Info("Waiting on DB sync")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}

	}
	// db sync completed... okay to store the hash to disable it
	if err := r.setDbSyncHash(ctx, instance, dbSyncHash); err != nil {
		return ctrl.Result{}, err
	}
	// delete the job
	_, err = common.DeleteJob(ctx, job, r.Kclient, r.Log)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Define a new Deployment object
	r.Log.Info("ConfigMapHash: ", "Data Hash:", configHash)
	deployment, err := keystone.Deployment(instance, instance.Name, configHash)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error creating deployment: %v", err)
	}
	deploymentHash, err := common.ObjectHash(deployment)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error deployment hash: %v", err)
	}
	r.Log.Info("DeploymentHash: ", "Deployment Hash:", deploymentHash)

	// Check if this Deployment already exists
	foundDeployment := &appsv1.Deployment{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		err = r.Client.Create(ctx, deployment)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Set KeystoneAPI instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, deployment, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second * 5}, err

	} else if err != nil {
		return ctrl.Result{}, err
	} else {

		if instance.Status.DeploymentHash != deploymentHash {
			r.Log.Info("Deployment Updated")
			foundDeployment.Spec = deployment.Spec
			err = r.Client.Update(ctx, foundDeployment)
			if err != nil {
				return ctrl.Result{}, err
			}
			if err := r.setDeploymentHash(ctx, instance, deploymentHash); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{RequeueAfter: time.Second * 10}, err
		}
		if foundDeployment.Status.ReadyReplicas == instance.Spec.Replicas {
			r.Log.Info("Deployment Replicas running:", "Replicas", foundDeployment.Status.ReadyReplicas)
		} else {
			r.Log.Info("Waiting on Keystone Deployment...")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	}

	// Create the service if none exists
	var keystonePort int32 = 5000
	service := keystone.Service(instance, instance.Name, keystonePort)

	// Check if this Service already exists
	foundService := &corev1.Service{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		err = r.Client.Create(ctx, service)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Set KeystoneAPI instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, service, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Create the route if none exists
	route := keystone.Route(instance, instance.Name)

	// Check if this Route already exists
	foundRoute := &routev1.Route{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, foundRoute)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new Route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
		err = r.Client.Create(ctx, route)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Set Keystone instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, route, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Look at the generated route to get the value for the initial endpoint
	// FIXME: need to support https default here
	var apiEndpoint string
	if !strings.HasPrefix(foundRoute.Spec.Host, "http") {
		apiEndpoint = fmt.Sprintf("http://%s", foundRoute.Spec.Host)
	} else {
		apiEndpoint = foundRoute.Spec.Host
	}
	err = r.setAPIEndpoint(ctx, instance, apiEndpoint)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Define a new BootStrap Job object
	bootstrapJob, err := keystone.BootstrapJob(instance, instance.Name, apiEndpoint)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error creating bootstrap job: %v", err)
	}
	bootstrapHash, err := common.ObjectHash(bootstrapJob)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating bootstrap hash: %v", err)
	}

	// Set KeystoneAPI instance as the owner and controller
	if instance.Status.BootstrapHash != bootstrapHash {
		op, err := controllerutil.CreateOrPatch(ctx, r.Client, bootstrapJob, func() error {
			err := controllerutil.SetControllerReference(instance, bootstrapJob, r.Scheme)
			if err != nil {
				// FIXME error conditions
				return err
			}

			return nil
		})
		if err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if op != controllerutil.OperationResultNone {
			// FIXME: error conditions
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}

		requeue, err := common.WaitOnJob(ctx, bootstrapJob, r.Client, r.Log)
		r.Log.Info("Running keystone bootstrap")
		if err != nil {
			return ctrl.Result{}, err
		} else if requeue {
			r.Log.Info("Waiting on keystone bootstrap")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}

	}
	// bootstrap completed... okay to store the hash to disable it
	if err := r.setBootstrapHash(ctx, instance, bootstrapHash); err != nil {
		return ctrl.Result{}, err
	}

	// delete the job
	_, err = common.DeleteJob(ctx, bootstrapJob, r.Kclient, r.Log)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.reconcileConfigMap(ctx, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager x
func (r *KeystoneAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keystonev1beta1.KeystoneAPI{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
		//Owns(&routev1.Route{}).
}

func (r *KeystoneAPIReconciler) setDbSyncHash(ctx context.Context, instance *keystonev1beta1.KeystoneAPI, hashStr string) error {

	if hashStr != instance.Status.DbSyncHash {
		instance.Status.DbSyncHash = hashStr
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *KeystoneAPIReconciler) setBootstrapHash(ctx context.Context, instance *keystonev1beta1.KeystoneAPI, hashStr string) error {

	if hashStr != instance.Status.BootstrapHash {
		instance.Status.BootstrapHash = hashStr
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *KeystoneAPIReconciler) setDeploymentHash(ctx context.Context, instance *keystonev1beta1.KeystoneAPI, hashStr string) error {

	if hashStr != instance.Status.DeploymentHash {
		instance.Status.DeploymentHash = hashStr
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return err
		}
	}
	return nil

}

// setAPIEndpoint func
func (r *KeystoneAPIReconciler) setAPIEndpoint(ctx context.Context, instance *keystonev1beta1.KeystoneAPI, apiEndpoint string) error {

	if apiEndpoint != instance.Status.APIEndpoint {
		instance.Status.APIEndpoint = apiEndpoint
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *KeystoneAPIReconciler) generateServiceConfigMaps(
	ctx context.Context,
	instance *keystonev1beta1.KeystoneAPI,
	envVars *map[string]common.EnvSetter,
) error {
	// FIXME: use common.GetLabels?
	cmLabels := keystone.GetLabels(instance.Name)
	templateParameters := make(map[string]interface{})

	// ConfigMaps for mariadb
	cms := []common.Template{
		// ScriptsConfigMap
		{
			Name:               "keystone-" + instance.Name,
			Namespace:          instance.Namespace,
			Type:               common.TemplateTypeScripts,
			InstanceType:       instance.Kind,
			AdditionalTemplate: map[string]string{},
			ConfigOptions:      templateParameters,
			Labels:             cmLabels,
		},
	}

	err := common.EnsureConfigMaps(ctx, r, instance, cms, envVars)

	if err != nil {
		// FIXME error conditions here
		return err
	}

	return nil
}

func (r *KeystoneAPIReconciler) reconcileConfigMap(ctx context.Context, instance *keystonev1beta1.KeystoneAPI) error {

	configMapName := "openstack-config"
	var openStackConfig keystone.OpenStackConfig
	openStackConfig.Clouds.Default.Auth.AuthURL = instance.Status.APIEndpoint
	openStackConfig.Clouds.Default.Auth.ProjectName = "admin"
	openStackConfig.Clouds.Default.Auth.UserName = "admin"
	openStackConfig.Clouds.Default.Auth.UserDomainName = "Default"
	openStackConfig.Clouds.Default.Auth.ProjectDomainName = "Default"
	openStackConfig.Clouds.Default.RegionName = "regionOne"

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
	openStackConfigSecret.Clouds.Default.Auth.Password = string(keystoneSecret.Data["AdminPassword"])

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
