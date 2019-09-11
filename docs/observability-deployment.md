
# Observability with Open liberty
The following document covers various topics for configuring and integrating your Open Liberty runtime with monitoring tools in the OpenShift cluster.

## How to deploy Kibana dashboards to monitor Open Liberty logging events 


Kibana dashboards are provided for visualizing events from the Open Liberty runtime.


To leverage the use of these dashboards the logging events must be emitted in JSON format into standard-out. For information regarding how to configure an Open Liberty image with JSON logging please see:   https://github.com/OpenLiberty/ci.docker#logging


Retrieve available Kibana dashboards tuned for Liberty logging events under https://github.com/OpenLiberty/open-liberty-operator/deploy/dashboards




## How to monitor your Liberty runtimes 
A Microprofile metrics enabled Open Liberty runtime is capable of tracking and observing metrics from the JVM and Open Liberty server as well as tracking Microprofile Metrics instrumented within a deployed application. The tracked metric data can then be scraped by Prometheus and visualized with Grafana.


As a prerequisite Prometheus and  Grafana will need to be first deployed before their functionality can be leveraged.


### MicroProfile Metrics 1.x


Configure your Open Liberty docker image to enable MicroProfile Metrics 1.x by setting the `ARG` value with `MP_MONITORING=true` before building the image with the provided build process at https://github.com/OpenLiberty/ci.docker 


For example:
```
FROM open-liberty:kernel
# Add my app and config
COPY --chown=1001:0  Sample1.war /config/dropins/
COPY --chown=1001:0  server.xml /config/


# Optional functionalityARG SSL=true
ARG MP_MONITORING=true


# This script will add the requested XML snippets and grow image to be fit-for-purpose
RUN configure.sh
```


Proceed to _Enabling Prometheus to scrape data_ on instructions on how to configure your deployment with Prometheus.


### MicroProfile Metrics 2.0


The build process provided by https://github.com/OpenLiberty/ci.docker does not currently include a configurable optional enterprise functionality `ARG` parameter for MicroProfile Metrics 2.0. The following steps outline how to manually create and modify a `server.xml` to add the `mpMetrics-2.0` feature and `monitor-1.0` feature that will be built as part of your Open Liberty image.


1.    Create a XML file named `server_mpMetrics_2.0.xml`, with the following contents and place it in the same directory as your Dockerfile:
```
<?xml version=“1.0” encoding=“UTF-8"?>
<server>
   <featureManager>
       <feature>mpMetrics-2.0</feature>
<feature>monitor-1.0</feature>
   </featureManager>
   <mpMetrics authentication="false" />
</server>
```

2.    In your DockerFile, create the `configDropins/overrides` directory, by adding the following line:
```
MKDIR /config/configDropins/overrides
```

3.    In your DockerFile, add the following line to copy the `server_mpMetrics_2.0.xml` file into the `configDropins/overrides` directory:
```
COPY server_mpMetrics_2.0.xml /config/configDropins/overrides/
```

Proceed to _Enabling Prometheus to scrape data_ on instructions on how to configure your deployment with Prometheus.


### Enabling Prometheus to scrape data 


There are two ways in which Prometheus can be deployed and configured for log consumption. The first approach is to deploy the Prometheus through the Prometheus Operator which will then utilize Service Monitors to monitor and scrape logs from target services.  The second approach is considered the _legacy_ approach in which Prometheus is deployed directly and configured to listen to deployments with specific annotations. Details regarding how to deploy and configure Prometheus in both approaches are covered in the following document https://github.com/kabanero-io/guide-logging-monitoring.


Using a Service Monitor would be the desired approach and will provide your micro service environment with greater inter-operability as your environment scales and evolves.


With regards to the _legacy approach_ the Open Liberty operator offers a configuration value to easily instrument the annotations. You can enable Prometheus to begin scraping metrics data from the Open Liberty /metrics endpoint by implementing the following snippet into your Open Liberty Operator YAML configuration file.


```
  monitoring:
    enabled: true
```
See https://github.com/OpenLiberty/open-liberty-operator/blob/master/deploy/crds/full_cr.yaml for full template of available fields.


### Visualizing your data with Grafana


There are IBM provided Grafana dashboards that leverage the metrics tracked from the JVM provided information as well as the Open Liberty runtime. 


An Open Liberty server configured with MicroProfile Metrics 1.1 will be instrumented with the `mpMerics-1.1` feature in the server's `server.xml`.  Similarly a MicroProfile Metrics 2.0  configured OpenLiberty server will be instrumented with the `mpMetrics-2.0` feature. Find the appropriate dashboards at:
https://github.com/OpenLiberty/open-liberty-operator/deploy/dashboards/


## How to use health info with service orchestrator 


MicroProfile Health Check allows services to report their health, and it publishes the overall health status to defined endpoints. If a service reports UP, then it’s available. If the service reports DOWN, then it’s unavailable. MicroProfile Health reports an individual service status at the endpoint and indicates the overall status as UP if all the services are UP. A service orchestrator can then use the health statuses to make decisions.
 
### MicroProfile Health Check 1.0
1.    Follow the instructions from the ci.docker GitHub README https://github.com/WASdev/ci.docker

  a. Set the `MP_HEALTH_CHECK` argument to true in your DockerFile:
  ```
  ARG MP_HEALTH_CHECK=true
  ```
  b. Add the following line in your DockerFile to run the script, which will add the requested XML snippets from the ARG arguments:
  ```
     RUN configure.sh
  ```

### MicroProfile Health Check 2.0
The build process provided by https://github.com/OpenLiberty/ci.docker does not currently include a configurable optional enterprise functionality `ARG` parameter for MicroProfile Health Check 2.0. The following steps outline how to manually create and modify a server.xml to add the mpHealth-2.0 feature that will be built as part of your Open Liberty image.


Configure mpHealth-2.0 feature in server.xml:
1.    Create a XML file named `server_mpHealth_2.0.xml`, with the following contents and place it in the same directory as your DockerFile:
```
<?xml version=“1.0” encoding=“UTF-8"?>
<server>
   <featureManager>
       <feature>mpHealth-2.0</feature>
   </featureManager>
</server>
```

2.    In your DockerFile, create the `configDropins/overrides` directory, by adding the following line:
```
MKDIR /config/configDropins/overrides
```

3.    In your DockerFile, add the following line to copy the `server_mpHealth_2.0.xml` file into the `configDropins/overrides` directory:
```
COPY server_mpHealth_2.0.xml /config/configDropins/overrides/
```

## Configure the Kubernetes Liveness and Readiness Probes with the MicroProfile Health Check REST Endpoints


Kubernetes provides liveness and readiness probes that are used to check the health of your containers. These probes can check certain files in your containers, check a TCP socket, or make HTTP requests. MicroProfile Health Check exposes readiness and liveness endpoints on your microservices. Kubernetes polls these endpoints as specified by the probes to react appropriately to any change in the microservice’s status.
 
1.    Configure the readiness and liveness probes fields to point to the MicroProfile Health Check REST endpoints in your OpenLiberty Operator YAML configuration file:
See https://github.com/OpenLiberty/open-liberty-operator/blob/master/deploy/crds/full_cr.yaml for full template of available fields.
For mpHealth-1.0:
Enable the MicroProfile Health Check in your OpenLiberty Operator YAML configuration file, with the following snippet:
microprofile:  
   health:     
     enabled: true
    
For mpHealth-2.0:
Modify the readiness and liveness probes fields to point to the MicroProfile Health Check REST endpoints:
```
spec:
 image:
   ...
   readinessProbe: { 
httpGet:
  path: /health/ready
  port: 9080
  initialDelaySeconds: 3
  periodSeconds: 5
    }
 
   livenessProbe: {
httpGet:
  path: /health/live
  port: 9080
  initialDelaySeconds: 40
  periodSeconds: 10
   }
...
```