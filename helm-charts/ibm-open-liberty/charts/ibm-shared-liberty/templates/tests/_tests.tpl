{{- /*
Shared Liberty Templates (SLT)

Tests:
  - slt.test.wget

Usage of "slt.test.*" requires the following line be include at 
the begining of template:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{- define "slt.test.wget" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
# SLT: 'slt.test.wget' from templates/test/_tests.tpl
apiVersion: v1
kind: Pod
metadata:
  name: "{{ include "slt.utils.fullname" (list $root) }}-test"
  labels:
    heritage: {{ $root.Release.Service }}
    release: {{ $root.Release.Name }}
    chart: {{ $root.Chart.Name }}-{{ $root.Chart.Version }}
    app: {{ include "slt.utils.name" (list $root) }}
  annotations:
    "helm.sh/hook": test-success
spec:
  containers:
    - name: "{{ include "slt.utils.fullname" (list $root) }}-test"
      image: alpine:3.8
      {{ if $root.Values.ssl.enabled }}
      command: ["sh", "-c", 'apk --no-cache add openssl && wget --no-check-certificate https://{{ include "slt.utils.fullname" (list $root) }}:{{ $root.Values.service.port }}']
      {{ else }}
      command: ['wget']
      args:  ['{{ include "slt.utils.fullname" (list $root) }}:{{ $root.Values.service.port }}']
      {{ end }}
  restartPolicy: Never
{{- end -}}

