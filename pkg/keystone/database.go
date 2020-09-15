package keystone

import (
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"k8s.io/apimachinery/pkg/util/yaml"
	"strings"
)

type databaseOptions struct {
	DatabaseHostname string
	DatabaseName     string
	Secret           string
}

// DatabaseObject func
func DatabaseObject(cr *keystonev1beta1.KeystoneAPI) (unstructured.Unstructured, error) {
	opts := databaseOptions{cr.Spec.DatabaseHostname, cr.Name, cr.Spec.Secret}

	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(util.ExecuteTemplateFile("mariadb_database.yaml", &opts)), 4096)
	u := unstructured.Unstructured{}
	err := decoder.Decode(&u)
	u.SetNamespace(cr.Namespace)
	return u, err
}
