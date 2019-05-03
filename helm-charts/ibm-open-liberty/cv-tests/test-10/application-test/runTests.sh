#!/bin/bash

# Exit when failures occur (including unset variables)
set -o errexit
set -o nounset
set -o pipefail

# Verify pre-req environment
command -v kubectl > /dev/null 2>&1 || { echo "kubectl pre-req is missing."; exit 1; }

[[ `dirname $0 | cut -c1` = '/' ]] && appTestDir=`dirname $0`/ || appTestDir=`pwd`/`dirname $0`/

# Process parameters notify of any unexpected
while test $# -gt 0; do
    [[ $1 =~ ^-c|--chartrelease$ ]] && { chartRelease="$2"; shift 2; continue; };
    echo "Parameter not recognized: $1, ignored"
    shift
done
: "${chartRelease:="default"}"

if [ "$CV_TEST_PROD" == "ics" ]
then
	printf "This test is invalid in this environment..."
	exit 0
fi

echo "Testing ingress:"

# Check if any ingress is deoployed for the relase
if [ "$(kubectl get ing -l release=${chartRelease} -o jsonpath="{.items}")" = "[]" ]; then 
  echo "FAIL - Ingress is not avalable in the ${chartRelease} release."
  exit 1
fi

i=0
ingress_ip=$(kubectl get ing -l release=${chartRelease} -o jsonpath="{.items[0].status.loadBalancer.ingress[0].ip}")
until [ -n "$ingress_ip" ]; do
    printf '.'
    ingress_ip=$(kubectl get ing -l release=${chartRelease} -o jsonpath="{.items[0].status.loadBalancer.ingress[0].ip}")
    i=$((i+1))
    if [ $i -gt 10 ]
    then
      printf "Could not get IP of the ingress\n"
      kubectl get ing -o json
      exit 4
    fi
    sleep 15
done

nipIO=".nip.io"
ingress_url=https://${ingress_ip}${nipIO}/${chartRelease}
printf "\nChecking if the ingress endpoint '$ingress_url' is available\n"
curl -k --connect-timeout 180 --output /dev/null --head --fail $ingress_url
_testResult=$?
if [ $_testResult -eq 0 ]; then
  echo "SUCCESS - Ingress endpoint is available"
else
  echo "FAIL - Could not reach ingress endpoint"
fi
exit $_testResult