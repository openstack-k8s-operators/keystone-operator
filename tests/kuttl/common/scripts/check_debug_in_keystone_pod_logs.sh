#!/bin/sh
#
found=0
not_found=1
if [ "$1" == "--reverse" ];then
    # sometimes we want to check that there is no DEBUG logs
    found=1
    not_found=0
fi

pod=$(oc get pods -n openstack -l service=keystone -o name)
# in the case the logs have DEBUG lines, keep only one to avoid cluttering the
# test output
debug=$(oc logs -n openstack "$pod" | grep "DEBUG" | head -n 1)

if [ -n "$debug" ]; then
    exit $found
else
    exit $not_found
fi
