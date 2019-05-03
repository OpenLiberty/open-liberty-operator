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
  [[ $1 =~ ^-e|--ipAddress$ ]] && { ipAddress="$2"; shift 2; continue; };
	[[ $1 =~ ^-c|--chartrelease$ ]] && { chartRelease="$2"; shift 2; continue; };
	echo "Parameter not recognized: $1, ignored"
	shift
done
: "${chartRelease:="default"}"
: "${ipAddress:=""}"

# Setup and execute application test on installation
echo "Running application test"

echo "Testing Persist Transaction Logs"

# Get the NodePort service
full_name=$(kubectl get statefulset -l release=${chartRelease} -o jsonpath="{.items[0].metadata.name}")

if [ "$CV_TEST_PROD" == "ics" ]
then
	printf "This test is invalid in this environment..."
	exit 0
else
  if [ -z "$ipAddress" ]
  then
    node_ip=$(kubectl get nodes -o jsonpath="{.items[0].status.addresses[0].address}")
  else
    node_ip=${ipAddress}
  fi
fi

node_port=$(kubectl get services -l release=${chartRelease} -o jsonpath="{.items[?(@.spec.type==\"NodePort\")].spec.ports[0].nodePort}")
nodeport_url=https://$node_ip:$node_port
printf "Found ${full_name} endpoint: ${nodeport_url}\n"

# Setup test
curl -k --connect-timeout 180 --output /dev/null --silent --head --fail $nodeport_url/txlog?test=setup
sleep 10

# Kill the pod
kubectl delete pods ${full_name}-0

# Wait for the pod to be available again
printf 'Waiting for to the txlog app to be available'
i=0
result=$(curl -k --connect-timeout 180 --silent $nodeport_url/txlog?test=ready) || true
until [[ $result = *"COMPLETED SUCCESSFULLY"* ]]; do
  printf '.'
  result=$(curl -k --connect-timeout 180 --silent $nodeport_url/txlog?test=ready) || true
  i=$((i+1))
  if [ $i -gt 10 ]
  then
    printf "\nFAILED: '$nodeport_url/txlog?test=ready' NOT available\n"
    exit 1
  fi
  sleep 15
done

printf '\nChecking the test results\n'
i=1
result=$(curl -k --connect-timeout 180 --silent $nodeport_url/txlog?test=check) || true
until [[ $result = *"COMPLETED SUCCESSFULLY"* ]]; do
  result=$(curl -k --connect-timeout 180 --silent $nodeport_url/txlog?test=check) || true
  printf "Check $i of 10:\n$result\n"
  i=$((i+1))
  if [ $i -gt 10 ]
  then
    printf "\nFAILED: '$nodeport_url/txlog?test=check' NOT available\n"
    exit 1
  fi
  sleep 15
done

echo "SUCCESS - Persist Transaction Logs test passed."