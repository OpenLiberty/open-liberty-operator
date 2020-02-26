# Open Liberty Operator

The Open Liberty Operator can be used to deploy and manage applications running on Open Liberty into [OKD](https://www.okd.io/) or [OpenShift](https://www.openshift.com/) clusters. You can also perform [Day-2 operations](#day-2-operations) such as gathering traces and dumps using the operator.

This documentation refers to the current master branch.  For documentation and samples of older releases, please check out the [main releases](https://github.com/OpenLiberty/open-liberty-operator/releases) page and navigate the corresponding tag.

## Operator installation

Use the instructions for one of the [releases](../deploy/releases) to install the operator into a Kubernetes cluster.

The Open Liberty Operator can be installed to:

- watch own namespace
- watch another namespace
- watch multiple namespaces
- watch all namespaces in the cluster

Appropriate cluster roles and bindings are required to watch another namespace, watch multiple namespaces or watch all namespaces in the cluster.

## Overview

The architecture of the Open Liberty Operator follows the basic controller pattern:  the Operator container with the controller is deployed into a Pod and listens for incoming resources with `Kind: OpenLibertyApplication`. Creating an `OpenLibertyApplication` custom resource (CR) triggers the Open Liberty Operator to create, update or delete Kubernetes resources needed by the application to run on your cluster.

In addition, Open Liberty Operator makes it easy to perform [Day-2 operations](#day-2-operations) on an Open Liberty server running inside a Pod as part of an `OpenLibertyApplication` instance:
- Gather server traces using resource `Kind: OpenLibertyTrace`
- Generate server dumps using resource `Kind: OpenLibertyDump`

## Configuration

### Custom Resource Definition (CRD)

Each instance of `OpenLibertyApplication` CR represents the application to be deployed on the cluster:

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-liberty-app
spec:
  stack: java-microprofile
  applicationImage: quay.io/my-repo/my-app:1.0
  service:
    type: ClusterIP
    port: 9080
  expose: true
  storage:
    size: 2Gi
    mountPath: "/logs"
```

The following table lists configurable parameters of the `OpenLibertyApplication` CRD. For complete OpenAPI v3 representation of these values please see [`OpenLibertyApplication` CRD](../deploy/crds/openliberty.io_openlibertyapplications_crd.yaml).

Each `OpenLibertyApplication` CR must specify `applicationImage` parameter. Specifying other parameters is optional.

| Parameter | Description |
|---|---|
| `applicationImage` | The absolute name of the image to be deployed, containing the registry and the tag. |
| `pullPolicy` | The policy used when pulling the image.  One of: `Always`, `Never`, and `IfNotPresent`. |
| `pullSecret` | If using a registry that requires authentication, the name of the secret containing credentials. |
| `version` | The current version of the application. Label `app.kubernetes.io/version` will be added to all resources when the version is defined. |
| `stack` | Optional. The name of the [Appsody application stack](https://github.com/appsody/stacks) that produced this application image. |
| `serviceAccountName` | The name of the OpenShift service account to be used during deployment. |
| `initContainers` | The list of [Init Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.14/#container-v1-core) definitions. |
| `extraContainers` | Additional containers to be added to the server pods |
| `architecture` | An array of architectures to be considered for deployment. Their position in the array indicates preference. |
| `service.port` | The port exposed by the container. |
| `service.type` | The Kubernetes [Service Type](https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types). |
| `service.annotations` | Annotations to be added to the service. |
| `service.provides.category` | Service binding type to be provided by this CR. At this time, the only allowed value is `openapi`. |
| `service.provides.protocol` | Protocol of the provided service. Defauts to `http`. |
| `service.provides.context` | Specifies context root of the service. |
| `service.provides.auth.username` | Optional value to specify username as [SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.14/#secretkeyselector-v1-core). |
| `service.provides.auth.password` | Optional value to specify password as [SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.14/#secretkeyselector-v1-core). |
| `service.consumes` | An array consisting of services to be consumed by the `OpenLibertyApplication`. |
| `service.consumes[].category` | The type of service binding to be consumed. At this time, the only allowed value is `openapi`. |
| `service.consumes[].name` | The name of the service to be consumed. If binding to an `OpenLibertyApplication`, then this would be the provider's CR name. |
| `service.consumes[].namespace` | The namespace of the service to be consumed. If binding to an `OpenLibertyApplication`, then this would be the provider's CR name. ||
| `service.consumes[].mountPath` | Optional field to specify which location in the pod, service binding secret should be mounted. If not specified, the secret keys would be injected as environment variables. |
| `createKnativeService`   | A boolean to toggle the creation of Knative resources and usage of Knative serving. |
| `expose`   | A boolean that toggles the external exposure of this deployment via a Route or a Knative Route resource.|
| `replicas` | The static number of desired replica pods that run simultaneously. |
| `autoscaling.maxReplicas` | Required field for autoscaling. Upper limit for the number of pods that can be set by the autoscaler. It cannot be lower than the minimum number of replicas. |
| `autoscaling.minReplicas`   | Lower limit for the number of pods that can be set by the autoscaler. |
| `autoscaling.targetCPUUtilizationPercentage`   | Target average CPU utilization (represented as a percentage of requested CPU) over all the pods. |
| `resourceConstraints.requests.cpu` | The minimum required CPU core. Specify integers, fractions (e.g. 0.5), or millicore values(e.g. 100m, where 100m is equivalent to .1 core). Required field for autoscaling. |
| `resourceConstraints.requests.memory` | The minimum memory in bytes. Specify integers with one of these suffixes: E, P, T, G, M, K, or power-of-two equivalents: Ei, Pi, Ti, Gi, Mi, Ki.|
| `resourceConstraints.limits.cpu` | The upper limit of CPU core. Specify integers, fractions (e.g. 0.5), or millicores values(e.g. 100m, where 100m is equivalent to .1 core). |
| `resourceConstraints.limits.memory` | The memory upper limit in bytes. Specify integers with suffixes: E, P, T, G, M, K, or power-of-two equivalents: Ei, Pi, Ti, Gi, Mi, Ki.|
| `env`   | An array of environment variables following the format of `{name, value}`, where value is a simple string. |
| `envFrom`   | An array of environment variables following the format of `{name, valueFrom}`, where `valueFrom` is YAML object containing a property named either `secretKeyRef` or `configMapKeyRef`, which in turn contain the properties `name` and `key`.|
| `readinessProbe`   | A YAML object configuring the [Kubernetes readiness probe](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-probes/#define-readiness-probes) that controls when the pod is ready to receive traffic. |
| `livenessProbe` | A YAML object configuring the [Kubernetes liveness probe](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-probes/#define-a-liveness-http-request) that controls when Kubernetes needs to restart the pod.|
| `volumes` | A YAML object representing a [pod volume](https://kubernetes.io/docs/concepts/storage/volumes). |
| `volumeMounts` | A YAML object representing a [pod volumeMount](https://kubernetes.io/docs/concepts/storage/volumes/). |
| `storage.size` | A convenient field to set the size of the persisted storage. Can be overridden by the `storage.volumeClaimTemplate` property. |
| `storage.mountPath` | The directory inside the container where this persisted storage will be bound to. |
| `storage.volumeClaimTemplate` | A YAML object representing a [volumeClaimTemplate](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#components) component of a `StatefulSet`. |
| `monitoring.labels` | Labels to set on [ServiceMonitor](https://github.com/coreos/prometheus-operator/blob/master/Documentation/api.md#servicemonitor). |
| `monitoring.endpoints` | A YAML snippet representing an array of [Endpoint](https://github.com/coreos/prometheus-operator/blob/master/Documentation/api.md#endpoint) component from ServiceMonitor. |
| `createAppDefinition`   | A boolean to toggle the automatic configuration of `OpenLibertyApplication`'s Kubernetes resources to allow creation of an application definition by [kAppNav](https://kappnav.io/). The default value is `true`. See [Application Navigator](#kubernetes-application-navigator-kappnav-support) for more information. |
| `serviceability.size` | A convenient field to request the size of the persisted storage to use for serviceability. Can be overridden by the `serviceability.volumeClaimName` property. See [Storage for serviceability](#storage-for-serviceability) for more information. |
| `serviceability.volumeClaimName` | The name of the [PersistentVolumeClaim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims) resource you created to be used for serviceability. Must be in the same namespace. |

### Basic usage

To deploy a Docker image containing an application running on Open Liberty to a Kubernetes environment you can use the following CR:

 ```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-liberty-app
spec:
  applicationImage: quay.io/my-repo/my-app:1.0
```

The `applicationImage` value must be defined in `OpenLibertyApplication` CR.

To get information on the deployed CR, use one of the following:

```sh
oc get olapp my-liberty-app
oc get olapps my-liberty-app
oc get openlibertyapplication my-liberty-app
```

### Service account

The operator can create a `ServiceAccount` resource when deploying an Open Liberty application. If `serviceAccountName` is not specified in a CR, the operator creates a service account with the same name as the CR (e.g. `my-liberty-app`).

Users can also specify `serviceAccountName` when they want to create a service account manually.

If applications require specific permissions but still want the operator to create a `ServiceAccount`, users can still manually create a role binding to bind a role to the service account created by the operator. To learn more about Role-based access control (RBAC), see Kubernetes [documentation](https://kubernetes.io/docs/reference/access-authn-authz/rbac/).

### Labels

By default, the operator adds the following labels into all resources created for an `OpenLibertyApplication` CR: `app.kubernetes.io/instance`, `app.kubernetes.io/name`, `app.kubernetes.io/managed-by`, `app.kubernetes.io/version` (when `version` is defined) and `stack.appsody.dev/id` (when `stack` is defined). You can set new labels in addition to the pre-existing ones or overwrite them, excluding the `app.kubernetes.io/instance` label. To set labels, specify them in your CR as key/value pairs.

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-liberty-app
  labels:
    my-label-key: my-label-value
spec:
  applicationImage: quay.io/my-repo/my-app:1.0
```

_After the initial deployment of `OpenLibertyApplication`, any changes to its labels would be applied only when one of the parameters from `spec` is updated._

### Annotations

To add new annotations into all resources created for an `OpenLibertyApplication`, specify them in your CR as key/value pairs. Annotations specified in CR would override any annotations specified on a resource, except for the annotations set on `Service` using `service.annotations`.

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-liberty-app
  annotations:
    my-annotation-key: my-annotation-value
spec:
  applicationImage: quay.io/my-repo/my-app:1.0
```

_After the initial deployment of `OpenLibertyApplication`, any changes to its annotations would be applied only when one of the parameters from `spec` is updated._

### Environment variables

You can set environment variables for your application container. To set environment variables, specify `env` and/or `envFrom` fields in your CR. The environment variables can come directly from key/value pairs, `ConfigMap`s or `Secret`s.

 ```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-liberty-app
spec:
  applicationImage: quay.io/my-repo/my-app:1.0
  env:
    - name: DB_PORT
      value: "6379"
    - name: DB_USERNAME
      valueFrom:
        secretKeyRef:
          name: db-credential
          key: adminUsername
    - name: DB_PASSWORD
      valueFrom:
        secretKeyRef:
          name: db-credential
          key: adminPassword
  envFrom:
    - configMapRef:
        name: env-configmap
    - secretRef:
        name: env-secrets
```

Use `envFrom` to define all data in a `ConfigMap` or a `Secret` as environment variables in a container. Keys from `ConfigMap` or `Secret` resources become environment variable name in your container.

#### Console Logging Variables

The Open Liberty Operator sets a number of environment variables related to console logging by default. The following table shows the variables and their corresponding values.

| Name                           | Value                        |
|--------------------------------|------------------------------|
| `WLP_LOGGING_CONSOLE_LOGLEVEL` | info                         |
| `WLP_LOGGING_CONSOLE_SOURCE`   | message,accessLog,ffdc,audit |
| `WLP_LOGGING_CONSOLE_FORMAT`   | json                         |

To override these default values with your own values, set them manually in your CR `env` list.

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-liberty-app
spec:
  applicationImage: quay.io/my-repo/my-app:1.0
  env:
    - name: WLP_LOGGING_CONSOLE_FORMAT
      value: "basic"
    - name: WLP_LOGGING_CONSOLE_SOURCE
      value: "messages,trace,accessLog"
    - name: WLP_LOGGING_CONSOLE_LOGLEVEL
      value: "error"
```


### High availability

Run multiple instances of your application for high availability using one of the following mechanisms: 
 - specify a static number of instances to run at all times using `replicas` parameter
 
    _OR_

 - configure auto-scaling to create (and delete) instances based on resource consumption using the `autoscaling` parameter.
      - Parameters `autoscaling.maxReplicas` and `resourceConstraints.requests.cpu` MUST be specified for auto-scaling.

### Persistence

Open Liberty Operator is capable of creating a `StatefulSet` and `PersistentVolumeClaim` for each pod if `storage` is specified in the `OpenLibertyApplication` CR.

Users also can provide mount points for their application. There are 2 ways to enable storage.

#### Basic storage

With the `OpenLibertyApplication` CR definition below the operator will create `PersistentVolumeClaim` called `pvc` with the size of `1Gi` and `ReadWriteOnce` access mode.

The operator will also create a volume mount for the `StatefulSet` mounting to `/data` folder. You can use `volumeMounts` field instead of `storage.mountPath` if you require to persist more then one folder.

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-liberty-app
spec:
  applicationImage: quay.io/my-repo/my-app:1.0
  storage:
    size: 1Gi
    mountPath: "/data"
```

#### Advanced storage

Open Liberty Operator allows users to provide entire `volumeClaimTemplate` for full control over automatically created `PersistentVolumeClaim`.

It is also possible to create multiple volume mount points for persistent volume using `volumeMounts` field as shown below. You can still use `storage.mountPath` if you require only a single mount point.

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-liberty-app
spec:
  applicationImage: quay.io/my-repo/my-app:1.0
  volumeMounts:
  - name: pvc
    mountPath: /data_1
    subPath: data_1
  - name: pvc
    mountPath: /data_2
    subPath: data_2
  storage:
    volumeClaimTemplate:
      metadata:
        name: pvc
      spec:
        accessModes:
        - "ReadWriteMany"
        storageClassName: 'glusterfs'
        resources:
          requests:
            storage: 1Gi
```

#### Storage for serviceability

The operator makes it easy to use a single storage for serviceability related operations, such as gatherig server traces or dumps (see [Day-2 Operations](#day-2-operations)). The single storage will be shared by all Pods of an `OpenLibertyApplication` instance. This way you don't need to mount a separate storage for each Pod. Your cluster must be configured to automatically bind the `PersistentVolumeClaim` (PVC) to a `PersistentVolume` or you must bind it manually.

You can specify the size of the persisted storage to request using `serviceability.size` parameter. The operator will automatically create a `PersistentVolumeClaim` with the specified size and access modes `ReadWriteMany` and `ReadWriteOnce`. It will be mounted at `/serviceability` inside all Pods of the `OpenLibertyApplication` instance.

You can also create the `PersistentVolumeClaim` yourself and specify its name using `serviceability.volumeClaimName` parameter. You must create it in the same namespace as the `OpenLibertyApplication` instance.

_Once a `PersistentVolumeClaim` is created by operator, its size can not be updated. It will not be deleted when serviceability is disabled or when the `OpenLibertyApplication` is deleted._

### Service binding

Open Liberty Operator can be used to help with service binding in a cluster. The operator creates a secret on behalf of the **provider** `OpenLibertyApplication` and injects the secret into pods of the **consumer** `OpenLibertyApplication` as either environment variable or mounted files. See [Design for Service Binding](https://docs.google.com/document/d/1riOX0iTnBBJpTKAHcQShYVMlgkaTNKb4m8fY7W1GqMA/edit) for more information on the architecture. At this time, the only supported service binding type is `openapi`.

The provider lists information about the REST API it provides:

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-provider
  namespace: pro-namespace
spec:
  applicationImage: quay.io/my-repo/my-provider:1.0
  service:
    port: 3000
    provides:
      category: openapi
      context: /my-context
      auth:
        password:
          name: my-secret
          key: password
        username:
          name: my-secret
          key: username
---
kind: Secret
apiVersion: v1
metadata:
  name: my-secret
  namespace: pro-namespace
data:
  password: bW9vb29vb28=
  username: dGhlbGF1Z2hpbmdjb3c=
type: Opaque
```

And the consumer lists the services it is intending to consume:

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-consumer
  namespace: con-namespace
spec:
  applicationImage: quay.io/my-repo/my-consumer:1.0
  expose: true
  service:
    port: 9080
    consumes:
    - category: openapi
      name: my-provider
      namespace: pro-namespace
      mountPath: /liberty
```

In the above example, the operator creates a secret named `pro-namespace-my-provider` and adds the following key-value pairs: `username`, `password`, `url`, `context`, `protocol` and `hostname`. The `url` value format is `<protocol>://<name>.<namespace>.svc.cluster.local:<port>/<context>`. Since the provider and the consumer are in two different namespaces, the operator copies the provider secret into consumer's namespace. The operator then mounts the provider secret into a directory with the pattern `<mountPath>/<namespace>/<service_name>` on application container within pods. In the above example, the secret will be serialized into `/liberty/pro-namespace/my-provider`, which means we will have a file for each key, where the filename is the key and the content is the key's value.

If consumer's CR does not include `mountPath`, the secret will be bound to environment variables with the pattern `<NAMESPACE>_<SERVICE-NAME>_<KEY>`, and the value of that env var is the key’s value. Due to syntax restrictions for Kubernetes environment variables, the string representing the namespace and the string representing the service name will have to be normalized by turning any non-`[azAZ09]` characters to become an underscore `(_)` character.

### Monitoring

Open Liberty Operator can create a `ServiceMonitor` resource to integrate with `Prometheus Operator`.

_This feature does not support integration with Knative Service. Prometheus Operator is required to use ServiceMonitor._

#### Basic monitoring specification

At minimum, a label needs to be provided that Prometheus expects to be set on `ServiceMonitor` objects. In this case, it is `apps-prometheus`.

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-liberty-app
spec:
  applicationImage: quay.io/my-repo/my-app:1.0
  monitoring:
    labels:
       apps-prometheus: ''
```

#### Advanced monitoring specification

For advanced scenarios, it is possible to set many `ServicerMonitor` settings such as authentication secret using [Prometheus Endpoint](https://github.com/coreos/prometheus-operator/blob/master/Documentation/api.md#endpoint)

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-liberty-app
spec:
  applicationImage: quay.io/my-repo/my-app:1.0
  monitoring:
    labels:
       app-prometheus: ''
    endpoints:
    - interval: '30s'
      basicAuth:
        username:
          key: username
          name: metrics-secret
        password:
          key: password
          name: metrics-secret
      tlsConfig:
        insecureSkipVerify: true
```

### Knative support

Open Liberty Operator can deploy serverless applications with [Knative](https://knative.dev/docs/) on a Kubernetes cluster. To achieve this, the operator creates a [Knative `Service`](https://github.com/knative/serving/blob/master/docs/spec/spec.md#service) resource which manages the whole life cycle of a workload.

To create Knative service, set `createKnativeService` to `true`:

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-liberty-app
spec:
  applicationImage: quay.io/my-repo/my-app:1.0
  createKnativeService: true
```

By setting this parameter, the operator creates a Knative service in the cluster and populates the resource with applicable `OpenLibertyApplication` fields. Also, it ensures non-Knative resources including Kubernetes `Service`, `Route`, `Deployment` and etc. are deleted.

The CRD fields which are used to populate the Knative service resource include `applicationImage`, `serviceAccountName`, `livenessProbe`, `readinessProbe`, `service.Port`, `volumes`, `volumeMounts`, `env`, `envFrom`, `pullSecret` and `pullPolicy`.

For more details on how to configure Knative for tasks such as enabling HTTPS connections and setting up a custom domain, checkout [Knative Documentation](https://knative.dev/docs/serving/).

_Autoscaling related fields in `OpenLibertyApplication` are not used to configure Knative Pod Autoscaler (KPA). To learn more about how to configure KPA, see [Configuring the Autoscaler](https://knative.dev/docs/serving/configuring-the-autoscaler/)._

_This feature is only available if you have Knative installed on your cluster._

### Exposing service externally

#### Non-Knative deployment

To expose your application externally, set `expose` to `true`:

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-liberty-app
spec:
  applicationImage: quay.io/my-repo/my-app:1.0
  expose: true
```

By setting this parameter, the operator creates an unsecured route based on your application service. Setting this parameter is same as running `oc expose service <service-name>`.

To create a secured HTTPS route, see [secured routes](https://docs.openshift.com/container-platform/3.11/architecture/networking/routes.html#secured-routes) for more information.

_This feature is only available if you are running on OKD or OpenShift._

#### Knative deployment

To expose your application as a Knative service externally, set `expose` to `true`:

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyApplication
metadata:
  name: my-liberty-app
spec:
  applicationImage: quay.io/my-repo/my-app:1.0
  createKnativeService: true
  expose: true
```

When `expose` is **not** set to `true`, the Knative service is labeled with `serving.knative.dev/visibility=cluster-local` which makes the Knative route to only be available on the cluster-local network (and not on the public Internet). However, if `expose` is set `true`, the Knative route would be accessible externally.

To configure secure HTTPS connections for your deployment, see [Configuring HTTPS with TLS certificates](https://knative.dev/docs/serving/using-a-tls-cert/) for more information.


### Kubernetes Application Navigator (kAppNav) support

By default, Open Liberty Operator configures the Kubernetes resources it generates to allow automatic creation of an application definition by [kAppNav](https://kappnav.io/), Kubernetes Application Navigator. You can easily view and manage the deployed resources that comprise your application using Application Navigator. You can disable auto-creation by setting `createAppDefinition` to `false`.

To join an existing application definition, disable auto-creation and set the label(s) needed to join the application on `OpenLibertyApplication` CR. See [Labels](#labels) section for more information.

_This feature is only available if you have kAppNav installed on your cluster. Auto creation of an application definition is not supported when Knative service is created_

### Troubleshooting

See the [troubleshooting guide](troubleshooting.md) for information on how to investigate and resolve deployment problems.

## Day-2 Operations

### Prerequisite 

 - The corresponding `OpenLibertyApplication` must already have [storage for serviceability](#storage-for-serviceability) configured in order to use the day-2 operations
 - The custom resource (CR) for a day-2 operation must be created in the same namespace as the `OpenLibertyApplication`
 
 
 ### Operation discovery
 
To allow auto-discovery of supported day-2 operations from external tools the following annotation has been added to the `OpenLibertyApplication` CRD:

```
  annotations:
    openliberty.io/day2operations: OpenLibertyTrace,OpenLibertyDump
```

Additionally, each day-2 operation CRD has the following annotation which illustrates the k8s `Kind`(s) the operation applies to:

```
  annotations:
    day2operation.openliberty.io/targetKinds: Pod
```

### Request server dump

You can request a snapshot of the server status including different types of server dumps, from an instance of Open Liberty server running inside a `Pod`, using Open Liberty Operator and `OpenLibertyDump` custom resource (CR). To use this feature the `OpenLibertyApplication` needs to have [storage for serviceability](#storage-for-serviceability) already configured. Also, the `OpenLibertyDump` CR must be created in the same namespace as the `Pod` to operate on.

The configurable parameters are:

| Parameter | Description |
|---|---|
| `podName` | The name of the Pod, which must be in the same namespace as the `OpenLibertyDump` CR. |
| `include` | Optional. List of memory dump types to request: _thread,heap,system_  |

Example including heap and thread dump:

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyDump
metadata:
  name: example-dump
spec:
  podName: Specify_Pod_Name_Here
  include: 
    - thread
    - heap
```

Dump file name will be added to OpenLibertyDump CR status and file will be stored in serviceability folder
using format such as /serviceability/NAMESPACE/POD_NAME/TIMESTAMP.zip

Once the dump has started, the CR can not be re-used to take more dumps. A new CR needs to be created for each server dump.

You can check the status of a dump operation using the `status` field inside the CR YAML. You can also run the command `oc get oldump -o wide` to see the status of all dump operations in the current namespace. 

Note:
_System dump might not work on certain Kubernetes versions, such as OpenShift 4.x_

### Request server traces

You can request server traces, from an instance of Open Liberty server running inside a `Pod`, using Open Liberty Operator and `OpenLibertyTrace` custom resource (CR). To use this feature the `OpenLibertyApplication` must already have [storage for serviceability](#storage-for-serviceability) configured. Also, the `OpenLibertyTrace` CR must be created in the same namespace as the `Pod` to operate on. 

The configurable parameters are:

| Parameter | Description |
|---|---|
| `podName` | The name of the Pod, which must be in the same namespace as the `OpenLibertyTrace` CR. |
| `traceSpecification` | The trace string to be used to selectively enable trace. The default is *=info. |
| `maxFileSize` | The maximum size (in MB) that a log file can reach before it is rolled. To disable this attribute, set the value to 0. By default, the value is 20. This setting does not apply to the `console.log` file. |
| `maxFiles` | If an enforced maximum file size exists, this setting is used to determine how many of each of the logs files are kept. This setting also applies to the number of exception logs that summarize exceptions that occurred on any particular day.  |
| `disable` | Set to _true_ to stop tracing. |

Example:

```yaml
apiVersion: openliberty.io/v1beta1
kind: OpenLibertyTrace
metadata:
  name: example-trace
spec:
  podName: Specify_Pod_Name_Here
  traceSpecification: "*=info:com.ibm.ws.webcontainer*=all"
  maxFileSize: 20
  maxFiles: 5
```
Generated trace files, along with _messages.log_ files, will be in the folder using format _/serviceability/NAMESPACE/POD_NAME/_

Once the trace has started, it can be stopped by setting the `disable` parameter to `true`. Deleting the CR will also stop the tracing. Changing the `podName` will first stop the tracing on the old Pod before enabling traces on the new Pod.

You can check the status of a trace operation using the `status` field inside the CR YAML. You can also run the command `oc get oltrace -o wide` to see the status of all trace operations in the current namespace. 

Note:
_The operator doesn't monitor the Pods. If the Pod is restarted or deleted after the trace is enabled, then the tracing wouldn't be automatically enabled when the Pod comes back up. In that case, the status of the trace operation may not correctly report whether the trace is enabled or not._

