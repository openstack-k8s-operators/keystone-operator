module github.com/openstack-k8s-operators/keystone-operator/api

go 1.24

require (
	github.com/go-logr/logr v1.4.3
	github.com/google/uuid v1.6.0
	github.com/gophercloud/gophercloud v1.14.1
	github.com/onsi/gomega v1.34.1
	github.com/openstack-k8s-operators/infra-operator/apis v0.6.1-0.20250922061840-2c06b5a7d10a
	github.com/openstack-k8s-operators/lib-common/modules/common v0.6.1-0.20250922082314-c83d83092a04
	github.com/openstack-k8s-operators/lib-common/modules/openstack v0.6.1-0.20250922082314-c83d83092a04
	github.com/openstack-k8s-operators/lib-common/modules/storage v0.6.1-0.20250922082314-c83d83092a04
	github.com/openstack-k8s-operators/lib-common/modules/test v0.6.1-0.20250922082314-c83d83092a04
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56
	k8s.io/api v0.31.12
	k8s.io/apimachinery v0.31.12
	k8s.io/utils v0.0.0-20250820121507-0af2bda4dd1d
	sigs.k8s.io/controller-runtime v0.19.7
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/emicklei/go-restful/v3 v3.12.0 // indirect
	github.com/evanphx/json-patch/v5 v5.9.0 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.7.7 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/openshift/api v3.9.0+incompatible // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.19.1 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.55.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/net v0.28.0 // indirect
	golang.org/x/oauth2 v0.21.0 // indirect
	golang.org/x/sys v0.23.0 // indirect
	golang.org/x/term v0.23.0 // indirect
	golang.org/x/text v0.17.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.4.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiextensions-apiserver v0.31.12 // indirect
	k8s.io/client-go v0.31.12 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20240322212309-b815d8309940 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)

// mschuppert: map to latest commit from release-4.18 tag
// must consistent within modules and service operators
replace github.com/openshift/api => github.com/openshift/api v0.0.0-20250711200046-c86d80652a9e //allow-merging
