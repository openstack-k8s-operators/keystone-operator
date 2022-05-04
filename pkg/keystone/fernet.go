package keystone

import (
	"encoding/base64"

	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"

	"math/rand"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func generateFernetKey() string {
	rand.Seed(time.Now().UnixNano())
	data := make([]byte, 32)
	for i := 0; i < 32; i++ {
		data[i] = byte(rand.Intn(10))
	}
	return base64.StdEncoding.EncodeToString(data)
}

// FernetSecret func
func FernetSecret(cr *keystonev1beta1.KeystoneAPI, name string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "keystone-" + name,
			Namespace: cr.Namespace,
		},
		Type: "Opaque",
		StringData: map[string]string{
			"0": generateFernetKey(),
			"1": generateFernetKey(),
		},
	}
	return secret
}
