#!/bin/bash 

NAMESPACE=$1

LTPA_SECRET_NAME=$2

LTPA_FILE_NAME=$3

ENCODING_TYPE=$4

KEY_FILE="/tmp/${LTPA_FILE_NAME}" 

ENCODED_KEY_FILE="/tmp/${LTPA_FILE_NAME}-encoded"

APISERVER=https://kubernetes.default.svc

SERVICEACCOUNT=/var/run/secrets/kubernetes.io/serviceaccount

TOKEN=$(cat ${SERVICEACCOUNT}/token)

CACERT=${SERVICEACCOUNT}/ca.crt

NOT_FOUND_COUNT=$(curl --cacert ${CACERT} --header "Content-Type: application/json" --header "Authorization: Bearer ${TOKEN}" -X GET ${APISERVER}/api/v1/namespaces/${NAMESPACE}/secrets/${LTPA_SECRET_NAME} | grep -c "NotFound")

if [ $NOT_FOUND_COUNT -eq 0 ]; then exit 0; fi

TIME_SINCE_EPOCH_SECONDS=$(date '+%s')

PASSWORD=$(openssl rand -base64 15)

securityUtility createLTPAKeys --file=${KEY_FILE} --password=${PASSWORD} --passwordEncoding=${ENCODING_TYPE}

cat ${KEY_FILE} | base64 > ${ENCODED_KEY_FILE}

ENCODED_PASSWORD=$(securityUtility encode --encoding=${ENCODING_TYPE} ${PASSWORD})

BEFORE_LTPA_KEYS="{\"apiVersion\": \"v1\", \"stringData\": {\"lastRotation\": \"$TIME_SINCE_EPOCH_SECONDS\", \"password\": \"$ENCODED_PASSWORD\"}, \"data\": {\"${LTPA_FILE_NAME}\": \""

AFTER_LTPA_KEYS="\"},\"kind\": \"Secret\",\"metadata\": {\"name\": \"$LTPA_SECRET_NAME\",\"namespace\": \"$NAMESPACE\"},\"type\": \"Opaque\"}"

echo $BEFORE_LTPA_KEYS | cat - ${ENCODED_KEY_FILE} > /tmp/tmp.keys && mv /tmp/tmp.keys ${ENCODED_KEY_FILE}

echo $AFTER_LTPA_KEYS >> ${ENCODED_KEY_FILE}

curl --cacert ${CACERT} --header "Content-Type: application/json" --header "Authorization: Bearer ${TOKEN}" -X POST ${APISERVER}/api/v1/namespaces/${NAMESPACE}/secrets --data "@${ENCODED_KEY_FILE}"