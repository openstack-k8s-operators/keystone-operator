package keystone

import (
	comv1 "github.com/openstack-k8s-operators/keystone-operator/pkg/apis/keystone/v1"
	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"k8s.io/apimachinery/pkg/util/yaml"
	"strings"
)

type schemaOptions struct {
	DatabaseHostname string
	SchemaName       string
	SchemaPassword   string
}

// SchemaObject func
func SchemaObject(cr *comv1.KeystoneAPI) (unstructured.Unstructured, error) {
	opts := schemaOptions{cr.Spec.DatabaseHostname, cr.Name, cr.Spec.DatabasePassword}

	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(util.ExecuteTemplateFile("mariadb_schema.yaml", &opts)), 4096)
	u := unstructured.Unstructured{}
	err := decoder.Decode(&u)
	u.SetNamespace(cr.Namespace)
	return u, err
}
