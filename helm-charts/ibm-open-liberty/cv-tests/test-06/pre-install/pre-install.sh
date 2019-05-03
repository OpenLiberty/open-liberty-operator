#!/bin/bash
#

# Pre-install script for Liberty to setup :
# 1) Create PVs
#    - PV for Liberty logs
#
# Pre-req: authentication to cluster & kubectl cli install / setup

# Exit when failures occur (including unset variables)
set -o errexit
set -o nounset
set -o pipefail

# Verify pre-req environment
command -v kubectl > /dev/null 2>&1 || { echo "kubectl pre-req is missing."; exit 1; }

# Create PVs
# - determine directory path of script which contains PV definition files
[[ `dirname $0 | cut -c1` = '/' ]] && preinstallDir=`dirname $0`/ || preinstallDir=`pwd`/`dirname $0`/

# Process parameters notify of any unexpected
while test $# -gt 0; do
    [[ $1 =~ ^-c|--chartrelease$ ]] && { chartRelease="$2"; shift 2; continue; };
    echo "Parameter not recognized: $1, ignored"
    shift
done
: "${chartRelease:="default"}" 
: "${CV_TEST_NFS_SERVER:="icp-nfs.rtp.raleigh.ibm.com"}"
: "${CV_TEST_NFS_PATH:="/mnt/nfs/data/open_liberty"}"

if [ "$CV_TEST_ARCHITECTURE" == "s390x" ]
then
    echo "Prepare a base directory for Persistent Volumes"
    nfsDir="${preinstallDir}../icp-nfs"
    mkdir -p "${nfsDir}"

    echo "Setting up nfs-common"
    sudo apt-get update
    sudo apt-get install -qqy nfs-common

    echo "Mounting NFS directory: ${nfsDir}"
    sudo mount -t nfs "${CV_TEST_NFS_SERVER}:${CV_TEST_NFS_PATH}" "${nfsDir}"

    echo "Removing directory: ${nfsDir}/${chartRelease}"
    sudo rm -rf "${nfsDir}/${chartRelease}"

    echo "Creating directory: ${nfsDir}/${chartRelease}"
    sudo mkdir -p "${nfsDir}/${chartRelease}"

    echo "Changing permissions for directory: ${nfsDir}/${chartRelease}"
    sudo chmod -R 777 "${nfsDir}/${chartRelease}"

    echo "Injecting values into liberty-logs-pv.yaml and creating PV's"
    sed 's/{{ .cv.release }}/'$chartRelease'/g' $preinstallDir/liberty-logs-pv.yaml | sed 's#{{ .cv.nfs.server }}#'$CV_TEST_NFS_SERVER'#g' | sed 's#{{ .cv.nfs.path }}#'$CV_TEST_NFS_PATH/${chartRelease}'#g'| kubectl create -f -
fi