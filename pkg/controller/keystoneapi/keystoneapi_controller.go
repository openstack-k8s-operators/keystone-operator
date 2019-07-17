package keystoneapi

import (
	"context"
	"errors"
	"reflect"
	"time"

	keystone "github.com/openstack-k8s-operators/keystone-operator/pkg/keystone"
        comv1 "github.com/openstack-k8s-operators/keystone-operator/pkg/apis/keystone/v1"
        appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	batchv1 "k8s.io/api/batch/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_keystoneapi")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

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

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
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
	}


	// Define a new Job object
        job := keystone.DbSyncJob(instance, instance.Name)

	// Set KeystoneApi instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, job, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if this Job already exists
	foundJob := &batchv1.Job{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, foundJob)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Job", "Job.Namespace", job.Namespace, "Job.Name", job.Name)
		err = r.client.Create(context.TODO(), job)
		if err != nil {
			return reconcile.Result{}, err
		}

                status := comv1.KeystoneApiStatus{
                        DbSyncVersion: "syncing",
                }
	        err = updateStatus(instance, r.client, status)
		if err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{RequeueAfter: time.Second * 5}, err
	} else if err != nil {
		return reconcile.Result{}, err
	} else {
                //FIXME replace with WaitForJobCompletion from pkg/operator/k8sutil/job.go in rook
                if foundJob.Status.Active > 0 {
	                reqLogger.Info("Dbsync Job Status Active... requeuing")
		        return reconcile.Result{RequeueAfter: time.Second * 5}, err
                }
                if foundJob.Status.Failed > 0 {
	                reqLogger.Info("DbSync Job Status Failed")
			return reconcile.Result{}, k8s_errors.NewInternalError(errors.New("DbSync Failed. Check job logs."))
                }
                if foundJob.Status.Succeeded > 0 {
	                reqLogger.Info("Job Status Successful")
                }
	}

	// Define a new Deployment object
        deployment := keystone.Deployment(instance, instance.Name)

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
                //FIXME replace with WaitForJobCompletion from pkg/operator/k8sutil/job.go in rook
                if foundDeployment.Status.ReadyReplicas == instance.Spec.Replicas {
			reqLogger.Info("Keystone Deployment Replicas running:")
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

	// Set KeystoneApi instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, bootstrapJob, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if this Job already exists
	foundBootStrapJob := &batchv1.Job{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: bootstrapJob.Name, Namespace: bootstrapJob.Namespace}, foundBootStrapJob)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Bootstrap Job", "Job.Namespace", bootstrapJob.Namespace, "Job.Name", bootstrapJob.Name)
		err = r.client.Create(context.TODO(), bootstrapJob)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{RequeueAfter: time.Second * 5}, err
	} else if err != nil {
		return reconcile.Result{}, err
	} else {
                //FIXME replace with WaitForJobCompletion from pkg/operator/k8sutil/bootstrapJob.go in rook
                if foundBootStrapJob.Status.Active > 0 {
	                reqLogger.Info("Bootstrap Job Status Active... requeuing")
		        return reconcile.Result{RequeueAfter: time.Second * 5}, err
                }
                if foundBootStrapJob.Status.Failed > 0 {
	                reqLogger.Info("Bootstrap Job Status Failed")
			return reconcile.Result{}, k8s_errors.NewInternalError(errors.New("Bootstrap Failed. Check job logs."))
                }
                if foundBootStrapJob.Status.Succeeded > 0 {
	                reqLogger.Info("Bootstrap Successful")
                }
	}
	return reconcile.Result{}, nil
}


func updateStatus(instance *comv1.KeystoneApi, client client.Client, status comv1.KeystoneApiStatus) error {

        if !reflect.DeepEqual(instance.Status, status) {
                instance.Status = status
		return client.Status().Update(context.TODO(), instance)
        }
    return nil
}
