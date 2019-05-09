# Open Liberty Operator

This repository hosts the Open Liberty Operator to be used in Kubernetes clusters.

## Operator Image

Current image in Docker Hub:  `openliberty/operator:0.0.1`

## Instructions to install and use the Open Liberty Operator

* Fetch a Linux VM

* Download and unpack oc client  
  * https://github.com/openshift/origin/releases/download/v3.11.0/openshift-origin-client-tools-v3.11.0-0cbc58b-linux-64bit.tar.gz
  * `tar -zxvf <tar.gz>`
    * you get the `oc` client and `kubectl` client with this.  You should add these into your `PATH`

* Start OKD cluster (5-10 minutes):
  * `oc cluster up --public-hostname=<hostNameorIP>  --skip-registry-check=true`
    * You should see some information about how to reach your Web Console, etc
  * `oc login -u system:admin`

* Install Operator artifacts
  * `git clone https://github.com/OpenLiberty/open-liberty-operator.git`
  * `cd open-liberty-operator`
  * `kubectl apply -f olm/open-liberty-crd.yaml`
  * `kubectl apply -f deploy/operator_rbac.yaml`
  * `kubectl apply -f deploy/operator_deployment.yaml`

* Install SCC
  * `cd helm-charts/ibm-open-liberty/ibm_cloud_pak/pak_extensions/pre-install/clusterAdministration`
  * `./createSecurityClusterPrereqs.sh`
  * `cd ../namespaceAdministration`
  * `./createSecurityNamespacePrereqs.sh <namespace>`

* Test Operator with default CR
  * cd back up to project root
  * `kubectl apply -f deploy/full_cr.yaml`

* Bringing down the cluster:
  * `oc cluster down`
  * `rm -rf <oc/openshift.local.clusterup>`
    * If you get an error about a busy device, reboot the VM, and re-run rm -rf
