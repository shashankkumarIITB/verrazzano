#!/usr/bin/env bash
#
# Copyright (c) 2021, Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
#

if [ -z "$JENKINS_URL" ] || [ -z "$WORKSPACE" ] || [ -z "$OCI_OS_NAMESPACE" ] || [ -z "$OCI_OS_BUCKET" ] || [ -z "$OCIR_SCAN_REGISTRY" ]  || [ -z "$OCIR_SCAN_REPOSITORY_PATH" ]; then
  echo "This script must only be called from Jenkins and requires a number of environment variables are set"
  exit 1
fi

if [ ! -f "${WORKSPACE}/verrazzano-bom.json" ]; then
  echo "There is no verrazzano-bom.json from this run, so we can't push anything to OCIR"
  exit 1
fi

# Periodic runs happen much more frequently than master promotions do, so we only conditionally do pushes to OCIR

# If we have a previous last-ocir-pushed-verrazzno-bom.json, then see if it matches the verrazzano-bom.json used
# to test with in this run. If they match, then we have already pushed the images for this verrazzano-bom.json
# into OCIR for Master periodic runs and we do not need to do that again.
# If they don't match, or if we didn't have one to compare, then we will proceed to push them to OCIR
if [ -f "${WORKSPACE}/last-ocir-pushed-verrazzano-bom.json" ]; then
  diff ${WORKSPACE}/last-ocir-pushed-verrazzano-bom.json ${WORKSPACE}/verrazzano-bom.json > /dev/null
  if [ $? -eq 0 ]; then
    echo "OCIR images for this verrazzano-bom.json have already been pushed to OCIR for scanning in a previous periodic run, skipping this step"
    exit 0
  fi
fi

# We should have image tar files created already in ${WORKSPACE}/tar-files
if [ ! -d "${WORKSPACE}/tar-files" ]; then
  echo "No tar files were found to push into OCIR"
  exit 1
fi

# This assumes that the docker login has happened, and that the OCI CLI has access as well with default profile

# This also currently assumes that the repository structure has been setup. That assumption will go away
# once we add in scripting which will ensure that the OCIR repositories for the images in the BOM are created
# and setup for scanning. Most of the time these already will exist and be setup, but if there is a new image
# or images it should get things setup for them.
#
# This will likely be done by enhancing the tests/e2e/config/scripts/create_ocir_repositories.sh script
# to handle our use cases as well.

# Push the images. NOTE: If a new image was added before we do the above "ensure" step, this may have the side
# effect of pushing that image to the root compartment rather than the desired sub-compartment (OCIR behaviour),
# and that new image will not be getting scanned until that is rectified (manually)

sh vz-registry-image-helper.sh -t $OCIR_SCAN_REGISTRY -r $OCIR_SCAN_REPOSITORY_PATH -l ${WORKSPACE}/tar-files

# Finally push the current verrazzano-bom.json up as the last-ocir-pushed-verrazzano-bom.json so we know those were the latest images
# pushed up. This is used above for avoiding pushing things multiple times for no reason, and it also is used when polling for results
# to know which images were last pushed for Master (which results are the latest)
oci --region us-phoenix-1 os object put --force --namespace ${OCI_OS_NAMESPACE} -bn ${OCI_OS_BUCKET} --name master-last-clean-periodic-test/last-ocir-pushed-verrazzano-bom.json --file ${WORKSPACE}/verrazzano-bom.json

# TBD: We could also save the list of repositories as well, that may save the polling job some work so it doesn't need to figure that out
# or simply just rely on the BOM there and compute from that.
