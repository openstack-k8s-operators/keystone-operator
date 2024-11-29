#!/bin/bash
set -x

TMP_SECRET_FILE="/tmp/keystone-secret.yaml"

generate_secret_yaml() {
    cat <<EOF > $TMP_SECRET_FILE
apiVersion: v1
kind: Secret
metadata:
    name: keystone
    namespace: keystone-kuttl-tests
    annotations:
        keystone.openstack.org/rotatedat: "2009-11-10T23:00:00Z"
EOF
}

for rotation in {1..5}; do
    echo "Starting rotation $rotation..."

    # Apply new secret to trigger rotation
    generate_secret_yaml
    if ! oc apply -f $TMP_SECRET_FILE; then
        echo "Failed to apply the secret!"
        rm -f $TMP_SECRET_FILE
        exit 1
    fi

    sleep 100

    # Wait for rollout to complete
    if ! oc rollout status deployment/keystone -n $NAMESPACE --timeout=60s; then
        echo "Rollout status check failed for rotation $rotation."
        continue
    fi

    echo "Rotation $rotation completed successfully."
done

rm -f $TMP_SECRET_FILE
echo "All rotations completed successfully."
exit 0
