# Open Liberty Operator v0.3.0

## Changelog

All notable changes are documented in the [Changelog](/CHANGELOG.md#0.3.0).

## Installation

The Open Liberty Operator can be installed to:

- watch own namespace
- watch another namespace
- watch multiple namespaces
- watch all namespaces in the cluster

Appropriate cluster role and binding are required to watch another namespace, watch multiple namespaces or watch all namespaces.

---

1. Install Custom Resource Definitions (CRDs) for `OpenLibertyApplication` and day-2 operations `OpenLibertyTrace` and `OpenLibertyDump`. This needs to be done only ONCE per cluster:

    ```console
    kubectl apply -f https://raw.githubusercontent.com/OpenLiberty/open-liberty-operator/master/deploy/releases/0.3.0/openliberty-app-crd.yaml
    ```

2. Install the Open Liberty Operator:

    **Important: In Step 2.1, ensure that you replace  `<SPECIFY_OPERATOR_NAMESPACE_HERE>` and `<SPECIFY_WATCH_NAMESPACE_HERE>` with proper values:**

    2.1. Set operator namespace and the namespace to watch:

    - To watch all namespaces in the cluster, set `WATCH_NAMESPACE='""'`
    - To watch multiple namespaces in the cluster, set `WATCH_NAMESPACE` to a comma-separated list of namespaces e.g. `WATCH_NAMESPACE=my-liberty-ns-1,my-liberty-ns-2,my-liberty-ns-3`

    ```console
    OPERATOR_NAMESPACE=<SPECIFY_OPERATOR_NAMESPACE_HERE>
    WATCH_NAMESPACE=<SPECIFY_WATCH_NAMESPACE_HERE>
    ```

    2.2. _Optional_: Install cluster-level role-based access. This step can be skipped if the operator is only watching own namespace:
  
    ```console
    curl -L https://raw.githubusercontent.com/OpenLiberty/open-liberty-operator/master/deploy/releases/0.3.0/openliberty-app-cluster-rbac.yaml \
      | sed -e "s/OPEN_LIBERTY_OPERATOR_NAMESPACE/${OPERATOR_NAMESPACE}/" \
      | kubectl apply -f -
    ```

    2.3. Install the operator:

    ```console
    curl -L https://raw.githubusercontent.com/OpenLiberty/open-liberty-operator/master/deploy/releases/0.3.0/openliberty-app-operator.yaml \
      | sed -e "s/OPEN_LIBERTY_WATCH_NAMESPACE/${WATCH_NAMESPACE}/" \
      | kubectl apply -n ${OPERATOR_NAMESPACE} -f -
    ```

## Uninstallation

To uninstall the operator, run commands from Step 2.3 first and then Step 2.2 (if applicable), but after replacing `kubectl apply` with `kubectl delete`.

To delete the CRD, run command from Step 1, but after replacing `kubectl apply` with `kubectl delete`.

_Deleting the CRD will also delete all `OpenLibertyApplication` in the cluster_

## Current Limitations

- Knative support is limited. Values specified for `autoscaling`, `resources` and `replicas` parameters would not apply for Knative when enabled using `createKnativeService` parameter.
- The auto-creation of an application definition by kAppNav is not supported when Knative is enabled.
- Monitoring feature does not support integration with Knative Service. Prometheus Operator is required to use ServiceMonitor.
- After the initial deployment of `OpenLibertyApplication`, any changes to its labels would be applied only when one of the parameters from `spec` is updated.