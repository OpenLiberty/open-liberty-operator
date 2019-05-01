{{- /*
Shared Liberty Templates (SLT)

Affinity functions:
  - slt.affinity.nodeaffinity

Usage of "slt.affinity.*" requires the following line be include at 
the begining of template:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{/* affinity - https://kubernetes.io/docs/concepts/configuration/assign-pod-node/ */}}

{{- define "slt.affinity.nodeaffinity" }}
  {{- $params := . -}}
  {{- $root := first $params -}}
# https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
    {{ include "slt.nodeAffinityRequiredDuringScheduling" (list $root) }}
    preferredDuringSchedulingIgnoredDuringExecution:
    {{- include "slt.nodeAffinityPreferredDuringScheduling" (list $root) }}
{{- end }}

{{- define "slt.nodeAffinityRequiredDuringScheduling" }}
  {{- $params := . -}}
  {{- $root := first $params -}}
    # If you specify multiple nodeSelectorTerms associated with nodeAffinity types,
    # then the pod can be scheduled onto a node if one of the nodeSelectorTerms is satisfied.
    #
    # If you specify multiple matchExpressions associated with nodeSelectorTerms,
    # then the pod can be scheduled onto a node only if all matchExpressions can be satisfied.
    #
    # valid operators: In, NotIn, Exists, DoesNotExist, Gt, Lt
      nodeSelectorTerms:
      - matchExpressions:
        - key: beta.kubernetes.io/arch
          operator: In
          values:
        {{- range $key, $val := $root.Values.arch }}
          {{- if gt ($val | trunc 1 | int) 0 }}
          - {{ $key }}
          {{- end }}
        {{- end }}
{{- end }}

{{- define "slt.nodeAffinityPreferredDuringScheduling" }}
  {{- $params := . -}}
  {{- $root := first $params -}}
  {{- range $key, $val := $root.Values.arch }}
    {{- if gt ($val | trunc 1 | int) 0 }}
    - weight: {{ $val | trunc 1 | int }}
      preference:
        matchExpressions:
        - key: beta.kubernetes.io/arch
          operator: In
          values:
          - {{ $key }}
    {{- end }}
  {{- end }}
{{- end }}
