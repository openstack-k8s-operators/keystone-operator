# OpenStack Keystone Operator

A Kubernetes Operator built using the [Operator Framework](https://github.com/operator-framework) for Go. The Operator provides a way to easily install and manage an OpenStack Keystone installation
on Kubernetes. This Operator was developed using [RDO](https://www.rdoproject.org/) containers for openStack.

# Creating resources

Create the following resources to start the operator with a single KeystoneAPI
instance:

<pre>
oc create -f deploy/crds/keystone.openstack.org\_keystoneapis\_crd.yaml
oc create -f deploy/role.yaml
oc create -f deploy/role\_binding.yaml
oc create -f deploy/service\_account.yaml
oc create -f deploy/operator.yaml
</pre>

# API Example

The Operator creates a custom KeystoneAPI resource that can be used to create Keystone API
instances within the cluster. Example CR to create an Keystone API in your cluster:

```yaml
apiVersion: keystone.openstack.org/v1
kind: KeystoneAPI
metadata:
  name: keystone
spec:
  adminPassword: foobar123
  containerImage: docker.io/tripleostein/centos-binary-keystone:current-tripleo
  replicas: 1
  databasePassword: foobar123
  databaseHostname: openstack-db-mariadb
  # used for keystone-manage bootstrap endpoints
  apiEndpoint: http://keystone-test.apps.test.dprince/
``` 

# Design
The current design takes care of the following:

- Creates keystone config files via config maps
- Creates a keystone deployment with the specified replicas
- Creates a keystone service
- Generates Fernet keys (TODO: rotate them, and bounce the APIs upon rotation)
- Keystone bootstrap, and db sync are executed automatically on install and updates
- ConfigMap is recreated on any changes KeystoneAPI object changes and the Deployment updated.

# Requirements

- A MariaDB database. TODO move the db-create logic to a mariadb operator for OpenStack services
