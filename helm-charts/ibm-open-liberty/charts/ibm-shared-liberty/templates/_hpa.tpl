{{- /*
Shared Liberty Templates (SLT)

HorizontalPodAutoscaler templates:
  - slt.hpa

Usage of "slt.hpa.*" requires the following line be include at 
the begining of template:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{- define "slt.hpa" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{ if $root.Values.autoscaling.enabled }}
---
# SLT: 'slt.hpa' from templates/_hpa.tpl
apiVersion: autoscaling/v1
kind: HorizontalPodAutoscaler
metadata:
  name: {{ include "slt.utils.fullname" (list $root) }}-hpa
  labels:
    chart: "{{ $root.Chart.Name }}-{{ $root.Chart.Version }}"
    app: {{ include "slt.utils.fullname" (list $root) }}
    release: "{{ $root.Release.Name }}"
    heritage: "{{ $root.Release.Service }}"    
spec:
  maxReplicas: {{ $root.Values.autoscaling.maxReplicas }}
  minReplicas: {{ $root.Values.autoscaling.minReplicas }}
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ include "slt.utils.fullname" (list $root) }}
  targetCPUUtilizationPercentage: {{ $root.Values.autoscaling.targetCPUUtilizationPercentage }}
{{- end }}
{{- end -}}

