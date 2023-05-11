#!/bin/bash
set -ex

oc delete validatingwebhookconfiguration/vkeystoneapi.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/mkeystoneapi.kb.io --ignore-not-found
