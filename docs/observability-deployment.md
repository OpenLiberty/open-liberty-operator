

# Observability with Open Liberty


The following document covers various topics for configuring and integrating your Open Liberty runtime with monitoring tools in the OpenShift cluster.

## How to deploy Kibana dashboards to monitor Open Liberty logging events  

Kibana dashboards are provided for visualizing events from the Open Liberty runtime.

To leverage the use of these dashboards the logging events must be emitted in JSON format into standard-out. For information regarding how to configure an Open Liberty image with JSON logging please see [the following](https://github.com/OpenLiberty/ci.docker#logging).

Retrieve available Kibana dashboards tuned for Open Liberty logging events [here](https://github.com/OpenLiberty/open-liberty-operator/deploy/dashboards/logging).

For information regarding how to import Kibana dashboards see the official documentation [here](https://www.elastic.co/guide/en/kibana/5.6/loading-a-saved-dashboard.html).

For effective management of logs emitted from applications it is advised to deploy your own Elasticsearch, FluentD and Kibana (EFK) stack. For more information see the following [article](https://kabanero.io/guides/app-logging/). 

## How to monitor your Liberty runtimes  

A MicroProfile Metrics enabled Open Liberty runtime is capable of tracking and observing metrics from the JVM and Open Liberty server as well as tracking MicroProfile Metrics instrumented within a deployed application. The tracked metrics can then be scraped by Prometheus and visualized with Grafana.

### MicroProfile Metrics 1.x and 2.x

The build process provided by [ci.docker](https://github.com/OpenLiberty/ci.docker) currently includes a configurable optional enterprise functionality `ARG` parameter for MicroProfile Metrics 1.1. However this parameter option will be deprecated and the below steps should be taken to configure your Open Liberty image with MicroProfile Metrics 1.1. The following steps will be using the `mpMetrics-2.0` feature directly as an example. If you intend to configure with MicroProfile Metrics 1.1 you can use the `mpMetrics-1.1` feature in place of `mpMetrics-2.0`. 

The following steps outline how to manually create and modify a `server.xml` to add the `mpMetrics-2.0` feature and `monitor-1.0` feature that will be built as part of your Open Liberty image.

1. Create a XML file named `server_mpMetrics.xml` with the following contents and place it in the same directory as your Dockerfile:


```XML
<?xml version=“1.0” encoding=“UTF-8"?>
<server>
   <featureManager>
       <feature>mpMetrics-2.0</feature>
       <feature>monitor-1.0</feature>
   </featureManager>
   <quickStartSecurity userName="admin" userPassword="adminPwd"/>
</server>
```


The above `server.xml` configuration secures access to the server with basic authentication using the `<quickStartSecurity>` element. The `<quickStartSecurity>` is used in the above example for simplicity. When configuring your server you may wish to use a [basic registry](https://www.ibm.com/support/knowledgecenter/en/SSEQTP_liberty/com.ibm.websphere.wlp.doc/ae/twlp_sec_basic_registry.html) or a [LDAP registry](https://www.ibm.com/support/knowledgecenter/en/SSEQTP_liberty/com.ibm.websphere.wlp.doc/ae/twlp_sec_ldap.html) for securing authenticated access to your server. When using Prometheus to scrape data from the `/metrics` endpoint only the _Service Monitor_ approach can be configured to negotiate authentication with the Open Liberty server. 


2.    In your DockerFile, add the following line to copy the `server_mpMetrics_2.0.xml` file into the `configDropins/overrides` directory:


```DockerFile
COPY --chown=1001:0 server_mpMetrics_2.0.xml /config/configDropins/overrides/
```


Proceed to [Enabling Prometheus to scrape data](#ENABLING-PROMETHEUS-TO-SCRAPE-DATA) on instructions on how to configure your deployment with Prometheus.




### Enabling Prometheus to scrape data 


You will need to deploy Prometheus through the Prometheus Operator which will then utilize Service Monitors to monitor and scrape logs from target services. Details regarding how to deploy and configure Prometheus in both is found in the [following document](https://kabanero.io/guides/app-monitoring/#option-a-deploy-prometheus-prometheus-operator).

### Visualizing your data with Grafana


There are IBM provided Grafana dashboards that leverage metrics from the JVM as well as from the Open Liberty runtime.  Details regarding how to deploy and configure Grafana are covered in the following [document](https://kabanero.io/guides/app-monitoring#deploy-grafana).


You can find the access point of Grafana by running the following:


```bash
# oc get routes -n grafana
NAME          HOST/PORT                                      PATH      SERVICES      PORT      TERMINATION   WILDCARD
grafana-ocp   grafana-ocp-grafana.apps.9.37.135.153.nip.io             grafana-ocp   <all>     reencrypt     None
```


The `grafana` value is the namespace that you deploy Grafana to.


An Open Liberty server configured with MicroProfile Metrics 1.1 will be instrumented with the `mpMetrics-1.1` feature in the server's `server.xml`.  Similarly a MicroProfile Metrics 2.0  configured Open Liberty server will be instrumented with the `mpMetrics-2.0` feature. Find the appropriate dashboards [here](https://github.com/OpenLiberty/open-liberty-operator/deploy/dashboards/metrics).


## How to use health info with service orchestrator  


MicroProfile Health allows services to report their readiness and liveness statuses (i.e UP if it is ready or alive and DOWN if its not ready/alive) through two endpoints. The Health data will be available on the `/health/live` and `/health/ready` endpoints for the liveness checks and for the readiness checks, respectively.
Readiness check allows third party services to know if the service is ready to process requests or not. e.g., dependency checks, such as database connections, application initialization, etc. 
Liveness check allows third party services to determine if the service is running. This means that if this procedure fails the service can be discarded (terminated, shutdown). e.g., running out of memory.
It reports an individual service status at the endpoints and indicates the overall status as UP if all the services are UP. A service orchestrator can then use these health check statuses to make decisions.


### MicroProfile Health 2.0

 The following steps outline how to manually create and modify a server.xml to add the mpHealth-2.0 feature that will be built as part of your Open Liberty image.


Configure mpHealth-2.0 feature in server.xml:


1.    Create a XML file named `server_mpHealth_2.0.xml`, with the following contents and place it in the same directory as your DockerFile:


```XML
<?xml version=“1.0” encoding=“UTF-8"?>
<server>
   <featureManager>
       <feature>mpHealth-2.0</feature>
   </featureManager>
</server>
```


2.    In your DockerFile, add the following line to copy the `server_mpHealth_2.0.xml` file into the `configDropins/overrides` directory:


```DockerFile
COPY --chown=1001:0 server_mpHealth_2.0.xml /config/configDropins/overrides/
```


## Configure the Kubernetes Liveness and Readiness Probes with the MicroProfile Health REST Endpoints


Kubernetes provides liveness and readiness probes that are used to check the health of your containers. These probes can check certain files in your containers, check a TCP socket, or make HTTP requests. MicroProfile Health exposes readiness and liveness endpoints on your microservices. Kubernetes polls these endpoints as specified by the probes to react appropriately to any change in the microservice’s status.
  
Configure the readiness and liveness probe's fields to point to the MicroProfile Health REST endpoints in your Open Liberty Operator YAML configuration file:
See [the following](https://github.com/OpenLiberty/open-liberty-operator/blob/master/deploy/crds/full_cr.yaml) for full template of available fields.

### For mpHealth-2.0


Modify the readiness and liveness probe's fields to point to the MicroProfile Health REST endpoints:


```YAML
spec:
 image:
   ...
   readinessProbe: {  
      httpGet:
         path: /health/ready
         port: 9443
         scheme: HTTPS
      initialDelaySeconds: 3
      periodSeconds: 5
   }


   livenessProbe: {
      httpGet:
         path: /health/live
         port: 9443
         scheme: HTTPS
      initialDelaySeconds: 40
      periodSeconds: 10
   }
...
```