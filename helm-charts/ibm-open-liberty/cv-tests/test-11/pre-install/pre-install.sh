#!/bin/bash

# Pre-install script REQUIRED ONLY IF additional setup is required prior to
# helm install for this test path.
#
# For example, a secret is required for chart installation
# it will need to be created prior to helm install.
#
# Parameters :
#   -c <chartReleaseName>, the name of the release used to install the helm chart
#
# Pre-req environment: authenticated to cluster & kubectl cli install / setup complete

# Exit when failures occur (including unset variables)
set -o errexit
set -o nounset
set -o pipefail

# Verify pre-req environment
command -v kubectl > /dev/null 2>&1 || { echo "kubectl pre-req is missing."; exit 1; }

# Create pre-requisite components
# For example, create pre-requisite PV/PVCs using yaml definition in current directory
[[ `dirname $0 | cut -c1` = '/' ]] && preinstallDir=`dirname $0`/ || preinstallDir=`pwd`/`dirname $0`/

# Process parameters notify of any unexpected
while test $# -gt 0; do
	[[ $1 =~ ^-c|--chartrelease$ ]] && { chartRelease="$2"; shift 2; continue; };
    echo "Parameter not recognized: $1, ignored"
    shift
done
: "${chartRelease:="default"}" 

# Create temporary tls.key and tls.crt
openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout $preinstallDir/$chartRelease.key -out $preinstallDir/$chartRelease.crt -subj "/CN=ingress-test"

# Create temporary secrect.yaml file
echo "
apiVersion: v1
kind: Secret
metadata:
  name: ${chartRelease}-ing-tls
data:
  tls.crt: `base64 -w 0 $preinstallDir/$chartRelease.crt`
  tls.key: `base64 -w 0 $preinstallDir/$chartRelease.key`
" > $preinstallDir/secret.yaml

# Create secret for Object store server credential
kubectl create -f $preinstallDir/secret.yaml

sed -i.bak "s/CV_RELEASE/$chartRelease/g" "$preinstallDir/../values.yaml"