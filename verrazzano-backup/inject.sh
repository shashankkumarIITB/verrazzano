#!/bin/bash
#
# Copyright (c) 2022, Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
#

OPENSEARCH_BINARY_PATH="/usr/share/opensearch/data/verrazzano-bin"
COMPONENT_PATH="/usr/share/opensearch/data/verrazzano-bin/.component"
VZ_BINARY="verrazzano-backup"

function log () {
  echo $(date -u) $1
}

exit_trap () {
  if [ $? != 0 ]; then
    local lc="$BASH_COMMAND" rc=$?
    log  "Command [$lc] exited with code [$rc]"
  fi
}

trap exit_trap EXIT
set -e


function copy_opensearch () {
  log "Copy file '${VZ_BINARY}' to '$1'"
  cp -f ${VZ_BINARY} $1
}

log "Creating directory  ${OPENSEARCH_BINARY_PATH}"
mkdir -p ${OPENSEARCH_BINARY_PATH}
copy_opensearch ${OPENSEARCH_BINARY_PATH}

# Setup component flag. Will be used for more components later on
OPENSEARCH_FILE_PATH=$(df -h | grep -i /usr/share/opensearch/data | awk '{print $NF}')
if [ ${OPENSEARCH_FILE_PATH} != "" ]; then
  echo "opensearch" > ${COMPONENT_PATH}
fi

