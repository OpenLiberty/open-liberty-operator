{{- /*
Shared Liberty Templates (SLT)

ConfigMap templates:
  - slt.configmap

Usage of "slt.configmap.*" requires the following line be include at 
the begining of template:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{- define "slt.configmap" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
---
# SLT: 'slt.configmap' from templates/_configmap.tpl
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "slt.utils.fullname" (list $root) }}
  labels:
    chart: "{{ $root.Chart.Name }}-{{ $root.Chart.Version | replace "+" "_" }}"
    app: {{ include "slt.utils.fullname" (list $root) }}
    release: "{{ $root.Release.Name }}"
    heritage: "{{ $root.Release.Service }}"
data:
###############################################################################
#  Liberty Fabric
###############################################################################
  include-configmap.xml: |-
    <server>
      <include optional="true" location="/etc/wlp/configmap/server.xml"/>
      <include optional="true" location="/etc/wlp/configmap/cluster-ssl.xml"/>
    </server>

  server.xml: |-
    <server>
      <!-- Customize the running configuration. -->
    </server>

{{ if and $root.Values.ssl.enabled $root.Values.ssl.useClusterSSLConfiguration }}
  cluster-ssl.xml: |-
    <server>
      <featureManager>
        <feature>ssl-1.0</feature>
      </featureManager>
      <ssl id="defaultSSLConfig" keyStoreRef="defaultKeyStore" trustStoreRef="defaultTrustStore"/>
      <keyStore id="defaultKeyStore" location="/etc/wlp/config/keystore/key.jks" password="${env.MB_KEYSTORE_PASSWORD}" />
      <keyStore id="defaultTrustStore" location="/etc/wlp/config/truststore/trust.jks" password="${env.MB_TRUSTSTORE_PASSWORD}" />
    </server>
{{ end }}

{{- end -}}

