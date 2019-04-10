{{- /*
Shared Liberty Templates (SLT)

Role templates:
  - slt.role

Usage of "slt.role.*" requires the following line be include at 
the begining of template:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{- define "slt.role" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{ if $root.Values.rbac.install }}
---
# SLT: 'slt.role' from templates/_role.tpl
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: {{ include "slt.utils.fullname" (list $root) }}
  labels:
    chart: "{{ $root.Chart.Name }}-{{ $root.Chart.Version }}"
    app: {{ include "slt.utils.fullname" (list $root) }}
    release: "{{ $root.Release.Name }}"
    heritage: "{{ $root.Release.Service }}"
rules:
- apiGroups:
  - ""
  resources:
  - endpoints
  verbs:
  - get
  - list
- apiGroups: 
  - ""
  resources:
  - secrets
  verbs:
  - get
  - list
  - watch
  - create
- apiGroups: 
  - ""
  resources:
  - configmaps
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
{{ end }}
{{- end -}}