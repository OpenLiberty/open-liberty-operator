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
    oc login $CLUSTER_URL --token=$CLUSTER_TOKEN
    # Set variables for rest of script to use
    readonly DEFAULT_REGISTRY=$(oc get route docker-registry -o jsonpath="{ .spec.host }" -n default)
    readonly BUILD_IMAGE=$DEFAULT_REGISTRY/openshift/application-operator-$TRAVIS_BUILD_NUMBER:daily
}

## cleanup : Delete generated resources that are not bound to a test namespace.
cleanup() {
    # Remove image related resources after the test has finished
    oc delete imagestream "open-liberty-operator:${TRAVIS_BUILD_NUMBER}" -n openshift
}

main() {
    echo "****** Logging into remote cluster..."
    login_cluster
    echo "****** Logging into private registry..."
    docker login -u unused -p $(oc sa get-token "${SERVICE_ACCOUNT}" -n default) $DEFAULT_REGISTRY

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
    operator-sdk test local github.com/OpenLiberty/open-liberty-operator/test/e2e --go-test-flags "-timeout 35m" --image $(oc registry info)/openshift/open-liberty-operator:$TRAVIS_BUILD_NUMBER --verbose
    result=$?
    echo "****** Cleaning up tests..."
    cleanup

    return $result
}

main
