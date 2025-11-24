#!/bin/sh
set -euxo pipefail

oc wait --for=condition=ready pod openstackclient --timeout=30s -n $NAMESPACE

alias openstack="oc exec -tn $NAMESPACE openstackclient -- openstack"

export OS_TOKEN=$(openstack token issue -f value -c id)

echo $OS_TOKEN > /tmp/temporary_test_token
