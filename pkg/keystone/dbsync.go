package keystone

import (
        comv1 "github.com/openstack-k8s-operators/keystone-operator/pkg/apis/keystone/v1"
        util "github.com/openstack-k8s-operators/keystone-operator/pkg/util"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type dbCreateOptions struct {
	DatabasePassword string
	DatabaseHostname string
}

func DbSyncJob(cr *comv1.KeystoneApi, cmName string) *batchv1.Job {

	opts := dbCreateOptions{cr.Spec.DatabasePassword, cr.Spec.DatabaseHostname}
	runAsUser := int64(0)

	labels := map[string]string{
		"app": "keystone-api",
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      "OnFailure",
					ServiceAccountName: "keystone-operator",
					Containers: []corev1.Container{
						{
							Name:  "keystone-db-sync",
							Image: cr.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: []corev1.EnvVar{
								{
									Name:  "KOLLA_CONFIG_STRATEGY",
									Value: "COPY_ALWAYS",
								},
								{
									Name:  "KOLLA_BOOTSTRAP",
									Value: "TRUE",
								},
							},
                                                        VolumeMounts: getVolumeMounts(),
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:    "keystone-db-drop",
							Image:   cr.Spec.MysqlContainerImage,
							Command: []string{"/bin/sh", "-c", `mysql -h openstack-db-mariadb -u admin -P 3306 -e "DROP DATABASE IF EXISTS keystone";`},
							Env: []corev1.EnvVar{
								{
									Name:  "MYSQL_PWD",
									Value: cr.Spec.AdminDatabasePassword,
								},
							},
						},
						{
							Name:    "keystone-db-create",
							Image:   "docker.io/tripleomaster/centos-binary-mariadb:current-tripleo",
							Command: []string{"/bin/sh", "-c", util.ExecuteTemplateFile("db_create.sh", &opts)},
							Env: []corev1.EnvVar{
								{
									Name:  "MYSQL_PWD",
									Value: cr.Spec.AdminDatabasePassword,
								},
							},
						},
					},
				},
			},
		},
	}
	job.Spec.Template.Spec.Volumes = getVolumes(cmName)
	return job
}
