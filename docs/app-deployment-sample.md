# Deploying An Application

The following set of steps show how to deploy an application to a kubernetes environment using the Open Liberty's Helm based operator.

## Before you start:
Install the following:
* Git
* Docker.
* Kubectl.
* Kubernetes environment (OpenShift, Minikube, etc).

The sample application used here is modresorts-1.0.war, which is part of the app modernization GIT repository (https://github.com/IBM/appmod-resorts). Clone this repository:

`git clone https://github.com/IBM/appmod-resorts.git`

### 1. Package your application in a docker container and publish it.

a. Go the app modernization repository previously cloned

`cd appmod-resorts`

b. Create a docker file to install the application.

Dockerfile:

```
FROM open-liberty:javaee8
COPY --chown=1001:0 data/examples/modresorts-1.0.war /config/dropins
```

c. Build the docker image and publish it to a repository the kubernetes cluster has access to.
The following commands shows how to build a docker image and how to push it to docker hub:

* `docker build -t <my-repo>/app-modernization:v1.0.0 .`
* `docker push <my-repo>/app-modernization:v1.0.0`

### 2. Clone the open-liberty operator and deploy the needed resources.

* `git clone https://github.com/OpenLiberty/open-liberty-operator.git`
* `cd open-liberty-operator`
* `kubectl apply -f olm/open-liberty-crd.yaml`
* `kubectl apply -f deploy/service_account.yaml`
* `kubectl apply -f deploy/role.yaml`
* `kubectl apply -f deploy/role_binding.yaml`
* `kubectl apply -f deploy/operator.yaml`

### 3. Define pod security.

For instructions on how to define pod security (Pod Security Policies, Security Context Constraints) go the install security section [here](../README.md).
This step is not required if running locally via Minikube.

### 4. Deploy your application as an OpenLiberty custom resource instance.

a. Go to the Open Liberty Operator local respository you cloned on step 2.

`cd open-liberty-operator`

b. Create an Open Liberty custom resource yaml file. You can use deploy/crds/full_cr.yaml as an example. 
If you have a simple app as one being used here, and you just want to test your app without having to create any files or fully customize full_cr.yaml, all you need to do is to update the repository (repository: <my-repo>/app-modernization) and tag (tag: v1.0.0) entries under spec.image in full_cr.yaml. Note that for more complex applications and deployments, customization will be required.

The following yaml shows the minimum required spec entries for application deployment.

deploy/apps/appmod/v1.0.0/app-mod_cr.yaml:

```
apiVersion: openliberty.io/v1alpha1
kind: OpenLiberty
metadata:
  name: appmod
spec:
  image:
    repository: <my-repo>/app-modernization
    tag: v1.0.0
    pullPolicy: IfNotPresent
  replicaCount: 1
```

c. Deploy the OpenLiberty custom resource instance:

`kubectl apply -f deploy/apps/appmod/v1.0.0/app-mod_cr.yaml`

```
[ibmadmin]$ kubectl get pods
NAME                                       READY     STATUS    RESTARTS   AGE
appmod-dm3oidh88r9b965vd-66bd87cb4-c5x2t   1/1       Running   1          42m
```

Note that by default, a Service resource of type NodePort is created:

```
[ibmadmin]$ kubectl get service
NAME                       TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)                   AGE
appmod-dm3oidh88r9b965vd   NodePort    x.x.x.x     <none>        9443:30033/TCP            45m
```
```
[ibmadmin]$ kubectl describe service appmod-dm3oidh88r9b965vd
Name:                     appmod-dm3oidh88r9b965vd
Namespace:                default
Labels:                   app=appmod-dm3oidh88r9b965vd
                          chart=ibm-open-liberty-1.9.0
                          heritage=Tiller
                          release=appmod-dm3oidh88r9b965vd611pxday
Annotations:              <none>
Selector:                 app=appmod-dm3oidh88r9b965vd
Type:                     NodePort
IP:                       x.x.x.x
Port:                     https  9443/TCP
TargetPort:               9443/TCP
NodePort:                 https  30033/TCP
```

### 5. Validate that the application is active.

Using the node address and nodeport, validate if the application can be reached.
For example:

```
[ibmadmin]$ curl -kL https://x.x.x.x:30033/resorts
...
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <link rel="stylesheet" href="styles.css">
    <link href="https://fonts.googleapis.com/css?family=Poppins:300,400,500,600" rel="stylesheet">
    <title>MOD RESORTS (A Transformation Advisor sample application)</title>
</head>
...
```
If you are using Minikube issue the following commands to find the host and port to use:
```
kubectl get service
NAME                       TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)          AGE
appmod-66qn3vgdypep4qefg   NodePort    x.x.x.x   <none>        9443:30835/TCP   22m
```
```
$ minikube service appmod-66qn3vgdypep4qefg --url
http://x.x.x.x:30835
```
Note that Liberty has SSL enabled. Use HTTPS as opposed to HTTP.

```
$ curl -kL https://x.x.x.x:30835/resorts
...
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <link rel="stylesheet" href="styles.css">
    <link href="https://fonts.googleapis.com/css?family=Poppins:300,400,500,600" rel="stylesheet">
    <title>MOD RESORTS (A Transformation Advisor sample application)</title>
</head>
...
```

### 6. Cleanup.
* `kubectl delete -f deploy/apps/appmod/v1.0.0/app-mod_cr.yaml`
* `kubectl delete -f deploy/operator.yaml`
* `kubectl delete -f deploy/role_binding.yaml`
* `kubectl delete -f deploy/role.yaml`
* `kubectl delete -f deploy/service_account.yaml`
* `kubectl delete -f olm/open-liberty-crd.yaml`
