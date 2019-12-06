package keystoneapi

import (
	"context"
	"errors"
	logr "github.com/go-logr/logr"
	comv1 "github.com/openstack-k8s-operators/keystone-operator/pkg/apis/keystone/v1"
	keystone "github.com/openstack-k8s-operators/keystone-operator/pkg/keystone"
	util "github.com/openstack-k8s-operators/keystone-operator/pkg/util"
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
	"time"
)

var log = logf.Log.WithName("controller_keystoneapi")

// Add creates a new KeystoneApi Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileKeystoneApi{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("keystoneapi-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource KeystoneApi
	err = c.Watch(&source.Kind{Type: &comv1.KeystoneApi{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner KeystoneApi
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &comv1.KeystoneApi{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileKeystoneApi implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileKeystoneApi{}

// ReconcileKeystoneApi reconciles a KeystoneApi object
type ReconcileKeystoneApi struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a KeystoneApi object and makes changes based on the state read
// and what is in the KeystoneApi.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileKeystoneApi) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling KeystoneApi")

	// Fetch the KeystoneApi instance
	instance := &comv1.KeystoneApi{}
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
	if err := controllerutil.SetControllerReference(instance, secret, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	// Check if this Secret already exists
	foundSecret := &corev1.Secret{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, foundSecret)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Secret", "Secret.Namespace", secret.Namespace, "Job.Name", secret.Name)
		err = r.client.Create(context.TODO(), secret)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// ConfigMap
	configMap := keystone.ConfigMap(instance, instance.Name)
	if err := controllerutil.SetControllerReference(instance, configMap, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	// Check if this ConfigMap already exists
	foundConfigMap := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new ConfigMap", "ConfigMap.Namespace", configMap.Namespace, "Job.Name", configMap.Name)
		err = r.client.Create(context.TODO(), configMap)
		if err != nil {
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
	dbSyncHash := util.ObjectHash(job)

	requeue := true
	if instance.Status.DbSyncHash != dbSyncHash {
		// Set KeystoneApi instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, job, r.scheme); err != nil {
			return reconcile.Result{}, err
		}
		requeue, err = EnsureJob(job, r, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		} else if requeue {
			return reconcile.Result{RequeueAfter: time.Second * 5}, err
		}
	}
	// db sync completed... okay to store the hash to disable it
	r.setDbSyncHash(instance, dbSyncHash)
	// delete the job
	requeue, err = DeleteJob(job, r, reqLogger)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Define a new Deployment object
	configMapHash := util.ObjectHash(configMap)
	reqLogger.Info("ConfigMapHash: ", "Data Hash:", configMapHash)
	deployment := keystone.Deployment(instance, instance.Name, configMapHash)
	deploymentHash := util.ObjectHash(deployment)
	reqLogger.Info("DeploymentHash: ", "Deployment Hash:", deploymentHash)

	// Set KeystoneApi instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, deployment, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if this Deployment already exists
	foundDeployment := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		err = r.client.Create(context.TODO(), deployment)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Deployment created successfully - don't requeue
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
			r.setDeploymentHash(instance, deploymentHash)
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
	service := keystone.Service(instance, instance.Name)

	// Set KeystoneApi instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, service, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if this Service already exists
	foundService := &corev1.Service{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		err = r.client.Create(context.TODO(), service)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{RequeueAfter: time.Second * 5}, err
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// Define a new BootStrap Job object
	bootstrapJob := keystone.BootstrapJob(instance, instance.Name)
	bootstrapHash := util.ObjectHash(bootstrapJob)

	// Set KeystoneApi instance as the owner and controller
	if instance.Status.BootstrapHash != dbSyncHash {
		if err := controllerutil.SetControllerReference(instance, bootstrapJob, r.scheme); err != nil {
			return reconcile.Result{}, err
		}

		requeue, err = EnsureJob(bootstrapJob, r, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		} else if requeue {
			return reconcile.Result{RequeueAfter: time.Second * 5}, err
		}
	}
	// db sync completed... okay to store the hash to disable it
	r.setBootstrapHash(instance, bootstrapHash)
	// delete the job
	requeue, err = DeleteJob(bootstrapJob, r, reqLogger)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil

}

func (r *ReconcileKeystoneApi) setDbSyncHash(instance *comv1.KeystoneApi, hashStr string) error {

	if hashStr != instance.Status.DbSyncHash {
		instance.Status.DbSyncHash = hashStr
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *ReconcileKeystoneApi) setBootstrapHash(instance *comv1.KeystoneApi, hashStr string) error {

	if hashStr != instance.Status.BootstrapHash {
		instance.Status.BootstrapHash = hashStr
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *ReconcileKeystoneApi) setDeploymentHash(instance *comv1.KeystoneApi, hashStr string) error {

	if hashStr != instance.Status.DeploymentHash {
		instance.Status.DeploymentHash = hashStr
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func EnsureJob(job *batchv1.Job, kr *ReconcileKeystoneApi, reqLogger logr.Logger) (bool, error) {

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
		return false, err
	} else {
		if foundJob.Status.Active > 0 {
			reqLogger.Info("Job Status Active... requeuing")
			return true, err
		}
		if foundJob.Status.Failed > 0 {
			reqLogger.Info("Job Status Failed")
			return false, k8s_errors.NewInternalError(errors.New("Job Failed. Check job logs."))
		}
		if foundJob.Status.Succeeded > 0 {
			reqLogger.Info("Job Status Successful")
		}
	}
	return false, nil

}

func DeleteJob(job *batchv1.Job, kr *ReconcileKeystoneApi, reqLogger logr.Logger) (bool, error) {

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
