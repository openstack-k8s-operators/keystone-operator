#!/bin/bash
set -x

export OS_TOKEN=$(cat /tmp/temporary_test_token)

output=$(oc exec -tn $NAMESPACE openstackclient -- env -u OS_CLOUD - OS_AUTH_URL=http://keystone-public.keystone-kuttl-tests.svc:5000 OS_AUTH_TYPE=token OS_TOKEN=$OS_TOKEN openstack endpoint list 2>&1)

filtered_output=$(echo "$output" | grep -i "Could not recognize Fernet token")

if echo "$filtered_output" | grep -q "Could not recognize Fernet token"; then
    exit 0
else
    exit 1
fi
