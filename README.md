# keystone-operator

A Kubernetes Operator built using the [Operator Framework](https://github.com/operator-framework) for Go. The Operator provides a way to easily install and manage an OpenStack Keystone installation
on Kubernetes. This Operator was developed using [TripleO](https://opendev.org/openstack/tripleo-common/src/branch/master/container-images/tripleo_containers.yaml) containers for OpenStack.

# Deployment

The operator is intended to be deployed via OLM [Operator Lifecycle Manager](https://github.com/operator-framework/operator-lifecycle-manager)

# API Example

The Operator creates a custom KeystoneAPI resource that can be used to create Keystone API
instances within the cluster. Example CR to create an Keystone API in your cluster:

```yaml
apiVersion: keystone.openstack.org/v1beta1
kind: KeystoneAPI
metadata:
  name: keystone
spec:
  containerImage: quay.io/tripleowallabycentos9/openstack-keystone:current-tripleo
  replicas: 1
  secret: keystone-secret
```

# Design
The current design takes care of the following:

- Creates keystone config files via config maps
- Creates a keystone deployment with the specified replicas
- Creates a keystone service
- Generates Fernet keys (TODO: rotate them, and bounce the APIs upon rotation)
- Keystone bootstrap, and db sync are executed automatically on install and updates
- ConfigMap is recreated on any changes KeystoneAPI object changes and the Deployment updated.

#test
