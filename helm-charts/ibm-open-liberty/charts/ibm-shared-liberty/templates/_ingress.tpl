{{- /*
Shared Liberty Templates (SLT)

Ingress templates:
  - slt.ingress

Usage of "slt.ingress.*" requires the following line be include at 
the begining of template:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{- define "slt.ingress" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{- if and $root.Values.service.enabled $root.Values.ingress.enabled }}
{{- $fullname := include "slt.utils.fullname" (list $root) }}
---
# SLT: 'slt.ingress' from templates/_ingress.tpl
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: {{ $fullname }}
  labels:
    chart: "{{ $root.Chart.Name }}-{{ $root.Chart.Version | replace "+" "_" }}"
    app: {{ $fullname }}
    release: "{{ $root.Release.Name }}"
    heritage: "{{ $root.Release.Service }}"
{{- with $root.Values.ingress.labels }}
{{ toYaml . | indent 4 }}
{{- end }}
  annotations:
    kubernetes.io/ingress.class: "nginx"
    # The NGINX ingress annotations contains a new prefix nginx.ingress.kubernetes.io.
    # To avoid breaking a running NGINX ingress controller, specify both new and old prefixes.
    ingress.kubernetes.io/affinity: "cookie"
    nginx.ingress.kubernetes.io/affinity: "cookie"
    ingress.kubernetes.io/session-cookie-name: "route"
    nginx.ingress.kubernetes.io/session-cookie-name: "route"
    {{- if $root.Values.ssl.enabled }}
    ingress.kubernetes.io/secure-backends: "true"
    nginx.ingress.kubernetes.io/secure-backends: "true"
    ingress.kubernetes.io/backend-protocol: "HTTPS"
    nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
    ingress.bluemix.net/ssl-services: ssl-service={{ include "slt.utils.servicename" (list $root) }}
    {{- end }}
    ingress.kubernetes.io/session-cookie-hash: "sha1"
    nginx.ingress.kubernetes.io/session-cookie-hash: "sha1"
    ingress.kubernetes.io/rewrite-target: {{ $root.Values.ingress.rewriteTarget }}
    nginx.ingress.kubernetes.io/rewrite-target: {{ $root.Values.ingress.rewriteTarget }}
    ingress.bluemix.net/sticky-cookie-services: "serviceName={{ include "slt.utils.servicename" (list $root) }} name=route expires=1h path={{ $root.Values.ingress.path }} hash=sha1"
    ingress.bluemix.net/rewrite-path: "serviceName={{ include "slt.utils.servicename" (list $root) }} rewrite={{ $root.Values.ingress.rewriteTarget }}"
{{- with $root.Values.ingress.annotations }}
{{ toYaml . | indent 4 }}
{{- end }}
spec:
  {{- if and $root.Values.ssl.enabled $root.Values.ingress.host }}
  tls:
  - secretName: {{ $root.Values.ingress.secretName }}
    {{- if $root.Values.ingress.host }}
    hosts:
    - {{ $root.Values.ingress.host }}
    {{- end -}}
  {{- end }}
  rules:
    - http:
        paths:
        - path: {{ $root.Values.ingress.path }}
          backend:
            serviceName: {{ include "slt.utils.servicename" (list $root) }}
            servicePort: {{ $root.Values.service.port }}
      {{- if $root.Values.ingress.host }}
      host: {{ $root.Values.ingress.host }}
      {{- end -}}
{{- end }}
{{- end -}}
