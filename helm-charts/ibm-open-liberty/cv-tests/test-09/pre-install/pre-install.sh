#!/bin/bash
#
# Pre-install script REQUIRED ONLY IF additional setup is required prior to
# helm install for this test path.  
#
# For example, if PersistantVolumes (PVs) are required for chart installation 
# they will need to be created prior to helm install.
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
	printf "This test is invalid in this environment..."
	exit 0
fi
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

sed -i.bak "s/CV_RELEASE/$chartRelease/g" "$preinstallDir/../values.yaml"