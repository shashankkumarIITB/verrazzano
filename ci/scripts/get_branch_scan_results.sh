#!/usr/bin/env bash
#
# Copyright (c) 2021, Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
#

# NOTE: This script assumes that:
#
#   1) "docker login" has been done for the image registry
#   2) OCI credentials have been configured to allow the OCI CLI to fetch scan results from OCIR
#   3) "gh auth login" has been done to allow the github CLI to list releases and fetch release artifacts
#
SCRIPT_DIR=$(cd $(dirname "$0"); pwd -P)
RELEASE_SCRIPT_DIR=${SCRIPT_DIR}/../../release/scripts

if [ -z "$JENKINS_URL" ] || [ -z "$WORKSPACE" ] || [ -z "$OCI_OS_NAMESPACE" ] || [ -z "$OCI_OS_BUCKET" ] || [ -z "$CLEAN_BRANCH_NAME" ]; then
  echo "This script must only be called from Jenkins and requires a number of environment variables are set"
  exit 1
fi

# Hack to get the generated BOM from a release by pulling down the operator.yaml from the release artifacts
# and copying the BOM from the platform operator image
function get_bom_from_release() {
    local releaseTag=$1
    local outputFile=$2
    local tmpDir=$(mktemp -d)

    # Download the operator.yaml for the release and get the platform-operator image and tag
    gh release download ${releaseTag} -p 'operator.yaml' -D ${tmpDir}
    local image=$(grep "verrazzano-platform-operator:" ${tmpDir}/operator.yaml | grep "image:" -m 1 | xargs | cut -d' ' -f 2)

    # Create a container from the image and copy the BOM from the container
    local containerId=$(docker create ${image})
    docker cp ${containerId}:/verrazzano/platform-operator/verrazzano-bom.json ${outputFile}
    docker rm ${containerId}

    rm -fr ${tmpDir}
}

BOM_DIR=${WORKSPACE}/boms
mkdir -p ${BOM_DIR}
SCAN_RESULTS_BASE_DIR=${WORKSPACE}/scan-results
export SCAN_RESULTS_DIR=${SCAN_RESULTS_BASE_DIR}/latest
mkdir -p ${SCAN_RESULTS_DIR}

# Where the results are kept for the branch depend on what kind of branch it is and where the updated bom is stored:
#    master, release-* branches are regularly updated using the periodic pipelines only
#
#        The BOM for the latest results from the NORMAL workflows is here (master, release-*, special runs of branches):
#             ${CLEAN_BRANCH_NAME}-last-clean-periodic-test/last-ocir-pushed-verrazzano-bom.json
#
#        It is possible that someone ran a job which needed to specify that the tip of master or release-* push images to
#        OCIR. This does NOT happen normally, the only situation where this is done from a pipeline is when performing a
#        release that required a BUILD to be done (ie: when releasing something that was NOT pre-baked for some reason).
#        In these cases, the BOM is stored here:
#
#             ${CLEAN_BRANCH_NAME}-last-snapshot/last-ocir-pushed-verrazzano-bom.json
#
#    all other branches only will be pushed if explicitly set as a parameter. In these cases, the BOM is stored here:
#
#             ${CLEAN_BRANCH_NAME}/last-ocir-pushed-verrazzano-bom.json

# Get the last pushed BOMs for the branch
echo "Attempting to fetch BOM from object storage for branch: ${CLEAN_BRANCH_NAME}"
mkdir -p ${BOM_DIR}/${CLEAN_BRANCH_NAME}-last-clean-periodic-test
mkdir -p ${BOM_DIR}/${CLEAN_BRANCH_NAME}-last-snapshot
mkdir -p ${BOM_DIR}/${CLEAN_BRANCH_NAME}
export SCAN_BOM_PERIODIC_PATH=${CLEAN_BRANCH_NAME}-last-clean-periodic-test/last-ocir-pushed-verrazzano-bom.json
export SCAN_BOM_SNAPSHOT_PATH=${CLEAN_BRANCH_NAME}-last-snapshot/last-ocir-pushed-verrazzano-bom.json
export SCAN_BOM_FEATURE_PATH=${CLEAN_BRANCH_NAME}/last-ocir-pushed-verrazzano-bom.json
export SCAN_LAST_PERIODIC_BOM_FILE=${BOM_DIR}/${SCAN_BOM_PERIODIC_PATH}
export SCAN_LAST_SNAPSHOT_BOM_FILE=${BOM_DIR}/${SCAN_BOM_SNAPSHOT_PATH}
export SCAN_FEATURE_BOM_FILE=${BOM_DIR}/${SCAN_BOM_FEATURE_PATH}

# If there is a periodic BOM file for this branch, get those results
oci --region us-phoenix-1 os object get --namespace ${OCI_OS_NAMESPACE} -bn ${OCI_OS_BUCKET} --name ${SCAN_BOM_PERIODIC_PATH} --file ${SCAN_LAST_PERIODIC_BOM_FILE} 2> /dev/null
if [ $? -eq 0 ]; then
  echo "Fetching scan results for BOM: ${SCAN_LAST_PERIODIC_BOM_FILE}"
  export SCAN_RESULTS_DIR=${SCAN_RESULTS_BASE_DIR}/latest-periodic
  mkdir -p ${SCAN_RESULTS_DIR}
  ${RELEASE_SCRIPT_DIR}/get_ocir_scan_results.sh ${SCAN_LAST_PERIODIC_BOM_FILE}
else
  echo "INFO: Did not find a periodic BOM for ${CLEAN_BRANCH_NAME}"
  rm ${SCAN_LAST_PERIODIC_BOM_FILE} || true
fi

# If there is a snapshot BOM file for this branch, get those results
oci --region us-phoenix-1 os object get --namespace ${OCI_OS_NAMESPACE} -bn ${OCI_OS_BUCKET} --name ${SCAN_BOM_SNAPSHOT_PATH} --file ${SCAN_LAST_SNAPSHOT_BOM_FILE} 2> /dev/null
if [ $? -eq 0 ]; then
  echo "Fetching scan results for BOM: ${SCAN_LAST_SNAPSHOT_BOM_FILE}"
  export SCAN_RESULTS_DIR=${SCAN_RESULTS_BASE_DIR}/last-snapshot-possibly-old
  mkdir -p ${SCAN_RESULTS_DIR}
  ${RELEASE_SCRIPT_DIR}/get_ocir_scan_results.sh ${SCAN_LAST_SNAPSHOT_BOM_FILE}
else
  echo "INFO: Did not find a snapshot BOM for ${CLEAN_BRANCH_NAME}"
  rm ${SCAN_LAST_SNAPSHOT_BOM_FILE} || true
fi

# If this is a feature branch, get those results
if [[ "${CLEAN_BRANCH_NAME}" != "master" ]] && [[ "${CLEAN_BRANCH_NAME}" != release-* ]]; then
  oci --region us-phoenix-1 os object get --namespace ${OCI_OS_NAMESPACE} -bn ${OCI_OS_BUCKET} --name ${SCAN_BOM_FEATURE_PATH} --file ${SCAN_FEATURE_BOM_FILE} 2> /dev/null
  if [ $? -eq 0 ]; then
    echo "Fetching scan results for BOM: ${SCAN_FEATURE_BOM_FILE}"
    export SCAN_RESULTS_DIR=${SCAN_RESULTS_BASE_DIR}/feature-branch-latest
    mkdir -p ${SCAN_RESULTS_DIR}
    ${RELEASE_SCRIPT_DIR}/get_ocir_scan_results.sh ${SCAN_FEATURE_BOM_FILE}
  else
    echo "INFO: Did not find a feature BOM for ${CLEAN_BRANCH_NAME}"
    rm ${SCAN_FEATURE_BOM_FILE} || true
  fi
fi

if [[ "${CLEAN_BRANCH_NAME}" == release-* ]]; then
  # Get the list of matching releases, for example, on branch "release-1.0" the matching releases are "v1.0.0", "v1.0.1", ...
  echo "Attempting to fetch BOMs for released versions on branch: ${CLEAN_BRANCH_NAME}"

  MAJOR_MINOR_VERSION=${CLEAN_BRANCH_NAME:8}
  VERSIONS=$(gh release list | cut -f 3 | grep v${MAJOR_MINOR_VERSION})

  # For now get the results for all versions, at some point we should ignore versions that we no longer support
  for VERSION in ${VERSIONS}
  do
    echo "Fetching BOM for ${VERSION}"
    export SCAN_BOM_FILE=${BOM_DIR}/${VERSION}-bom.json
    get_bom_from_release ${VERSION} ${SCAN_BOM_FILE}

    export SCAN_RESULTS_DIR=${SCAN_RESULTS_BASE_DIR}/${VERSION}
    mkdir -p ${SCAN_RESULTS_DIR}

    echo "Fetching scan results for BOM: ${SCAN_BOM_FILE}"
    ${RELEASE_SCRIPT_DIR}/get_ocir_scan_results.sh ${SCAN_BOM_FILE}
  done
fi
