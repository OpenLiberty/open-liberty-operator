{{- /*
Shared Liberty Templates (SLT)

Utility functions:
  - slt.utils.getItem
  - slt.utils.name
  - slt.utils.fullname

Usage of "slt.utils.*" requires the following line be include at 
the begining of template:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{/*
"slt.utils.getItem" is a helper to get an item based on the index in the 
list and default value if the item does not exist. If the item exists, its text is 
generated, if the index is out of range of the list, then the default text is generated.

Config Values Used: NA
  
Uses: NA
    
Parameters input as an array of one values:
  - a list of items (required)
  - the index of the list (required)
  - the default text (required)

Usage:
  {{- $param1 := (include "slt.utils.getItem" (list $params 1 "defaultValue")) -}}
 
*/}}
{{- define "slt.utils.getItem" -}}
  {{- $params := . -}}
  {{- $list := first $params -}}
  {{- $index := (index $params 1) -}}
  {{- $default := (index $params 2) -}}
  {{- if (gt (add $index 1) (len $list) ) -}}
    {{- $default -}}
  {{- else -}}
    {{- index $list $index -}}
  {{- end -}}
{{- end -}}

{{/*
Expand the name of the chart.
*/}}
{{- define "slt.utils.name" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{- default $root.Chart.Name $root.Values.resourceNameOverride | trunc 24 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 24 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "slt.utils.fullname" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{- $name := default $root.Chart.Name $root.Values.resourceNameOverride -}}
{{- default (printf "%s-%s" $root.Release.Name $name) $root.Values.fullnameOverride | trunc 24 | trimSuffix "-" -}}
{{- end -}}

{{/*
Creates a boolean value names "isICP" that determines if Helm chart is deployed to IBM Cloud Private (ICP).
*/}}
{{- define "slt.utils.isICP" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{- if or (contains "icp" $root.Capabilities.KubeVersion.GitVersion) (eq "OpenShift" $root.slt.kube.provider) -}}
{{- $_ := set $root.Values "isICP" true -}}
{{- end -}}
{{- end -}}

{{/*
Creates a boolean value names "isOpenShift" that determines if Helm chart is deployed to IBM Cloud Private (ICP) on OpenShift.
*/}}
{{- define "slt.utils.isOpenShift" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{- if (eq "OpenShift" $root.slt.kube.provider) -}}
{{- $_ := set $root.Values "isOpenShift" true -}}
{{- end -}}
{{- end -}}

{{/*
Creates a boolean value named "isMonitoringEnabled" that determines if the Monitoring is supported AND is enabled.
*/}}
{{- define "slt.utils.isMonitoringEnabled" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{- if $root.Values.monitoring -}}
{{- if $root.Values.monitoring.enabled -}}
{{- $_ := set $root.Values "isMonitoringEnabled" true -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Creates a boolean value named "isWebSphereLiberty" that determines if the shared chart is used by WebSphere Liberty.
*/}}
{{- define "slt.utils.isWebSphereLiberty" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{- if (eq "websphere-liberty" $root.slt.parentChart) -}}
{{- $_ := set $root.Values "isWebSphereLiberty" true -}}
{{- end -}}
{{- end -}}

{{/*
Creates a boolean value named "isOpenLiberty" that determines if the shared chart is used by Open Liberty.
*/}}
{{- define "slt.utils.isOpenLiberty" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{- if (eq "open-liberty" $root.slt.parentChart) -}}
{{- $_ := set $root.Values "isOpenLiberty" true -}}
{{- end -}}
{{- end -}}

{{/*
Creates a boolean value named "isWebSphereLibertyRhel" that determines if the shared chart is used by WebSphere Liberty RHEL.
*/}}
{{- define "slt.utils.isWebSphereLibertyRhel" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{- if (eq "websphere-liberty-rhel" $root.slt.parentChart) -}}
{{- $_ := set $root.Values "isWebSphereLibertyRhel" true -}}
{{- end -}}
{{- end -}}

{{/*
Create a service name, defaulting to fullname if not provided.
*/}}
{{- define "slt.utils.servicename" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{- $root.Values.service.name | trunc 63 | lower | trimSuffix "-" | default (include "slt.utils.fullname" (list $root)) -}}
{{- end -}}
