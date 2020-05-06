#!/bin/bash
set -x

# Get golint
go get -u golang.org/x/lint/golint

# Set to "" if lint errors should not fail the job (default golint behaviour)
# "-set_exit_status" otherwise
LINT_EXIT_STATUS="-set_exit_status"

BASE_DIR="$(dirname $0)"

pushd "${BASE_DIR}/.."

# Do not fail instantly when the error is found.
# This is useful as commit may have multiple lint errors.
ERR_CODE=0

for testdir in $(find . -type f -name "*.go" | rev | cut -d '/' -f2- | rev | sort | uniq | sed -e 's,^\./,,'); do
  pushd ${testdir}

    golint ${LINT_EXIT_STATUS}

    if [ $? -ne 0 ]; then
      ERR_CODE=1
    fi

  popd
done

popd

exit $ERR_CODE
