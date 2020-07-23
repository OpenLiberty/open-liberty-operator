#!/usr/bin/env bash

readonly SERVICE_ACCOUNT="travis-tests"

# login_cluster : Download oc cli and use it to log into our persistent cluster
login_cluster(){
    # Install kubectl and oc
    curl -L https://github.com/openshift/origin/releases/download/v3.11.0/openshift-origin-client-tools-v3.11.0-0cbc58b-linux-64bit.tar.gz | tar xvz
    cd openshift-origin-clien*
    sudo mv oc kubectl /usr/local/bin/
    cd ..
    # Start a cluster and login
    oc login $OC_URL --token=$OC_TOKEN
    # Set variables for rest of script to use
    readonly DEFAULT_REGISTRY=$(oc get route "${REGISTRY_NAME}" -o jsonpath="{ .spec.host }" -n "${REGISTRY_NAMESPACE}")
    readonly BUILD_IMAGE=$DEFAULT_REGISTRY/openshift/operator:${TRAVIS_BUILD_NUMBER}
}

## cleanup : Delete generated resources that are not bound to a test namespace.
cleanup() {
    # Remove image related resources after the test has finished
    oc delete imagestream "operator:${TRAVIS_BUILD_NUMBER}" -n openshift
}

main() {
    parse_args "$@"
    echo "****** Logging into remote cluster..."
    login_cluster
    echo "****** Logging into private registry..."
    echo $(oc sa get-token travis-tests -n default) | docker login -u unused --password-stdin $DEFAULT_REGISTRY

    if [[ $? -ne 0 ]]; then
        echo "Failed to log into docker registry as ${SERVICE_ACCOUNT}, exiting..."
        exit 1
    fi

    echo "****** Building image"
    operator-sdk build "${BUILD_IMAGE}"
    echo "****** Pushing image into registry..."
    docker push "${BUILD_IMAGE}"

    if [[ $? -ne 0 ]]; then
        echo "Failed to push ref: ${BUILD_IMAGE} to docker registry, exiting..."
        exit 1
    fi

    echo "****** Starting e2e tests..."
    CLUSTER_ENV="ocp" operator-sdk test local github.com/OpenLiberty/open-liberty-operator/test/e2e --go-test-flags "-timeout 35m" --image $(oc registry info)/openshift/operator:$TRAVIS_BUILD_NUMBER --verbose
    result=$?
    echo "****** Cleaning up tests..."
    cleanup

    return $result
}

parse_args() {
    while [ $# -gt 0 ]; do
    case "$1" in
    --cluster-url)
      shift
      readonly OC_URL="${1}"
      ;;
    --cluster-token)
      shift
      readonly OC_TOKEN="${1}"
      ;;
    --registry-name)
      shift
      readonly REGISTRY_NAME="${1}"
      ;;
    --registry-namespace)
      shift
      readonly REGISTRY_NAMESPACE="${1}"
      ;;
    *)
      echo "Error: Invalid argument - $1"
      echo "$usage"
      exit 1
      ;;
    esac
    shift
  done
}


main "$@"
