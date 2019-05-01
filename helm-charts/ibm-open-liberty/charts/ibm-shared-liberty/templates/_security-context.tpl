{{- /*
Shared Liberty Templates (SLT)

SecurityContext templates:
  - slt.security.context.pod
  - slt.security.context.container

Usage of "slt.security.context.*" requires the following line be include at 
the begining of template:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{- define "slt.security.context.pod" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
  {{ include "slt.utils.isWebSphereLiberty" (list $root) }}
  {{ include "slt.utils.isWebSphereLibertyRhel" (list $root) }}
  {{ include "slt.utils.isOpenLiberty" (list $root) }}
# SLT: 'slt.security.context.pod' from templates/_security-context.tpl
hostNetwork: false
hostPID: false
hostIPC: false
securityContext:
  runAsNonRoot: true
  runAsUser: 1001
  fsGroup: {{ $root.Values.persistence.fsGroupGid }}
{{- end -}}
{{- define "slt.security.context.container" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
  {{ include "slt.utils.isWebSphereLiberty" (list $root) }}
# SLT: 'slt.security.context.container' from templates/_security-context.tpl
securityContext:
  privileged: false
  readOnlyRootFilesystem: false
  allowPrivilegeEscalation: false
  capabilities:
    drop:
    - ALL
{{- end -}}
