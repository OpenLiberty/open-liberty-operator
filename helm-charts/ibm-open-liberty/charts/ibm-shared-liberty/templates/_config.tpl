{{- /*
Shared Liberty Templates (SLT)

Initialization and configuration:
  - slt.config.values
  - slt.chart.default.config.values
  - slt.config.init

slt<x.y.z>/_config.tpl is the config file slt. In addition a given
chart can specify additional values and/or override values via defined 
yaml structure passed during "slt.config.init" (see below). 
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{- /*
"slt.config.values" contains the default configuration values used by
the Shared Liberty Templates.

To override any of these values, modify the templates/_slt-config.tpl file 
*/ -}}
{{- define "slt.config.values" -}}
slt:
  httpService:
    name: http-service-liberty
    nonSecurePort: 9080
{{- end -}}

{{- /*
"slt.chart.default.config.values" contains a default configuration values used by
the Shared Liberty Templates if no chart specific override file exists.

To override any of these values, modify the templates/slt-config.tpl file
by defining "slt.chart.config.values" 
*/ -}}
{{- define "slt.chart.default.config.values" -}}

{{- end -}}


{{- /*
"slt.config.init" will merge the slt config and override into the root context (aka "dot", ".")

Uses:
  - "slt.utils.getItem"

Parameters input as an array of one values:
  - the root context (required)
  - "slt.chart.config.values" (optional) if defined by the chart, will default to use defined "slt.chart.default.config.values"

Any template in which uses slt should have the following at the begin of the template.

Usage:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
or 
{{- include "slt.config.init" (list .) -}}

*/ -}}
{{- define "slt.config.init" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
  {{- $sltChartConfigName := (include "slt.utils.getItem" (list $params 1 "slt.chart.default.config.values")) -}}
  {{- $sltChartConfig := fromYaml (include $sltChartConfigName $root) -}}
  {{- $sltConfig := fromYaml (include "slt.config.values" $root) -}}
  {{- $valuesMetadata := dict "valuesMetadata" (fromYaml ($root.Files.Get "values-metadata.yaml")) -}}
  {{- $_ := merge $root $sltChartConfig -}}
  {{- $_ := merge $root $sltConfig -}}
  {{- $_ := merge $root $valuesMetadata -}}
{{- end -}}


