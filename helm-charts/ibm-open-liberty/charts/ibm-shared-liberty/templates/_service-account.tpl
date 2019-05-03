{{- /*
Shared Liberty Templates (SLT)

ServiceAccount templates:
  - slt.service.account

Usage of "slt.service.account.*" requires the following line be include at 
the begining of template:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{- define "slt.service.account" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{ if $root.Values.rbac.install }}
---
# SLT: 'slt.service.account' from templates/_service-account.tpl
kind: ServiceAccount
apiVersion: v1
metadata:
  name: {{ include "slt.utils.fullname" (list $root) }}
  labels:
    chart: "{{ $root.Chart.Name }}-{{ $root.Chart.Version }}"
    app: {{ include "slt.utils.fullname" (list $root) }}
    release: "{{ $root.Release.Name }}"
    heritage: "{{ $root.Release.Service }}"
imagePullSecrets:
  - name: sa-{{ $root.Release.Namespace }}
{{- if $root.Values.image.pullSecret }}
  - name: {{ $root.Values.image.pullSecret }}
{{- end }}
{{ end }}
{{- end -}}
