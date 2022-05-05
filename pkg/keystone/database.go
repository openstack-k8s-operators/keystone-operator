package keystone

import (
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/lib-common/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"strings"

	"k8s.io/apimachinery/pkg/util/yaml"
)

type databaseOptions struct {
	DatabaseHostname string
	DatabaseName     string
	Secret           string
}

// DatabaseObject func
func DatabaseObject(cr *keystonev1beta1.KeystoneAPI) (unstructured.Unstructured, error) {
	opts := databaseOptions{cr.Spec.DatabaseHostname, cr.Name, cr.Spec.Secret}
	u := unstructured.Unstructured{}
	dbYAML, err := common.ExecuteTemplateFile("mariadb_database.yaml", &opts)
	if err != nil {
		return u, err
	}

	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(dbYAML), 4096)
	err = decoder.Decode(&u)
	u.SetNamespace(cr.Namespace)
	// set owner reference
	oref := metav1.NewControllerRef(cr, cr.GroupVersionKind())
	u.SetOwnerReferences([]metav1.OwnerReference{*oref})

	return u, err
}
