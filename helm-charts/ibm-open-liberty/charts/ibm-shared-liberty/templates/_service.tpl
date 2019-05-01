{{- /*
Shared Liberty Templates (SLT)

Service templates:
  - slt.service
  - slt.service.headless

Usage of "slt.service.*" requires the following line be include at 
the begining of template:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{- define "slt.service" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
  {{ include "slt.utils.isMonitoringEnabled" (list $root) }}
{{- $stateful := or $root.Values.logs.persistTransactionLogs $root.Values.logs.persistLogs }}
{{- if $root.Values.service.enabled }}
---
# SLT: 'slt.service' from templates/_service.tpl
apiVersion: v1
kind: Service
metadata:
  name: {{ include "slt.utils.servicename" (list $root) }}
  annotations:
{{- if and $root.Values.isMonitoringEnabled (not $root.Values.ssl.enabled) }}
    prometheus.io/scrape: 'true'
{{- end }}
{{- with $root.Values.service.annotations }}
{{ toYaml . | indent 4 }}
{{- end }}
  labels:
    chart: "{{ $root.Chart.Name }}-{{ $root.Chart.Version }}"
    app: {{ include "slt.utils.fullname" (list $root) }}
    release: "{{ $root.Release.Name }}"
    heritage: "{{ $root.Release.Service }}"
{{- with $root.Values.service.labels }}
{{ toYaml . | indent 4 }}
{{- end }}
spec:
{{- if or (not $root.Values.ingress.enabled) $root.Values.ssl.enabled }}  
  type: {{ $root.Values.service.type }}
{{- end }}
  ports:
  - port: {{ $root.Values.service.port }}
    targetPort: {{ $root.Values.service.targetPort }}
    protocol: TCP
  {{- if $root.Values.ssl.enabled }}
    name: "https"
  {{- else }}
    name: "http"
  {{- end }}
  
{{- if $root.Values.iiopService.enabled }}
  - port: {{ $root.Values.iiopService.nonSecurePort }}
    targetPort: {{ $root.Values.iiopService.nonSecureTargetPort }}
    protocol: TCP
    name: "iiop"
  {{- if $root.Values.ssl.enabled }}
  - port: {{ $root.Values.iiopService.securePort }}
    targetPort: {{ $root.Values.iiopService.secureTargetPort }}
    protocol: TCP
    name: "iiops"
  {{- end }}
{{- end }}

{{- if $root.Values.jmsService.enabled}}
  - port: {{ $root.Values.jmsService.port }}
    targetPort: {{ $root.Values.jmsService.targetPort }}
    protocol: TCP
  {{- if $root.Values.ssl.enabled }}
    name: "jmss"
  {{- else }}
    name: "jms"
  {{- end }}
{{- end }}

{{- with $root.Values.service.extraPorts }}
{{ toYaml . | indent 2 }}
{{- end }}

  selector:
    app: {{ include "slt.utils.fullname" (list $root) }}
{{- with $root.Values.service.extraSelectors }}
{{ toYaml . | indent 4 }}
{{- end }}
{{- end }}
{{- end -}}


{{- define "slt.service.headless" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{- $stateful := or $root.Values.logs.persistTransactionLogs $root.Values.logs.persistLogs -}}
{{ if $stateful }}
---
# SLT: 'slt.service.headless' from templates/_service.tpl
apiVersion: v1
kind: Service
metadata:
  name: {{ include "slt.utils.servicename" (list $root) | trunc 59 }}-sts
  labels:
    chart: "{{ $root.Chart.Name }}-{{ $root.Chart.Version }}"
    app: {{ include "slt.utils.fullname" (list $root) }}
    release: "{{ $root.Release.Name }}"
    heritage: "{{ $root.Release.Service }}"
spec:
  ports:
  - port: {{ $root.Values.service.port }}
    protocol: TCP
  {{- if $root.Values.ssl.enabled }}
    name: "https"
  {{- else }}
    name: "http"
  {{- end }}
  
{{- if $root.Values.iiopService.enabled }}
  - port: {{ $root.Values.iiopService.nonSecurePort }}
    protocol: TCP
    name: "iiop"
  {{- if $root.Values.ssl.enabled }}
  - port: {{ $root.Values.iiopService.securePort }}
    targetPort: {{ $root.Values.iiopService.secureTargetPort }}
    protocol: TCP
    name: "iiops"
  {{- end }}
{{- end }}

{{- if $root.Values.jmsService.enabled}}
  - port: {{ $root.Values.jmsService.port }}
    protocol: TCP
  {{- if $root.Values.ssl.enabled }}
    name: "jmss"
  {{- else }}
    name: "jms"
  {{- end }}
{{- end }}

  clusterIP: None
  selector:
    app: {{ include "slt.utils.fullname" (list $root) }}
{{ end }}
{{- end -}}


{{- define "slt.service.http.clusterip" -}} 
  {{- $params := . -}}
  {{- $root := first $params -}}
  {{ include "slt.utils.isMonitoringEnabled" (list $root) }}
{{- if and $root.Values.isMonitoringEnabled $root.Values.service.enabled $root.Values.ssl.enabled }}
---
# SLT: 'slt.service.http.clusterip' from templates/_service.tpl
apiVersion: v1
kind: Service
metadata:
  annotations:
    prometheus.io/scrape: 'true'
  name: {{ include "slt.utils.servicename" (list $root) | trunc 48 }}-http-clusterip
  labels:
    chart: "{{ $root.Chart.Name }}-{{ $root.Chart.Version }}"
    app: {{ include "slt.utils.fullname" (list $root) }}
    release: "{{ $root.Release.Name }}"
    heritage: "{{ $root.Release.Service }}"
spec:
  ports:
  - port: {{ $root.slt.httpService.nonSecurePort }}
    targetPort: {{ $root.slt.httpService.nonSecurePort }}
    name: {{ $root.slt.httpService.name }}-clusterip
    protocol: TCP
  selector:
    app: {{ include "slt.utils.fullname" (list $root) }}
  type: ClusterIP
{{- end }}
{{- end -}}

