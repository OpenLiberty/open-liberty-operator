{{- /*
Shared Liberty Templates (SLT)

Deployment templates:
  - slt.deployment

Usage of "slt.deployment.*" requires the following line be include at 
the begining of template:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{- define "slt.deployment" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
---
# SLT: 'slt.deployment' from templates/_deployment.tpl
{{- $stateful := or $root.Values.logs.persistTransactionLogs $root.Values.logs.persistLogs -}}
{{ if $stateful }}
apiVersion: apps/v1
kind: StatefulSet
{{ else }}
apiVersion: apps/v1
kind: Deployment
{{ end }}
metadata:
  name: {{ include "slt.utils.fullname" (list $root) }}
  labels:
    chart: "{{ $root.Chart.Name }}-{{ $root.Chart.Version }}"
    app: {{ include "slt.utils.fullname" (list $root) }}
    release: "{{ $root.Release.Name }}"
    heritage: "{{ $root.Release.Service }}"
{{- with $root.Values.deployment.labels }}
{{ toYaml . | indent 4 }}
{{- end }}
{{- with $root.Values.deployment.annotations }}
  annotations:
{{ toYaml . | indent 4}}
{{- end }}
spec:
  {{ if $stateful }}
  serviceName: {{ include "slt.utils.servicename" (list $root) }}
  {{ end }}
  {{ if not $root.Values.autoscaling.enabled -}}
  replicas: {{ $root.Values.replicaCount }}
  {{- end }}
  selector:
    matchLabels:
      app: {{ include "slt.utils.fullname" (list $root) }}
  template:
    metadata:
      labels:
        chart: "{{ $root.Chart.Name }}-{{ $root.Chart.Version }}"
        app: {{ include "slt.utils.fullname" (list $root) }}
        release: "{{ $root.Release.Name }}"
        heritage: "{{ $root.Release.Service }}"
{{- with $root.Values.pod.labels }}
{{ toYaml . | indent 8 }}
{{- end }}
      annotations:
        productName: {{ $root.slt.product.name }}
        productID: {{ $root.slt.product.id }}
        productVersion: {{ $root.slt.product.version }}
{{- with $root.Values.pod.annotations }}
{{ toYaml . | indent 8 }}
{{- end }}
    spec:
{{- with $root.Values.pod.security }}
{{ toYaml . | indent 6 }}
{{- else }}
{{- include "slt.security.context.pod" . | indent 6 }}
{{- end }}
      volumes:
      - name: liberty-overrides
        configMap:
          name: {{ include "slt.utils.fullname" (list $root) }}
          items:
          - key: include-configmap.xml
            path: include-configmap.xml
      - name: liberty-config
        configMap:
          name: {{ include "slt.utils.fullname" (list $root) }}
      {{- if $root.Values.image.serverOverridesConfigMapName }}
      - name: server-overrides-configmap
        configMap:
          name: {{ $root.Values.image.serverOverridesConfigMapName }}
          items:
          - key: server-overrides.xml
            path: server-overrides.xml
      {{- end }}
      {{ if and $root.Values.ssl.enabled $root.Values.ssl.useClusterSSLConfiguration }}
      - name: keystores
        secret:
          secretName: mb-keystore
      - name: truststores
        secret:
          secretName: mb-truststore
      {{ end }}
{{- with $root.Values.pod.extraVolumes }}
{{ toYaml . | indent 6 }}
{{- end -}}
      {{- if $root.Values.pod.extraInitContainers }}
      initContainers:
{{- with $root.Values.pod.extraInitContainers }}
{{ toYaml . | indent 6}}
{{- end }}
      {{- end }}
      {{ if $root.Values.rbac.install }}
      serviceAccountName: {{ include "slt.utils.fullname" (list $root) }}
      {{ end }}
      affinity:
      {{- include "slt.affinity.nodeaffinity" (list $root) | indent 6 }}
      {{/* Prefer horizontal scaling */}}
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app
                  operator: In
                  values:
                  - {{ include "slt.utils.fullname" (list $root) }}
                - key: release
                  operator: In
                  values:
                  - {{ $root.Release.Name | quote }}
              topologyKey: kubernetes.io/hostname
      containers:
      - name: {{ $root.Chart.Name }}
{{- with $root.Values.image.security }}
{{ toYaml . | indent 8 }}
{{- else }}
{{- include "slt.security.context.container" . | indent 8 }}
{{- end }}
      {{- if $root.Values.service.enabled }}
        readinessProbe:
        {{- if $root.Values.image.readinessProbe }}
{{ toYaml $root.Values.image.readinessProbe | indent 10 }}
        {{- else }}
          httpGet:
          {{- if $root.Values.microprofile.health.enabled }}
            path: /health
          {{- else }}
            path: /
          {{- end }}
            port: {{ $root.Values.service.targetPort }}
          {{- if $root.Values.ssl.enabled }}
            scheme: HTTPS
          {{- end }}
          initialDelaySeconds: 2
          periodSeconds: 5
        {{- end }}
        livenessProbe:
        {{- if $root.Values.image.livenessProbe }}
{{ toYaml $root.Values.image.livenessProbe | indent 10 }}
        {{- else }}
          httpGet:
          {{- if $root.Values.microprofile.health.enabled }}
            path: /health
          {{- else }}
            path: /
          {{- end }}
            port: {{ $root.Values.service.targetPort }}
          {{- if $root.Values.ssl.enabled }}
            scheme: HTTPS
          {{- end }}
          initialDelaySeconds: 20
          periodSeconds: 5
        {{- end }}
      {{- end }}
        image: "{{ $root.Values.image.repository }}:{{ $root.Values.image.tag }}"
        imagePullPolicy: {{ $root.Values.image.pullPolicy }}
{{- with $root.Values.image.lifecycle }}
        lifecycle:
{{ toYaml . | indent 10 }}
{{- end }}
        env:
{{- if $root.Values.image.extraEnvs }}
{{ toYaml $root.Values.image.extraEnvs | indent 8 }}
{{- end }}
        {{- if $root.Values.image.license }}
        - name: LICENSE
          value: {{ $root.Values.image.license }}
        {{- end }}
        {{- if $root.Values.env.jvmArgs }}
        - name: JVM_ARGS
          value: {{ $root.Values.env.jvmArgs }}
        {{- end }}
        - name: WLP_LOGGING_CONSOLE_FORMAT
          value: {{ $root.Values.logs.consoleFormat }}
        - name: WLP_LOGGING_CONSOLE_LOGLEVEL
          value: {{ $root.Values.logs.consoleLogLevel }}
        - name: WLP_LOGGING_CONSOLE_SOURCE
          value: {{ $root.Values.logs.consoleSource }}
        - name: KUBERNETES_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: IIOP_ENDPOINT_HOST
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
        - name : KEYSTORE_REQUIRED
          {{ if and $root.Values.ssl.enabled (not $root.Values.ssl.useClusterSSLConfiguration) }}
          value: "true"
          {{ else }}
          value: "false"
          {{ end }}
        {{ if and $root.Values.ssl.enabled $root.Values.ssl.useClusterSSLConfiguration -}}
        - name: MB_KEYSTORE_PASSWORD
          valueFrom:
            secretKeyRef:
              name: mb-keystore-password
              key: password
        - name: MB_TRUSTSTORE_PASSWORD
          valueFrom:
            secretKeyRef:
              name: mb-truststore-password
              key: password
        {{- end }}
        {{- if ($root.Values.oidcClient) }}
        {{- if $root.Values.oidcClient.enabled }}
        - name: OIDC_CLIENT_ID
          value: {{ $root.Values.oidcClient.clientId}}
        - name: OIDC_CLIENT_SECRET
          valueFrom:
            secretKeyRef:
              name: {{ $root.Values.oidcClient.clientSecretName}}
              key: clientSecret
        - name: OIDC_DISCOVERY_URL
          value: {{ $root.Values.oidcClient.discoveryURL}}
        {{- end }}
        {{- end }}
        volumeMounts:
        - name: liberty-overrides
          mountPath: /config/configDropins/overrides/include-configmap.xml
          subPath: include-configmap.xml
          readOnly: true
        - name: liberty-config
          mountPath: /etc/wlp/configmap
          readOnly: true
        {{- if $root.Values.image.serverOverridesConfigMapName }}
        - name: server-overrides-configmap
          mountPath: /config/configDropins/overrides/server-overrides.xml
          subPath: server-overrides.xml
          readOnly: true
        {{- end }}
        {{ if or $stateful (and $root.Values.ssl.enabled $root.Values.ssl.useClusterSSLConfiguration) }}
        {{ if and $root.Values.ssl.enabled $root.Values.ssl.useClusterSSLConfiguration -}}
        - name: keystores
          mountPath: /etc/wlp/config/keystore
          readOnly: true
        - name: truststores
          mountPath: /etc/wlp/config/truststore
          readOnly: true
        {{ end }}
        {{ if $root.Values.logs.persistTransactionLogs }}
        - mountPath: /output/tranlog
          name: {{ $root.Values.persistence.name | trunc 63 | lower | trimSuffix "-" | quote }}
          subPath: tranlog
        {{ end }}
        {{ if $root.Values.logs.persistLogs }}
        - mountPath: /logs
          name: {{ $root.Values.persistence.name | trunc 63 | lower | trimSuffix "-" | quote }}
          subPath: logs
        {{ end }}
        {{ end }}
{{- with $root.Values.image.extraVolumeMounts }}
{{ toYaml . | indent 8 }}
{{- end -}}
        resources:
          {{- if $root.Values.resources.constraints.enabled }}
          limits:
{{ toYaml $root.Values.resources.limits | indent 12 }}
          requests:
{{ toYaml $root.Values.resources.requests | indent 12 }}
          {{- end }}
{{- with $root.Values.pod.extraContainers }}
{{ toYaml . | indent 6}}
{{- end }}
      restartPolicy: "Always"
      terminationGracePeriodSeconds: 30
      dnsPolicy: "ClusterFirst"
  {{ if $stateful -}}
  volumeClaimTemplates:
  - metadata:
      name: {{ $root.Values.persistence.name | trunc 63 | lower | trimSuffix "-"  | quote }}
      labels:
        chart: "{{ $root.Chart.Name }}"
        app: {{ include "slt.utils.fullname" (list $root) }}
        release: "{{ $root.Release.Name }}"
        heritage: "{{ $root.Release.Service }}"
    spec:
      {{- if $root.Values.persistence.useDynamicProvisioning }}
      # if present, use the storageClassName from the values.yaml, else use the
      # default storageClass setup by kube Administrator
      # setting storageClassName to nil means use the default storage class
      storageClassName: {{ default nil $root.Values.persistence.storageClassName | quote }}
      {{- else }}
      # bind to an existing pv.
      # setting storageClassName to "" disables dynamic provisioning
      storageClassName: {{ default "" $root.Values.persistence.storageClassName | quote }}
      {{- if $root.Values.persistence.selector.label }}
      # use selectors in the binding process
      selector:
        matchExpressions:
          - {key: {{ $root.Values.persistence.selector.label }}, operator: In, values: [{{ $root.Values.persistence.selector.value }}]}
      {{- end }}
      {{- end }}
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: {{ $root.Values.persistence.size | quote }}
  {{- end }}
{{- end -}}

