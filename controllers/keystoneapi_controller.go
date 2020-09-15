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
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	keystone "github.com/openstack-k8s-operators/keystone-operator/pkg/keystone"
	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
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

// KeystoneAPIReconciler reconciles a KeystoneAPI object
type KeystoneAPIReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// Reconcile reconcile keystone API requests
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;create;update;delete;
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;create;update;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;create;update;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;create;update;delete;
func (r *KeystoneAPIReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("keystoneapi", req.NamespacedName)

	// Fetch the KeystoneAPI instance
	instance := &keystonev1beta1.KeystoneAPI{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)
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
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, foundSecret)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new Secret", "Secret.Namespace", secret.Namespace, "Job.Name", secret.Name)
		err = r.Client.Create(context.TODO(), secret)
		if err != nil {
			return ctrl.Result{}, err
		}
		if err := controllerutil.SetControllerReference(instance, secret, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
	}

	// ConfigMap
	configMap := keystone.ConfigMap(instance, instance.Name)
	// Check if this ConfigMap already exists
	foundConfigMap := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new ConfigMap", "ConfigMap.Namespace", configMap.Namespace, "Job.Name", configMap.Name)
		err = r.Client.Create(context.TODO(), configMap)
		if err != nil {
			return ctrl.Result{}, err
		}
		if err := controllerutil.SetControllerReference(instance, configMap, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
	} else if !reflect.DeepEqual(configMap.Data, foundConfigMap.Data) {
		r.Log.Info("Updating ConfigMap")
		foundConfigMap.Data = configMap.Data
		err = r.Client.Update(context.TODO(), foundConfigMap)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}

	// Create the database (unstructured so we don't explicitly import mariadb-operator code)
	databaseObj, err := keystone.DatabaseObject(instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	foundDatabase := &unstructured.Unstructured{}
	foundDatabase.SetGroupVersionKind(databaseObj.GroupVersionKind())
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: databaseObj.GetName(), Namespace: databaseObj.GetNamespace()}, foundDatabase)
	if err != nil && k8s_errors.IsNotFound(err) {
		err := r.Client.Create(context.TODO(), &databaseObj)
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
	job := keystone.DbSyncJob(instance, instance.Name)
	dbSyncHash, err := util.ObjectHash(job)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating DB sync hash: %v", err)
	}

	requeue := true
	if instance.Status.DbSyncHash != dbSyncHash {
		requeue, err = EnsureJob(job, r)
		r.Log.Info("Running DB sync")
		if err != nil {
			return ctrl.Result{}, err
		} else if requeue {
			r.Log.Info("Waiting on DB sync")
			// Set KeystoneAPI instance as the owner and controller
			if err := controllerutil.SetControllerReference(instance, job, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	}
	// db sync completed... okay to store the hash to disable it
	if err := r.setDbSyncHash(instance, dbSyncHash); err != nil {
		return ctrl.Result{}, err
	}
	// delete the job
	requeue, err = DeleteJob(job, r)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Define a new Deployment object
	configMapHash, err := util.ObjectHash(configMap)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating config map hash: %v", err)
	}
	r.Log.Info("ConfigMapHash: ", "Data Hash:", configMapHash)
	deployment := keystone.Deployment(instance, instance.Name, configMapHash)
	deploymentHash, err := util.ObjectHash(deployment)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error deployment hash: %v", err)
	}
	r.Log.Info("DeploymentHash: ", "Deployment Hash:", deploymentHash)

	// Check if this Deployment already exists
	foundDeployment := &appsv1.Deployment{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		err = r.Client.Create(context.TODO(), deployment)
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
			err = r.Client.Update(context.TODO(), foundDeployment)
			if err != nil {
				return ctrl.Result{}, err
			}
			if err := r.setDeploymentHash(instance, deploymentHash); err != nil {
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
	var keystonePort int32
	keystonePort = 5000
	service := keystone.Service(instance, instance.Name, keystonePort)

	// Check if this Service already exists
	foundService := &corev1.Service{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		err = r.Client.Create(context.TODO(), service)
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
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, foundRoute)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new Route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
		err = r.Client.Create(context.TODO(), route)
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

	// Define a new BootStrap Job object

	// Look at the generated route to get the value for the initial endpoint
	// TODO (slagle): support/assume https
	var apiEndpoint string
	if !strings.HasPrefix(foundRoute.Spec.Host, "http") {
		apiEndpoint = fmt.Sprintf("http://%s", foundRoute.Spec.Host)
	} else {
		apiEndpoint = foundRoute.Spec.Host
	}
	r.setAPIEndpoint(instance, apiEndpoint)

	bootstrapJob := keystone.BootstrapJob(instance, instance.Name, apiEndpoint)
	bootstrapHash, err := util.ObjectHash(bootstrapJob)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating bootstrap hash: %v", err)
	}

	// Set KeystoneAPI instance as the owner and controller
	if instance.Status.BootstrapHash != bootstrapHash {

		requeue, err = EnsureJob(bootstrapJob, r)
		if err != nil {
			return ctrl.Result{}, err
		} else if requeue {

			if err := controllerutil.SetControllerReference(instance, bootstrapJob, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	}
	// bootstrap completed... okay to store the hash to disable it
	if err := r.setBootstrapHash(instance, bootstrapHash); err != nil {
		return ctrl.Result{}, err
	}

	// delete the job
	requeue, err = DeleteJob(bootstrapJob, r)
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
		Owns(&routev1.Route{}).
		Complete(r)
}

func (r *KeystoneAPIReconciler) setDbSyncHash(instance *keystonev1beta1.KeystoneAPI, hashStr string) error {

	if hashStr != instance.Status.DbSyncHash {
		instance.Status.DbSyncHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *KeystoneAPIReconciler) setBootstrapHash(instance *keystonev1beta1.KeystoneAPI, hashStr string) error {

	if hashStr != instance.Status.BootstrapHash {
		instance.Status.BootstrapHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *KeystoneAPIReconciler) setDeploymentHash(instance *keystonev1beta1.KeystoneAPI, hashStr string) error {

	if hashStr != instance.Status.DeploymentHash {
		instance.Status.DeploymentHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

// EnsureJob func
func EnsureJob(job *batchv1.Job, kr *KeystoneAPIReconciler) (bool, error) {
	// Check if this Job already exists
	foundJob := &batchv1.Job{}
	err := kr.Client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, foundJob)
	if err != nil && k8s_errors.IsNotFound(err) {
		kr.Log.Info("Creating a new Job", "Job.Namespace", job.Namespace, "Job.Name", job.Name)
		err = kr.Client.Create(context.TODO(), job)
		if err != nil {
			return false, err
		}
		return true, err
	} else if err != nil {
		kr.Log.Info("EnsureJob err")
		return true, err
	} else if foundJob != nil {
		kr.Log.Info("EnsureJob foundJob")
		if foundJob.Status.Active > 0 {
			kr.Log.Info("Job Status Active... requeuing")
			return true, err
		} else if foundJob.Status.Failed > 0 {
			kr.Log.Info("Job Status Failed")
			return true, k8s_errors.NewInternalError(errors.New("Job Failed. Check job logs"))
		} else if foundJob.Status.Succeeded > 0 {
			kr.Log.Info("Job Status Successful")
		} else {
			kr.Log.Info("Job Status incomplete... requeuing")
			return true, err
		}
	}
	return false, nil

}

// DeleteJob func
func DeleteJob(job *batchv1.Job, kr *KeystoneAPIReconciler) (bool, error) {

	// Check if this Job already exists
	foundJob := &batchv1.Job{}
	err := kr.Client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, foundJob)
	if err == nil {
		kr.Log.Info("Deleting Job", "Job.Namespace", job.Namespace, "Job.Name", job.Name)
		background := metav1.DeletePropagationBackground
		err = kr.Kclient.BatchV1().Jobs(job.Namespace).Delete(context.TODO(), job.Name, metav1.DeleteOptions{PropagationPolicy: &background})
		if err != nil {
			return false, err
		}
		return true, err
	}
	return false, nil
}

// setAPIEndpoint func
func (r *KeystoneAPIReconciler) setAPIEndpoint(instance *keystonev1beta1.KeystoneAPI, apiEndpoint string) error {

	if apiEndpoint != instance.Status.APIEndpoint {
		instance.Status.APIEndpoint = apiEndpoint
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}
