{{- /*
Shared Liberty Templates (SLT)

Job templates:
  - slt.job.secret.generator

Usage of "slt.job.*" requires the following line be include at 
the begining of template:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{- define "slt.job.secret.generator" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{ if $root.Values.ssl.createClusterSSLConfiguration }}
---
# SLT: 'slt.job.secret.generator' from templates/_job.tpl
apiVersion: batch/v1
kind: Job
metadata:
  name: liberty-secret-generator-deploy
  annotations:
    "helm.sh/hook-delete-policy": hook-succeeded,hook-failed
  labels:
    app: {{ include "slt.utils.fullname" (list $root) }}
    heritage: {{$root.Release.Service | quote }}
    release: {{$root.Release.Name | quote }}
    chart: "{{$root.Chart.Name}}-{{$root.Chart.Version}}"  
spec:
  template:
    metadata:
      name: liberty-secret-generator-deploy
      annotations:
        sidecar.istio.io/inject: "false"
{{- include "slt.security.context.pod" . | indent 4 }}
    spec:
      restartPolicy: Never
      {{ if $root.Values.rbac.install }}
      serviceAccountName: {{ include "slt.utils.fullname" (list $root) }}
      {{ end }}
      containers:
      - name: liberty-secret-generator-deploy
        image: ibmcom/mb-tools:2.0.0
{{- include "slt.security.context.container" . | indent 8 -}}
{{ end }}
{{- end -}}
