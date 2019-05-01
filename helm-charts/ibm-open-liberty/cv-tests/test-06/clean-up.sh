#!/bin/bash
#
# Clean-up script for Liberty:
# 1) Delete PVs
#    - PVC: Deleting Helm release wouldn't delete PVC created by the chart, since the PVC is already bound to a PV
#
# Pre-req environment: authenticated to cluster & kubectl cli install / setup complete

# Exit when failures occur (including unset variables)
set -o errexit
set -o nounset
set -o pipefail

[[ `dirname $0 | cut -c1` = '/' ]] && preinstallDir=`dirname $0`/ || preinstallDir=`pwd`/`dirname $0`/

# Process parameters notify of any unexpected
while test $# -gt 0; do
        [[ $1 =~ ^-c|--chartrelease$ ]] && { chartRelease="$2"; shift 2; continue; };
    echo "Parameter not recognized: $1, ignored"
    shift
done
: "${chartRelease:="default"}"
 
# Verify pre-req environment of kubectl exists
command -v kubectl > /dev/null 2>&1 || { echo "kubectl pre-req is missing."; exit 1; }

# Execute clean-up kubectl commands.
# Deleting PVC gets rid of the bound PV
kubectl delete pvc -l app=$chartRelease-ibm-websphere-li || true
