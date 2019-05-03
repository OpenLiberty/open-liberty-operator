#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

# Verify pre-req environment
command -v kubectl > /dev/null 2>&1 || { echo "kubectl pre-req is missing."; exit 1; }

# Create pre-requisite components
# For example, create pre-requisite PV/PVCs using yaml definition in current directory
[[ `dirname $0 | cut -c1` = '/' ]] && preinstallDir=`dirname $0`/ || preinstallDir=`pwd`/`dirname $0`/

echo $preinstallDir

# Process parameters notify of any unexpected
while test $# -gt 0; do
	[[ $1 =~ ^-c|--chartrelease$ ]] && { chartRelease="$2"; shift 2; continue; };
    echo "Parameter not recognized: $1, ignored"
    shift
done
: "${chartRelease:="default"}"

echo "
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: ${chartRelease}-test-ingress
spec:
  backend:
    serviceName: testsvc
    servicePort: 80
" > "$preinstallDir/ingress.yaml"

if [ "$CV_TEST_PROD" == "ics" ]
then
	ingress_ip="1.1.1.1"
else
  kubectl create -f "$preinstallDir/ingress.yaml"
  i=0
  ingress_ip=$(kubectl get ing ${chartRelease}-test-ingress -o jsonpath="{.status.loadBalancer.ingress[0].ip}")
  until [ -n "$ingress_ip" ]; do
      printf '.'
      ingress_ip=$(kubectl get ing ${chartRelease}-test-ingress -o jsonpath="{.status.loadBalancer.ingress[0].ip}")
      i=$((i+1))
      if [ $i -gt 10 ]

      then
        printf " Could not get IP of the ingress\n"
        kubectl get ing -o json
        exit 2
      fi
      sleep 15
  done
fi
sed -i.bak "s/CV_RELEASE/$chartRelease/g; s/CV_HOST/$ingress_ip.nip.io/g;" "$preinstallDir/../values.yaml"