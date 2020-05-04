#!/bin/bash
set -ex

BASE_DIR="$(dirname $0)"

pushd "${BASE_DIR}/.."

for testdir in $(find . -type f -name "*.go" | rev | cut -d '/' -f2- | rev | sort | uniq | sed -e 's,^\./,,'); do
  pushd ${testdir}
    go vet
  popd
done

popd
