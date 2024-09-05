#!/bin/bash 

NAMESPACE=$1;
LTPA_SECRET_BASE_NAME=$2;
LTPA_SECRET_NAME=$3;
LTPA_CONFIG_BASE_NAME=$4;
LTPA_CONFIG_SECRET_NAME=$5;
LTPA_FILE_NAME=$6;
ENCODING_TYPE=$7;
PASSWORD_KEY_SECRET_NAME=$8;
ENCRYPTION_KEY_SHARING_ENABLED=$9;
LTPA_LABEL_KEY=${10};
LTPA_LABEL_VALUE=${11};
LTPA_JOB_REQUEST_NAME=${12};
KEY_FILE="/tmp/${LTPA_FILE_NAME}";
ENCODED_KEY_FILE="/tmp/${LTPA_FILE_NAME}-encoded";
ENCODED_PASSWORD_FILE="/tmp/${LTPA_FILE_NAME}-encoded-password";
NOT_FOUND_LOG_FILE="/tmp/not_found.out";
APISERVER="https://kubernetes.default.svc";
SERVICEACCOUNT="/var/run/secrets/kubernetes.io/serviceaccount";
TOKEN=$(cat ${SERVICEACCOUNT}/token);
CACERT="${SERVICEACCOUNT}/ca.crt";
RETRY_MESSAGE="Delete ConfigMap '$LTPA_JOB_REQUEST_NAME' to run this Job again.";
NETWORK_POLICY_MESSAGE="Is a NetworkPolicy blocking the pod's egress traffic? This pod must enable egress traffic to the API server and the cluster's DNS provider.";

function log() {
    echo "[$(basename ${0%.*})] $1"
}

function error() {
    echo "[$(basename ${0%.*})] ERROR: $1"
}

rm -f $NOT_FOUND_LOG_FILE;
curl --cacert ${CACERT} --header "Content-Type: application/json" --header "Authorization: Bearer ${TOKEN}" -X GET ${APISERVER}/api/v1/namespaces/${NAMESPACE}/secrets/${LTPA_SECRET_NAME} &> $NOT_FOUND_LOG_FILE;
NOT_FOUND_COUNT=$(cat $NOT_FOUND_LOG_FILE | grep -c "NotFound");

if [ $NOT_FOUND_COUNT -ne 0 ]; then 
    log "Could not validate that Secret '$LTPA_SECRET_NAME' exists in namespace '$NAMESPACE'."
    log "Trying again..."
    curl --cacert ${CACERT} --header "Content-Type: application/json" --header "Authorization: Bearer ${TOKEN}" -X GET ${APISERVER}/api/v1/namespaces/${NAMESPACE}/secrets/${LTPA_SECRET_NAME} &> /dev/null
    GET_SECRET_EXIT_CODE=$?
    if [[ "$GET_SECRET_EXIT_CODE" -ne 0 ]]; then
        error "cURL returned exit code $GET_SECRET_EXIT_CODE"
        if [[ "$GET_SECRET_EXIT_CODE" -eq 6 ]]; then
            log "Could not resolve hostname ${APISERVER}."
            log "${NETWORK_POLICY_MESSAGE}"
        elif [[ "$GET_SECRET_EXIT_CODE" -eq 28 ]]; then
            log "Connection timed out trying to reach ${APISERVER}."
            log "${NETWORK_POLICY_MESSAGE}"
        fi
    else
        error "Failed to parse response from the API server."
    fi
    log "$RETRY_MESSAGE"
    exit 0;
fi

rm -f $NOT_FOUND_LOG_FILE;
curl --cacert ${CACERT} --header "Content-Type: application/json" --header "Authorization: Bearer ${TOKEN}" -X GET ${APISERVER}/api/v1/namespaces/${NAMESPACE}/secrets/${PASSWORD_KEY_SECRET_NAME} &> $NOT_FOUND_LOG_FILE;
NOT_FOUND_COUNT=$(cat $NOT_FOUND_LOG_FILE | grep -c "NotFound");
LAST_ROTATION=$(curl --cacert ${CACERT} --header "Content-Type: application/json" --header "Authorization: Bearer ${TOKEN}" -X GET ${APISERVER}/api/v1/namespaces/${NAMESPACE}/secrets/${LTPA_SECRET_NAME} | grep -o '"lastRotation": "[^"]*' | grep -o '[^"]*$' | base64 -d);
PASSWORD=$(curl --cacert ${CACERT} --header "Content-Type: application/json" --header "Authorization: Bearer ${TOKEN}" -X GET ${APISERVER}/api/v1/namespaces/${NAMESPACE}/secrets/${LTPA_SECRET_NAME} | grep -o '"password": "[^"]*' | grep -o '[^"]*$' | base64 -d);

if [ "$ENCRYPTION_KEY_SHARING_ENABLED" == "true" ] && [ $NOT_FOUND_COUNT -eq 0 ]; then
    PASSWORD_KEY_LAST_ROTATION=$(curl --cacert ${CACERT} --header "Content-Type: application/json" --header "Authorization: Bearer ${TOKEN}" -X GET ${APISERVER}/api/v1/namespaces/${NAMESPACE}/secrets/${PASSWORD_KEY_SECRET_NAME} | grep -o '"lastRotation": "[^"]*' | grep -o '[^"]*$' | base64 -d);
    PASSWORD_KEY=$(curl --cacert ${CACERT} --header "Content-Type: application/json" --header "Authorization: Bearer ${TOKEN}" -X GET ${APISERVER}/api/v1/namespaces/${NAMESPACE}/secrets/${PASSWORD_KEY_SECRET_NAME} | grep -o '"passwordEncryptionKey": "[^"]*' | grep -o '[^"]*$' | base64 -d);
    ENCODED_PASSWORD=$(securityUtility encode --encoding=${ENCODING_TYPE} --key=${PASSWORD_KEY} ${PASSWORD});
    LTPA_ENCODED_PASSWORD="{\"apiVersion\": \"v1\", \"stringData\": {\"lastRotation\": \"$LAST_ROTATION\", \"password\": \"$ENCODED_PASSWORD\"}, \"kind\": \"Secret\",\"metadata\": {\"name\": \"$LTPA_CONFIG_SECRET_NAME\", \"passwordKeyLastRotation\": \"$PASSWORD_KEY_LAST_ROTATION\", \"namespace\": \"$NAMESPACE\",\"labels\": {\"app.kubernetes.io/name\": \"${LTPA_CONFIG_BASE_NAME}\", \"app.kubernetes.io/instance\": \"${LTPA_CONFIG_SECRET_NAME}\", \"$LTPA_LABEL_KEY\": \"$LTPA_LABEL_VALUE\"}},\"type\": \"Opaque\"}";
else
    SECRET_NAME="${LTPA_SECRET_NAME}-password"
    ENCODED_PASSWORD=$(securityUtility encode --encoding=${ENCODING_TYPE} ${PASSWORD});
    LTPA_ENCODED_PASSWORD="{\"apiVersion\": \"v1\", \"stringData\": {\"lastRotation\": \"$LAST_ROTATION\", \"password\": \"$ENCODED_PASSWORD\"}, \"kind\": \"Secret\",\"metadata\": {\"name\": \"$LTPA_CONFIG_SECRET_NAME\",\"namespace\": \"$NAMESPACE\",\"labels\": {\"app.kubernetes.io/name\": \"${LTPA_CONFIG_BASE_NAME}\", \"app.kubernetes.io/instance\": \"${LTPA_CONFIG_SECRET_NAME}\", \"$LTPA_LABEL_KEY\": \"$LTPA_LABEL_VALUE\"}},\"type\": \"Opaque\"}";
fi

echo $LTPA_ENCODED_PASSWORD > /tmp/tmp.password && mv /tmp/tmp.password ${ENCODED_PASSWORD_FILE};
CREATE_SECRET_STATUS_CODE=$(curl -s -o /dev/null -w "%{http_code}" --cacert ${CACERT} --header "Content-Type: application/json" --header "Authorization: Bearer ${TOKEN}" -X POST ${APISERVER}/api/v1/namespaces/${NAMESPACE}/secrets --data "@${ENCODED_PASSWORD_FILE}");
if [[ "$CREATE_SECRET_STATUS_CODE" == "201" ]]; then
    log "Successfully created Secret '$LTPA_CONFIG_SECRET_NAME' in namespace '$NAMESPACE'."
else
    error "Failed to create Secret '$LTPA_CONFIG_SECRET_NAME' in namespace '$NAMESPACE'. Received status code $CREATE_SECRET_STATUS_CODE."
    log "$RETRY_MESSAGE"
    curl --cacert ${CACERT} --header "Content-Type: application/json" --header "Authorization: Bearer ${TOKEN}" -X POST ${APISERVER}/api/v1/namespaces/${NAMESPACE}/secrets --data "@${ENCODED_PASSWORD_FILE}"
fi