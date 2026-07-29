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

package keystone

import (
	"fmt"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/volume"
	"github.com/openstack-k8s-operators/lib-common/modules/users"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

// Deployment func
func Deployment(
	instance *keystonev1.KeystoneAPI,
	configHash string,
	labels map[string]string,
	annotations map[string]string,
	topology *topologyv1.Topology,
	federationFilenames []string,
	memcached *memcachedv1.Memcached,
	customConfigKeys []string,
) (*appsv1.Deployment, error) {

	livenessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds:      30,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
	}
	readinessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds:      30,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
	}

	args := []string{"-c", "/usr/sbin/httpd -DFOREGROUND"}

	//
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
	//
	livenessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: "/v3",
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(KeystonePublicPort)},
	}
	readinessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: "/v3",
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(KeystonePublicPort)},
	}

	if instance.Spec.TLS.API.Enabled(service.EndpointPublic) {
		livenessProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
		readinessProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
	}

	envVars := map[string]env.Setter{}
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

	// create Volume and VolumeMounts
	volumes := getVolumes(instance, instance.Spec.ExtraMounts, KeystonePropagation)
	volumeMounts := getVolumeMounts(instance.Spec.ExtraMounts, KeystonePropagation)

	volumes = append(volumes,
		volume.WritableDirVolume(volume.RunHttpdVolumeName),
		volume.WritableDirVolume(VarLogKeystoneVolumeName),
		volume.WritableDirVolume(volume.VarLogHttpdVolumeName),
	)

	// add CA cert if defined
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	// add Federation volumes and volume mounts
	if instance.Spec.FederatedRealmConfig != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "federation-realm-dir",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "federation-realm-dir",
			MountPath: FederationDefaultMountPath,
		})

		volumes = append(volumes, getFederationVolumes(federationFilenames)...)
		volumeMounts = append(volumeMounts, getFederationVolumeMounts(FederationDefaultMountPath, federationFilenames)...)
	}

	// add MTLS cert if defined
	if memcached.GetMemcachedMTLSSecret() != "" {
		volumes = append(volumes, memcached.CreateMTLSVolume())
		certMountPath := memcachedv1.CertPathDst
		keyMountPath := memcachedv1.KeyPathDst
		volumeMounts = append(volumeMounts, memcached.CreateMTLSVolumeMounts(&certMountPath, &keyMountPath)...)
	}

	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		if instance.Spec.TLS.API.Enabled(endpt) {
			var tlsEndptCfg tls.GenericService
			switch endpt {
			case service.EndpointPublic:
				tlsEndptCfg = instance.Spec.TLS.API.Public
			case service.EndpointInternal:
				tlsEndptCfg = instance.Spec.TLS.API.Internal
			}

			svc, err := tlsEndptCfg.ToService()
			if err != nil {
				return nil, err
			}
			certMount := fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String())
			keyMount := fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String())
			svc.CertMount = &certMount
			svc.KeyMount = &keyMount
			volumes = append(volumes, svc.CreateVolume(endpt.String()))
			volumeMounts = append(volumeMounts, svc.CreateVolumeMounts(endpt.String())...)
		}
	}

	// mount custom httpd override configs directly to /etc/httpd/conf/
	for _, key := range customConfigKeys {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "config-data",
			MountPath: fmt.Sprintf("/etc/httpd/conf/%s", key),
			SubPath:   key,
			ReadOnly:  true,
		})
	}

	// httpd's master process now runs as the non-root keystone user, so it
	// needs the apache group to read RPM-shipped conf.d files (e.g.
	// mod_auth_openidc's auth_openidc.conf) that are baked into the image
	// with restrictive group ownership rather than volume-mounted.
	podSecurityContext := pod.RestrictivePodSecurityContext(users.KeystoneUID, users.KeystoneGID, users.ApacheGID)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:           instance.RbacResourceName(),
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext:              podSecurityContext,
					Volumes:                      volumes,
					Containers: []corev1.Container{
						{
							Name: ServiceName + "-api",
							Command: []string{
								"/bin/bash",
							},
							Args:            args,
							Image:           instance.Spec.ContainerImage,
							SecurityContext: pod.RestrictiveSecurityContext(users.KeystoneUID, users.KeystoneGID),
							Env:             env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:    volumeMounts,
							Resources:       instance.Spec.Resources,
							ReadinessProbe:  readinessProbe,
							LivenessProbe:   livenessProbe,
						},
					},
				},
			},
		},
	}

	if instance.Spec.NodeSelector != nil {
		deployment.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}

	if topology != nil {
		topology.ApplyTo(&deployment.Spec.Template)
	} else {
		// If possible two pods of the same service should not
		// run on the same worker node. If this is not possible
		// the get still created on the same worker node.
		deployment.Spec.Template.Spec.Affinity = affinity.DistributePods(
			common.AppSelector,
			[]string{
				ServiceName,
			},
			corev1.LabelHostname,
		)
	}

	return deployment, nil
}
