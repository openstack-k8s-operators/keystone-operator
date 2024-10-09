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
	"encoding/base64"

	"math/rand"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GenerateFernetKey -
func GenerateFernetKey() string {
	data := make([]byte, 32)
	for i := 0; i < 32; i++ {
		data[i] = byte(rand.Intn(10))
	}
	return base64.StdEncoding.EncodeToString(data)
}

const (
	// FernetRotationCommand -
	FernetRotationCommand = `
              echo $(date -u) Starting...
              case $MAX_ACTIVE_KEYS in
                ''|*[!0-9]*)
                  echo "MAX_ACTIVE_KEYS is not a number, exiting."
                  exit 1
                  ;;
                [01])
                  echo "MAX_ACTIVE_KEYS ($MAX_ACTIVE_KEYS) -lt 2, exiting."
                  exit 1
              esac

              cd /var/lib/fernet-keys
              mkdir /tmp/keys
              for file in FernetKeys[0-9]*;
              do
                cat "$file" > /tmp/keys/"${file#FernetKeys}"
              done

              cd /tmp/keys

              number_of_keys=$(ls -1 | wc -l)
              max_key=$(ls -1 | sort -n | tail -1)

              if [ $((number_of_keys - 1)) != $max_key ]; then
                echo "Corrupted FernetKeys secret, exiting."
                exit 1
              fi

              mv 0 $((max_key  + 1))
              dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64 > 0

              while [ -f "$MAX_ACTIVE_KEYS" ]; do
                i=2
                while [ -f "$i" ]; do
                  mv $i $((i-1))
                  i=$((i+1))
                done
              done

              echo '{"stringData": {' > /tmp/patch_file.json
              i=0
              while [ -f "$((i+1))" ]; do
                echo '"FernetKeys'$i'": "'$(cat $i)'",' >> /tmp/patch_file.json
                i=$((i+1))
              done
              echo '"FernetKeys'$i'": "'$(cat $i)'"' >> /tmp/patch_file.json
              echo '}}' >> /tmp/patch_file.json

              kubectl patch secret -n $NAMESPACE $SECRET_NAME \
                --patch-file=/tmp/patch_file.json
              echo $(date -u) $((i+1)) keys rotated.

              cd /var/lib/fernet-keys
              if [ -f "FernetKeys$MAX_ACTIVE_KEYS" ]; then
                echo '[' > /tmp/patch_file.json
                i=$((MAX_ACTIVE_KEYS-1))
                while [ -f "FernetKeys$((i+1))" ]; do
                  echo '{"op": "remove", "path": "/data/FernetKeys'$i'"},' \
                  >> /tmp/patch_file.json
                  i=$((i+1))
                done
                  echo '{"op": "remove", "path": "/data/FernetKeys'$i'"}' \
                  >> /tmp/patch_file.json
                echo ']' >> /tmp/patch_file.json

                kubectl patch secret -n $NAMESPACE $SECRET_NAME \
                   --type=json --patch-file=/tmp/patch_file.json
                echo $(date -u) $MAX_ACTIVE_KEYS through $i keys deleted.
              fi
`
)

// FernetCronJob func
func FernetCronJob(
	keystoneapiinstance *keystonev1.KeystoneAPI,
	labels map[string]string,
	annotations map[string]string,
) *batchv1.CronJob {
	instance := &keystonev1.KeystoneAPIFernet{KeystoneAPI: keystoneapiinstance}
	runAsUser := int64(0)
	suspend := false
	successfulJobsHistoryLimit := int32(3)
	failedJobsHistoryLimit := int32(1)

	args := []string{"-c", FernetRotationCommand}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["SECRET_NAME"] = env.SetValue(ServiceName)
	envVars["MAX_ACTIVE_KEYS"] = env.SetValue(
		instance.Spec.FernetMaxActiveKeys)

	backoffLimit := int32(0)
	parallelism := int32(1)
	completions := int32(1)

	// create Volume and VolumeMounts
	//volumes := getVolumes(instance)
	volumes := getVolumes(keystoneapiinstance)
	volumeMounts := getVolumeMounts()

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName + "-fernet-cronjob",
			Namespace: instance.Namespace,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   instance.Spec.FernetRotationSchedule,
			Suspend:                    &suspend,
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			SuccessfulJobsHistoryLimit: &successfulJobsHistoryLimit,
			FailedJobsHistoryLimit:     &failedJobsHistoryLimit,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: &backoffLimit,
					Parallelism:  &parallelism,
					Completions:  &completions,
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  ServiceName + "-fernet-job",
									Image: instance.Spec.FernetRotationContainerImage,
									Command: []string{
										"/bin/bash",
									},
									Args:         args,
									Env:          env.MergeEnvs([]corev1.EnvVar{}, envVars),
									VolumeMounts: volumeMounts,
									SecurityContext: &corev1.SecurityContext{
										RunAsUser: &runAsUser,
									},
								},
							},
							Volumes:            volumes,
							RestartPolicy:      corev1.RestartPolicyNever,
							ServiceAccountName: instance.RbacResourceName(),
						},
					},
				},
			},
		},
	}
	if instance.Spec.NodeSelector != nil && len(instance.Spec.NodeSelector) > 0 {
		cronjob.Spec.JobTemplate.Spec.Template.Spec.NodeSelector = instance.Spec.NodeSelector
	}

	return cronjob
}
