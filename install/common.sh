#
# Copyright (c) 2020, Oracle Corporation and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
#
if [ -z "{SCRIPT_DIR}" ] ; then
    echo "SCRIPT_DIR not set"
    exit 1
fi

set -u

BUILD_DIR="${SCRIPT_DIR}/build"
LOGDIR="${BUILD_DIR}/logs"
LOGFILE="${LOGDIR}/$(basename $0).log"

if [ ! -d "${LOGDIR}" ] ; then
    mkdir -p "${LOGDIR}"
fi

echo "Output redirected to $LOGFILE"

CONSOLE_STDOUT=5
CONSOLE_STDERR=6

# Reset standard out and standard error streams
exec 5<&1
exec 6<&2

mkdir -p "$LOGDIR"
exec 1> "$LOGFILE" 2>&1

function consoleout()
{
    echo "$@"
    echo "$@" >&${CONSOLE_STDOUT}
}

function consoleerr()
{
    echo "$@"
    echo "$@" >&${CONSOLE_STDERR}
}

function fail()
{
    consoleerr ""
    consoleerr "$@"
    exit 1;
}

RES_COL=60
MOVE_TO_COL="echo -en \\033[${RES_COL}G"
SETCOLOR_SUCCESS="echo -en \\033[1;32m"
SETCOLOR_FAILURE="echo -en \\033[1;31m"
SETCOLOR_NORMAL="echo -en \\033[0;39m"

function echo_success()
{
  $MOVE_TO_COL
  echo -n "["
  $SETCOLOR_SUCCESS
  echo -n $"  OK  "
  $SETCOLOR_NORMAL
  echo -n "]"
  echo -ne "\r"
  return 0
}

function echo_failure()
{
  $MOVE_TO_COL
  echo -n "["
  $SETCOLOR_FAILURE
  echo -n $"FAILED"
  $SETCOLOR_NORMAL
  echo -n "]"
  echo -ne "\r"
  return 1
}

function echo_progress()
{
  local _progress=$1
  $MOVE_TO_COL
  echo -n "["
  $SETCOLOR_NORMAL
  echo -n $" $_progress "
  $SETCOLOR_NORMAL
  echo -n "]"
  echo -ne "\r"
  return 0
}

function spin()
{
  local spinner='\|/-'
  while :
  do
    for i in `seq 0 3`
    do
      echo_progress "${spinner:$i:1}"
      sleep .1
    done
  done
}

function action() {
  local STRING rc spin_pid
  local DISABLE_SPINNER=${DISABLE_SPINNER:-}

  STRING=$1
  consoleout -n "$STRING "


  if [ -z "${DISABLE_SPINNER}" ] ; then
    spin >&$CONSOLE_STDOUT &
    spin_pid=$!
    trap "kill -0 $spin_pid && kill $spin_pid" INT ERR EXIT
  fi

  shift
  "$@"
  rc=$?

  if [ -z "${DISABLE_SPINNER}" ] ; then
    kill $spin_pid
    wait $spin_pid
  fi

  if [ $rc -eq 0 ] ; then
      echo_success >&$CONSOLE_STDOUT
  else
      echo_failure >&$CONSOLE_STDOUT
  fi
  consoleout
  return $rc
}

function onerror()
{
    rv=$?
    if [ $rv -ne 0 ] ; then
        consoleerr ""
        consoleerr "An error occurred, please see ${LOGFILE} for details"
        echo ""
        echo "An error occurred, please see ${LOGFILE} for details"
    fi
    exit $rv
}

trap onerror ERR EXIT

KIND_CLUSTER_NAME=${KIND_CLUSTER_NAME:=verrazzano}
VERRAZZANO_DIR=${SCRIPT_DIR}/.verrazzano
KIND_KUBE_CONTEXT="kind-${KIND_CLUSTER_NAME}"
KIND_KUBECONFIG="${BUILD_DIR}/kind-kubeconfig"


CLUSTER_TYPE="${CLUSTER_TYPE:-}"
if [ "${CLUSTER_TYPE}" != "KIND" ] && [ "${CLUSTER_TYPE}" != "OKE" ] ; then
    fail "CLUSTER_TYPE environment variable must be set to KIND or OKE"
fi

VERRAZZANO_KUBECONFIG="${VERRAZZANO_KUBECONFIG:-}"
if [ "${CLUSTER_TYPE}" == "KIND" ] && [ -z "${VERRAZZANO_KUBECONFIG}" ] ; then
    VERRAZZANO_KUBECONFIG="${KIND_KUBECONFIG}"
    mkdir -p $(dirname $VERRAZZANO_KUBECONFIG)
else
    if [ -z "${VERRAZZANO_KUBECONFIG}" ] ; then
        fail "Environment variable VERRAZZANO_KUBECONFIG must be set and point to a valid kubernetes configuration file"
    fi
    if [ ! -f "${VERRAZZANO_KUBECONFIG}" ] ; then
        fail "Environment variable VERRAZZANO_KUBECONFIG points to file ${VERRAZZANO_KUBECONFIG} which does not exist"
    fi
fi
export KUBECONFIG="${VERRAZZANO_KUBECONFIG}"


command -v helm >/dev/null 2>&1 || {
    fail "helm is required but cannot be found on the path. Aborting.";
}
command -v kubectl >/dev/null 2>&1 || {
    fail "kubectl is required but cannot be found on the path. Aborting.";
}
command -v openssl >/dev/null 2>&1 || {
    fail "openssl is required but cannot be found on the path. Aborting.";
}
command -v jq >/dev/null 2>&1 || {
    fail "jq is required but cannot be found on the path. Aborting.";
}
