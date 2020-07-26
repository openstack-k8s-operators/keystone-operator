package keystoneapi

import (
	"context"
	"errors"
	"fmt"
	logr "github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	comv1 "github.com/openstack-k8s-operators/keystone-operator/pkg/apis/keystone/v1"
	keystone "github.com/openstack-k8s-operators/keystone-operator/pkg/keystone"
	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strings"
	"time"
)

var log = logf.Log.WithName("controller_keystoneapi")

// Add creates a new KeystoneAPI Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileKeystoneAPI{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("keystoneapi-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource KeystoneAPI
	err = c.Watch(&source.Kind{Type: &comv1.KeystoneAPI{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner KeystoneAPI

	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &comv1.KeystoneAPI{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &comv1.KeystoneAPI{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &batchv1.Job{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &comv1.KeystoneAPI{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &routev1.Route{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &comv1.KeystoneAPI{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &comv1.KeystoneAPI{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileKeystoneAPI implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileKeystoneAPI{}

// ReconcileKeystoneAPI reconciles a KeystoneAPI object
type ReconcileKeystoneAPI struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a KeystoneAPI object and makes changes based on the state read
// and what is in the KeystoneAPI.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileKeystoneAPI) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling KeystoneAPI")

	// Fetch the KeystoneAPI instance
	instance := &comv1.KeystoneAPI{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Secret
	secret := keystone.FernetSecret(instance, instance.Name)
	// Check if this Secret already exists
	foundSecret := &corev1.Secret{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, foundSecret)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Secret", "Secret.Namespace", secret.Namespace, "Job.Name", secret.Name)
		err = r.client.Create(context.TODO(), secret)
		if err != nil {
			return reconcile.Result{}, err
		}
		if err := controllerutil.SetControllerReference(instance, secret, r.scheme); err != nil {
			return reconcile.Result{}, err
		}
	}

	// ConfigMap
	configMap := keystone.ConfigMap(instance, instance.Name)
	// Check if this ConfigMap already exists
	foundConfigMap := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new ConfigMap", "ConfigMap.Namespace", configMap.Namespace, "Job.Name", configMap.Name)
		err = r.client.Create(context.TODO(), configMap)
		if err != nil {
			return reconcile.Result{}, err
		}
		if err := controllerutil.SetControllerReference(instance, configMap, r.scheme); err != nil {
			return reconcile.Result{}, err
		}
	} else if !reflect.DeepEqual(configMap.Data, foundConfigMap.Data) {
		reqLogger.Info("Updating ConfigMap")
		foundConfigMap.Data = configMap.Data
		err = r.client.Update(context.TODO(), foundConfigMap)
		if err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{RequeueAfter: time.Second * 5}, err
	}

	// Define a new Job object
	job := keystone.DbSyncJob(instance, instance.Name)
	dbSyncHash, err := util.ObjectHash(job)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error calculating DB sync hash: %v", err)
	}

	requeue := true
	if instance.Status.DbSyncHash != dbSyncHash {
		requeue, err = EnsureJob(job, r, reqLogger)
		reqLogger.Info("Running DB sync")
		if err != nil {
			return reconcile.Result{}, err
		} else if requeue {
			reqLogger.Info("Waiting on DB sync")
			// Set KeystoneAPI instance as the owner and controller
			if err := controllerutil.SetControllerReference(instance, job, r.scheme); err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{RequeueAfter: time.Second * 5}, err
		}
	}
	// db sync completed... okay to store the hash to disable it
	if err := r.setDbSyncHash(instance, dbSyncHash); err != nil {
		return reconcile.Result{}, err
	}
	// delete the job
	requeue, err = DeleteJob(job, r, reqLogger)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Define a new Deployment object
	configMapHash, err := util.ObjectHash(configMap)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error calculating config map hash: %v", err)
	}
	reqLogger.Info("ConfigMapHash: ", "Data Hash:", configMapHash)
	deployment := keystone.Deployment(instance, instance.Name, configMapHash)
	deploymentHash, err := util.ObjectHash(deployment)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error deployment hash: %v", err)
	}
	reqLogger.Info("DeploymentHash: ", "Deployment Hash:", deploymentHash)

	// Check if this Deployment already exists
	foundDeployment := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		err = r.client.Create(context.TODO(), deployment)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Set KeystoneAPI instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, deployment, r.scheme); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{RequeueAfter: time.Second * 5}, err

	} else if err != nil {
		return reconcile.Result{}, err
	} else {

		if instance.Status.DeploymentHash != deploymentHash {
			reqLogger.Info("Deployment Updated")
			foundDeployment.Spec = deployment.Spec
			err = r.client.Update(context.TODO(), foundDeployment)
			if err != nil {
				return reconcile.Result{}, err
			}
			if err := r.setDeploymentHash(instance, deploymentHash); err != nil {
				return reconcile.Result{}, err
			}

			return reconcile.Result{RequeueAfter: time.Second * 10}, err
		}
		if foundDeployment.Status.ReadyReplicas == instance.Spec.Replicas {
			reqLogger.Info("Deployment Replicas running:", "Replicas", foundDeployment.Status.ReadyReplicas)
		} else {
			reqLogger.Info("Waiting on Keystone Deployment...")
			return reconcile.Result{RequeueAfter: time.Second * 5}, err
		}
	}

	// Create the service if none exists
	var keystonePort int32
	keystonePort = 5000
	service := keystone.Service(instance, instance.Name, keystonePort)

	// Check if this Service already exists
	foundService := &corev1.Service{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		err = r.client.Create(context.TODO(), service)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Set KeystoneAPI instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, service, r.scheme); err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{RequeueAfter: time.Second * 5}, err
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// Create the route if none exists
	route := keystone.Route(instance, instance.Name)

	// Check if this Route already exists
	foundRoute := &routev1.Route{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, foundRoute)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
		err = r.client.Create(context.TODO(), route)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Set Keystone instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, route, r.scheme); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{RequeueAfter: time.Second * 5}, err
	} else if err != nil {
		return reconcile.Result{}, err
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
		return reconcile.Result{}, fmt.Errorf("error calculating bootstrap hash: %v", err)
	}

	// Set KeystoneAPI instance as the owner and controller
	if instance.Status.BootstrapHash != bootstrapHash {

		requeue, err = EnsureJob(bootstrapJob, r, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		} else if requeue {

			if err := controllerutil.SetControllerReference(instance, bootstrapJob, r.scheme); err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{RequeueAfter: time.Second * 5}, err
		}
	}
	// bootstrap completed... okay to store the hash to disable it
	if err := r.setBootstrapHash(instance, bootstrapHash); err != nil {
		return reconcile.Result{}, err
	}

	// delete the job
	requeue, err = DeleteJob(bootstrapJob, r, reqLogger)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil

}

func (r *ReconcileKeystoneAPI) setDbSyncHash(instance *comv1.KeystoneAPI, hashStr string) error {

	if hashStr != instance.Status.DbSyncHash {
		instance.Status.DbSyncHash = hashStr
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *ReconcileKeystoneAPI) setBootstrapHash(instance *comv1.KeystoneAPI, hashStr string) error {

	if hashStr != instance.Status.BootstrapHash {
		instance.Status.BootstrapHash = hashStr
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *ReconcileKeystoneAPI) setDeploymentHash(instance *comv1.KeystoneAPI, hashStr string) error {

	if hashStr != instance.Status.DeploymentHash {
		instance.Status.DeploymentHash = hashStr
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

// EnsureJob func
func EnsureJob(job *batchv1.Job, kr *ReconcileKeystoneAPI, reqLogger logr.Logger) (bool, error) {
	// Check if this Job already exists
	foundJob := &batchv1.Job{}
	err := kr.client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, foundJob)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Job", "Job.Namespace", job.Namespace, "Job.Name", job.Name)
		err = kr.client.Create(context.TODO(), job)
		if err != nil {
			return false, err
		}
		return true, err
	} else if err != nil {
		reqLogger.Info("EnsureJob err")
		return true, err
	} else if foundJob != nil {
		reqLogger.Info("EnsureJob foundJob")
		if foundJob.Status.Active > 0 {
			reqLogger.Info("Job Status Active... requeuing")
			return true, err
		} else if foundJob.Status.Failed > 0 {
			reqLogger.Info("Job Status Failed")
			return true, k8s_errors.NewInternalError(errors.New("Job Failed. Check job logs"))
		} else if foundJob.Status.Succeeded > 0 {
			reqLogger.Info("Job Status Successful")
		} else {
			reqLogger.Info("Job Status incomplete... requeuing")
			return true, err
		}
	}
	return false, nil

}

// DeleteJob func
func DeleteJob(job *batchv1.Job, kr *ReconcileKeystoneAPI, reqLogger logr.Logger) (bool, error) {

	// Check if this Job already exists
	foundJob := &batchv1.Job{}
	err := kr.client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, foundJob)
	if err == nil {
		reqLogger.Info("Deleting Job", "Job.Namespace", job.Namespace, "Job.Name", job.Name)
		err = kr.client.Delete(context.TODO(), foundJob)
		if err != nil {
			return false, err
		}
		return true, err
	}
	return false, nil
}

// setAPIEndpoint func
func (r *ReconcileKeystoneAPI) setAPIEndpoint(instance *comv1.KeystoneAPI, apiEndpoint string) error {

	if apiEndpoint != instance.Status.APIEndpoint {
		instance.Status.APIEndpoint = apiEndpoint
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}
