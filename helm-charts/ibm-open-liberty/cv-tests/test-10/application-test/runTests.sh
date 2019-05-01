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

echo "Testing ingress:"

# Check if any ingress is deoployed for the relase
if [ "$(kubectl get ing -l release=${chartRelease} -o jsonpath="{.items}")" = "[]" ]; then 
  echo "FAIL - Ingress is not avalable in the ${chartRelease} release."
  exit 1
fi

[[ `dirname $0 | cut -c1` = '/' ]] && appDir=`dirname $0`/ || appDir=`pwd`/`dirname $0`/
ingress_ip=$(cat $appDir/../values.yaml | grep host | cut -d: -f 2 | sed 's/"//g')
endpoint=$(cat $appDir/../values.yaml | grep path | cut -d: -f 2 | sed 's/"//g')
echo $ingress_ip $endpoint

# Hit the ingress endpoint
newIP=$(echo -e ${ingress_ip} | sed -e 's/^[[:space:]]*//')
newEndpoint=$(echo -e ${endpoint} | sed -e 's/^[[:space:]]*//')
ingress_url=https://$newIP:3443$newEndpoint
printf "\nChecking if the ingress endpoint '$ingress_url' is available\n"
curl -k --connect-timeout 180 --output /dev/null --head --fail $ingress_url
_testResult=$?
if [ $_testResult -eq 0 ]; then
  echo "SUCCESS - Ingress endpoint is available"
else
  echo "FAIL - Could not reach ingress endpoint"
fi
exit $_testResult
