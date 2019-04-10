{{- /*
Shared Liberty Templates (SLT)

Shared templates:
  - slt.shared.templates

Usage of "slt.shared.templates.*" requires the following line be include at 
the begining of template:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{/*
Includes other shared templates (e.g. Deployment, Service, etc).

This is included by the product (parent) chart (see templates/shared.yaml in parent chart)
*/}}
{{- define "slt.shared.templates" -}}
{{ include "slt.configmap" . }}
{{ include "slt.role.binding" . }}
{{ include "slt.service.headless" . }}
{{ include "slt.service" . }}
{{ include "slt.service.http.clusterip" . }}
{{ include "slt.deployment" . }}
{{ include "slt.job.secret.generator" . }}
{{ include "slt.ingress" . }}
{{ include "slt.ingress.secret" . }}
{{ include "slt.hpa" . }}
{{ include "slt.security.context.pod" . }}
{{ include "slt.security.context.container" . }}
{{ include "slt.service.account" . }}
{{ include "slt.role" . }}
{{- end -}}
