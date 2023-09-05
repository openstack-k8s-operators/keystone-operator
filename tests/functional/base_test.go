/*
Copyright 2023.

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

package functional_test

import (
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

func GetDefaultKeystoneAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance": "openstack",
		"replicas":         1,
		"secret":           SecretName,
	}
}

func GetDefaultKeystoneAPITLSSpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance": "openstack",
		"replicas":         1,
		"secret":           SecretName,
		"tls": map[string]interface{}{
			"service": map[string]interface{}{
				"secretName": CertName,
			},
			"ca": map[string]interface{}{
				"caSecretName": CaCertName,
			},
		},
	}
}

func CreateKeystoneAPICert(namespace string, name string) *corev1.Secret {
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"tls.crt": []byte("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURhRENDQXc2Z0F3SUJBZ0lSQU94OFdCL1JjQnhvRFFxV01Qc1NkTGd3Q2dZSUtvWkl6ajBFQXdJd0d6RVoKTUJjR0ExVUVBeE1RYlhrdGMyVnNabk5wWjI1bFpDMWpZVEFlRncweU16RXhNVFF3T1RNeE5EbGFGdzB5TXpFeApNVFF4TlRNeE5EbGFNRE14RmpBVUJnTlZCQW9URFdOc2RYTjBaWEl1Ykc5allXd3hHVEFYQmdOVkJBTVRFRzl3ClpXNXpkR0ZqYXkxbllXeGxjbUV3Z2dFaU1BMEdDU3FHU0liM0RRRUJBUVVBQTRJQkR3QXdnZ0VLQW9JQkFRRFYKbk9UU3JObjN6MGg0M3laUUJXTHUvYVVOcHFmWGdaSVB2Z3FiUHBRaTMvc0VnUDdFb2VtU2tCKy9mVk5WOW9aNApPUFdBOWxOUFZPa1d1OXI1TlNYMVJlYlRLUjZ3a1FWc2tiNHMrR2ZKZXBGQm1wZ2JrcmtabzJqa1UzT3k3cTA0Ci9tU0JKaEwrNFVWVGN4cTJUc0dlMmNxbTZ5cy9HdEZtaFphVC9vNTVOOG5ZdjA4OUl1WGFaQVVWQXJpOG1tK2MKaHNYckFtK05GYzVXQU10Zll0Njk0UVJxZVBEWFZZWnBzMUNjMnUvcUYzQjdxVndnYXFWT00rU2hDMFdvcjIxQwpwNk1EYWY2SnJtNzhaS2dNTlpJN0NXMjZHcFpYQW4yT0EwNUswbDhlYU9ROHNGaVIrRDBVR2ZDUHQxdW1iaG4wCnRtamd6a3V6bHBubXJqaHNhQ3dmQWdNQkFBR2pnZ0ZPTUlJQlNqQWRCZ05WSFNVRUZqQVVCZ2dyQmdFRkJRY0QKQVFZSUt3WUJCUVVIQXdJd0RBWURWUjBUQVFIL0JBSXdBREFmQmdOVkhTTUVHREFXZ0JRVzdnNmFQM0F6ZnIzYgpzOUptNUJBOWxHYi96ekNCK1FZRFZSMFJCSUh4TUlIdWdoZHZjR1Z1YzNSaFkyc3ViM0JsYm5OMFlXTnJMbk4yClk0SWxiM0JsYm5OMFlXTnJMbTl3Wlc1emRHRmpheTV6ZG1NdVkyeDFjM1JsY2k1c2IyTmhiSUlTS2k1dmNHVnUKYzNSaFkyc3RaMkZzWlhKaGdod3FMbTl3Wlc1emRHRmpheTFuWVd4bGNtRXViM0JsYm5OMFlXTnJnaUFxTG05dwpaVzV6ZEdGamF5MW5ZV3hsY21FdWIzQmxibk4wWVdOckxuTjJZNElvS2k1dmNHVnVjM1JoWTJzdFoyRnNaWEpoCkxtOXdaVzV6ZEdGamF5NXpkbU11WTJ4MWMzUmxjb0l1S2k1dmNHVnVjM1JoWTJzdFoyRnNaWEpoTG05d1pXNXoKZEdGamF5NXpkbU11WTJ4MWMzUmxjaTVzYjJOaGJEQUtCZ2dxaGtqT1BRUURBZ05JQURCRkFpQkN0Tk9leE9QSApnV0dPY2tZeTVjbnlZQkNmOHJRMTA2R3VFU1VJbUZvbWtnSWhBUGtrditoblNuYTZYYUtuaDM5Y20ybmp6QzNzClhwT0svWENOQUJJU2ZyVncKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="),
			"tls.key": []byte("LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JSUV2Z0lCQURBTkJna3Foa2lHOXcwQkFRRUZBQVNDQktnd2dnU2tBZ0VBQW9JQkFRRFZuT1RTck5uM3owaDQKM3laUUJXTHUvYVVOcHFmWGdaSVB2Z3FiUHBRaTMvc0VnUDdFb2VtU2tCKy9mVk5WOW9aNE9QV0E5bE5QVk9rVwp1OXI1TlNYMVJlYlRLUjZ3a1FWc2tiNHMrR2ZKZXBGQm1wZ2JrcmtabzJqa1UzT3k3cTA0L21TQkpoTCs0VVZUCmN4cTJUc0dlMmNxbTZ5cy9HdEZtaFphVC9vNTVOOG5ZdjA4OUl1WGFaQVVWQXJpOG1tK2Noc1hyQW0rTkZjNVcKQU10Zll0Njk0UVJxZVBEWFZZWnBzMUNjMnUvcUYzQjdxVndnYXFWT00rU2hDMFdvcjIxQ3A2TURhZjZKcm03OApaS2dNTlpJN0NXMjZHcFpYQW4yT0EwNUswbDhlYU9ROHNGaVIrRDBVR2ZDUHQxdW1iaG4wdG1qZ3prdXpscG5tCnJqaHNhQ3dmQWdNQkFBRUNnZ0VBRkZrOHh3RG1ScC85VkY1VmhQdnVYN3ZUMUVnUzV3bVZ3MkFrSElQS2tzUHAKWXBWekw0SUpBUWd2cmdzZlBDb2V4eWNONC9vVEY1U01HN2xMYzcvblhNVUs1d1Njd3M4ZHlDamVCK1NsNW1DQgpvOUU1T2k4dzZNdFRPNlRqZWFFWTZUdjZrUDd5Z2tzdFVuUzlqNjVTN1hIcnh4alI1dElFTHBMOU5CM2tycEZBClJ2blArenJVaEQyWHZNaVgxK1lqSFpReGpqR3pYVTN3L3MvbmJncmxPVXh6VHk0QmxINVFqdklHdVZPMjF6QWMKS2lTTW9aOEo2YW5RaWxYT0hWWEVDaThoeGcvcGpNNndUeG1MZjhOWUh2SnJTQUJlUUY2emdkTVBrVm9RR0MvNQpBSlRRM3FpSHZ1dUt5NHIwOFk2L1M1Z0huTmxwZWNqeUFoSzJaQlVMMlFLQmdRRDZDdmpENW9tSVhkK2xOZG9vCmpUL29wS1EwdXhpTkZsL3A1c2FsT0ZUUlRIRTRQMXJDR0dWOEhkdzlHdTgybVpseFowbCtmMFExT1hxZGQvSmYKMFFTZEdUWW1JZStNTWNDeFlSNmdDbzRIQVFqSlArcWVBM2J0RDVtQ01JdHF2N3JXZ0NJeWplODA2VW5TV0h4SwpXbDlla1pRZmFSMmVaVC9qZEZIL2VPRWdVd0tCZ1FEYXM3dXQ4Zzl1Z0Q2QVZUM3hUczVUemZiY0RFOVo5R3dSCndaSTRSNFNyOXMwa29pVXAySDBaTmpTL1RzODN4OXpKbzNTMHZWV3lITnFtQUVnOXdOOXpDaHVWQm5xQTZYMXIKcjNSWDZoVWR6NUhxeVFQZzF6ZVczb0lFQkk4bDBBNzc0RkhrYllieHpQQkhRbHAveVhkaW1HMmx2Ri9DU3YvKwpYcy9YRkJ2N2hRS0JnUUNmR1FRWWdrUFloUUtjdUp0TFdqVGo3bjZkSHI4TVpzUTRyQ0tSVmpxQndrWDRLRGV6CmNKcUNVdTJqNDlONXhsb2dFanh0Uk1VOXFJa2dVUVhqZWJlWnprVHFGb1c1aXA2MVBycWgwcFYwVjNBanZZdW4KWjBUd3FoQmZDa3hyYS91U0tJMlo1VDNqU04wei9pRjNuZkU0MXlDTXEvR3dxM1B2WWtBYWNldXRDUUtCZ1FDbQorK3FGMHJkem9KbVlOUDJabkprdkphaWh0UWgxWDRtUU9TTWlzNENhS0ZQVDc3VytjSng3dm9haHQxUENmR2lZCjBLUVFTQ3dCVmNTZ1VNRFgzY2IrdUMzOUtEZ3E2NXdtdDQxMmZyVm0wSkRTR205S29pakFtZDNkb1htRzNvaEMKU3JGY1gwQlVxU3lnekFuN1hlRTR0N2VvZnQ4Q28yODRVajRSTXpwMlhRS0JnRHkwT1dhQVBleU1ueVBwMyttcwo2RFBGVCtKcm5yRGtVckt2UGVkMll2MnZuMGUwc2gzSncyRHlONzU0M2U1REJWNnpNS0MvN211T1NVQ2ZxaWV0CnVWWEZTQmVxaC9mS2JlbmhkR1VHMU0vSk5xaWNTelJlK3R3T0dtWSt0aEkycFJJSXBuVDBpeXlsbk5GZTlWNUgKcmFkS3l5Ti9FVGMvMjNsbVVRYW51OHc2Ci0tLS0tRU5EIFBSSVZBVEUgS0VZLS0tLS0K"),
		},
	)
}
func CreateKeystoneAPICaCert(namespace string, name string) *corev1.Secret {
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"ca.crt": []byte("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJkakNDQVIyZ0F3SUJBZ0lSQU43WEZBeFBuZk1mTGdmdTJ0VzRXVXd3Q2dZSUtvWkl6ajBFQXdJd0d6RVoKTUJjR0ExVUVBeE1RYlhrdGMyVnNabk5wWjI1bFpDMWpZVEFlRncweU16RXdNREl4TkRJME16SmFGdzB5TXpFeQpNekV4TkRJME16SmFNQnN4R1RBWEJnTlZCQU1URUcxNUxYTmxiR1p6YVdkdVpXUXRZMkV3V1RBVEJnY3Foa2pPClBRSUJCZ2dxaGtqT1BRTUJCd05DQUFUMCtrVWZCaUlBTSsrU01maVpjaDVDeTh3WmpSbWpZd2ZZZWlhZGl3TU0KWXVIK1V0ckMwbkZqanN1ZzNjc3NXOUJTMVNFcUYrUjVQOGVBVHUwcEN0N3RvMEl3UURBT0JnTlZIUThCQWY4RQpCQU1DQXFRd0R3WURWUjBUQVFIL0JBVXdBd0VCL3pBZEJnTlZIUTRFRmdRVUZ1NE9tajl3TTM2OTI3UFNadVFRClBaUm0vODh3Q2dZSUtvWkl6ajBFQXdJRFJ3QXdSQUlnTUVOMW15WTBWNmV3R2hJaUJzbEZZN2Q0MFlsd3FudjEKcmswMGVBKys4Wm9DSUdKVVFTYlVQcjRsY0NpWTYzUDIzU2hWbUpHSzNXOHE3U1Z2eEZ0WGwvRjUKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="),
		},
	)
}

func CreateKeystoneAPI(name types.NamespacedName, spec map[string]interface{}) client.Object {

	raw := map[string]interface{}{
		"apiVersion": "keystone.openstack.org/v1beta1",
		"kind":       "KeystoneAPI",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetKeystoneAPI(name types.NamespacedName) *keystonev1.KeystoneAPI {
	instance := &keystonev1.KeystoneAPI{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func CreateKeystoneAPISecret(namespace string, name string) *corev1.Secret {
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"AdminPassword":            []byte("12345678"),
			"KeystoneDatabasePassword": []byte("12345678"),
		},
	)
}

func KeystoneConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetKeystoneAPI(name)
	return instance.Status.Conditions
}

func GetCronJob(name types.NamespacedName) *batchv1.CronJob {
	instance := &batchv1.CronJob{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}
