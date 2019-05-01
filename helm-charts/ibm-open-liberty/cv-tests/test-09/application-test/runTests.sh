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

# Check if any ingress is deoployed for the relase
if [ "$(kubectl get ing -l release=${chartRelease} -o jsonpath="{.items}")" = "[]" ]; then
  echo "FAIL - Ingress is not avalable in the ${chartRelease} release."
  exit 1
fi

# Get the automatically created secret name
temp=$(kubectl get ing -l release=${chartRelease} -o jsonpath="{.items[0].metadata.labels.app}")
ingress_secret_name="$temp-tls"

# Check whether the secret is created
kubectl get secret $ingress_secret_name
if [ $? -eq 0 ]
then
	echo "Secret name matched"
else
	echo "Secret name does not match"
	kubectl get secret
	exit 3
fi

i=0
ingress_ip=$(kubectl get ing -l release=${chartRelease} -o jsonpath="{.items[0].status.loadBalancer.ingress[*]['hostname','ip']}")
until [ -n "$ingress_ip" ]; do
    printf '.'
    ingress_ip=$(kubectl get ing -l release=${chartRelease} -o jsonpath="{.items[0].status.loadBalancer.ingress[*]['hostname','ip']}")
    i=$((i+1))
    if [ $i -gt 10 ]
    then
      printf "Could not get IP of the ingress\n"
      kubectl get ing -o json
      exit 4
    fi
    sleep 15
done

# Hit the ingress endpoint
# NOTE: /$chartRelease is setup in the test's values.yaml -> .Values.ingress.path
ingress_url=https://$ingress_ip:3443/$chartRelease
printf "Checking if the ingress endpoint '$ingress_url' is available\n"
curl -k --connect-timeout 180 --output /dev/null --head --fail $ingress_url
if [ $? -ne 0 ]
then
	printf "Cannot reach ingress endpoint\n"
else
	printf "Ingress endpoint is available\n"
fi

# Check the functionality of the secret

printf "Checking if the tls is working properly\n"
curl $ingress_url -v -k > website_response 2> extra_stuff

cat extra_stuff | grep subject | awk -F': ' '{ print $2 }' > need


cat extra_stuff
tls_subject="$(cat need)"
cat need | grep CN | cut -d "=" -f 3 > needdd

check="$(cat needdd)"

echo "tls subject is $tls_subject"

if [ "$check" != "Kubernetes Ingress Controller Fake Certificate" ]
then
  printf "tls_subject does not match\n"
  exit 5
else
  printf "tls is working properly\n"
fi

rm website_response
rm extra_stuff
rm need
rm needdd