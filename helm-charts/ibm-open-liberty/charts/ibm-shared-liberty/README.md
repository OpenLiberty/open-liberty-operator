# Shared Liberty Templates (SLT)
A chart of shared templates to be used as a sub chart by WebSphere Liberty and Open Liberty Helm charts.

## Introduction
The goal is to develop a package of `Shared Liberty Templates` (referred to as `SLT`) as a sharable package of templates which are configurable and reusable by WebSphere Liberty and Open Liberty chart templates.

## Chart Details
* This chart does not install any kubernetes resources directly.

## Prerequisites
* Helm (Tiller) >= 2.7.0

## Resources Required
* NA - no resource requirements

## Installing the Chart
* This chart does not install as a standalone chart

## Initialization
The initialization step is needed in each template which uses the Shared Liberty Templates. This initialization step merges the config data into the root context of the template, referred to as the dot, “.”, root context.

For example, `include "slt.config.init"` passing a list containing the root context and the name of chart specific configuration.

__Example__

```go
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
```

## Configuration

The default configuration and initiation helpers for SLT (Shared Liberty Templates) is defined in `templates/_config.tpl`. In addition a given chart can specify additional values and/or override values via defined yaml structure passed during `"slt.config.init"` ([see below](#initialization)).

__Example__

```go
{{- /*
"slt.config.values" contains the default configuration values used by
the Shared Liberty Templates.

To override any of these values or to provide additional configuration values, modify the templates/_slt-config.tpl file 
*/ -}}
{{- define "slt.chart.config.values" -}}
slt:
  paths:
    wlpInstallDir: "/opt/ibm/wlp"
{{- end -}}
```
