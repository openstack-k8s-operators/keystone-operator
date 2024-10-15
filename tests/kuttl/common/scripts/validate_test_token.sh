#!/bin/sh
set -euxo pipefail

seconds=1
while [ $seconds -le 30 ]; do
    rotatedat=$(oc get secret keystone -n $NAMESPACE -o jsonpath="{.metadata.annotations['keystone\.openstack\.org/rotatedat']}")
    if [ $rotatedat != "2009-11-10T23:00:00Z" ]; then
        break
    fi
    sleep 1
    seconds=$(( $seconds + 1 ))
done

sleep 20 # make sure a rollout started

oc rollout status deployment/keystone -n $NAMESPACE

export OS_TOKEN=$(cat /tmp/temporary_test_token)

alias openstack="oc exec -tn $NAMESPACE openstackclient -- env -u OS_CLOUD - OS_AUTH_URL=http://keystone-public.keystone-kuttl-tests.svc:5000 OS_AUTH_TYPE=token OS_TOKEN=$OS_TOKEN openstack"

if openstack endpoint list 2>&1 | grep "Failed to validate token"; then
    exit 1
else
    exit 0
fi
