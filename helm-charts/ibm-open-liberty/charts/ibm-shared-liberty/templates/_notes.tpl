{{- /*
Shared Liberty Templates (SLT)

Notes templates:
  - slt.notes.application.url

Usage of "slt.notes.*" requires the following line be include at 
the begining of template:
{{- include "slt.config.init" (list . "slt.chart.config.values") -}}
 
********************************************************************
*** This file is shared across multiple charts, and changes must be 
*** made in centralized and controlled process. 
*** Do NOT modify this file with chart specific changes.
*****************************************************************
*/ -}}

{{- define "slt.notes.application.url" -}}
  {{- $params := . -}}
  {{- $root := first $params -}}
{{ include "slt.utils.isICP" (list $root) }}
{{- if $root.Values.service.enabled }}

{{- if $root.Values.ingress.enabled }}
+ Get the application URL by running these commands:
  {{- if $root.Values.ingress.host }}
  export INGRESS_HOST=$(kubectl get ing --namespace {{ $root.Release.Namespace }} {{ include "slt.utils.fullname" (list $root) }} -o jsonpath="{.spec.rules[0].host}")
  export APP_PATH={{ $root.Values.ingress.path }}
  {{- if $root.Values.ssl.enabled }}
  echo https://$INGRESS_HOST$APP_PATH
  {{- else }}
  echo http://$INGRESS_HOST$APP_PATH
  {{- end }}
  {{- else }}
  export INGRESS_IP=$(kubectl get nodes -l proxy=true -o jsonpath="{.items[0].status.addresses[?(@.type==\"Hostname\")].address}")
  export INGRESS_PORT=$(kubectl -n kube-system get DaemonSet nginx-ingress-controller -o jsonpath="{.spec.template.spec.containers[0].ports[?(@.name==\"{{ if $root.Values.ssl.enabled }}https{{ else }}http{{ end }}\")].hostPort}")
  export APP_PATH={{ $root.Values.ingress.path }}
  {{- if $root.Values.ssl.enabled }}
  echo https://$INGRESS_IP:$INGRESS_PORT$APP_PATH
  {{- else }}
  echo http://$INGRESS_IP:$INGRESS_PORT$APP_PATH
  {{- end }}
  {{- end -}}
{{- else if contains "NodePort" $root.Values.service.type }}
  {{- if $root.Values.isICP }}
+ Get the application URL by running these commands:
  export NODE_PORT=$(kubectl get --namespace {{ $root.Release.Namespace }} -o jsonpath="{.spec.ports[0].nodePort}" services {{ include "slt.utils.fullname" (list $root) }})
  export NODE_IP=$(kubectl get nodes -l proxy=true -o jsonpath="{.items[0].status.addresses[?(@.type==\"Hostname\")].address}")
  {{- if $root.Values.ssl.enabled }}
  echo https://$NODE_IP:$NODE_PORT
  {{- else }}
  echo http://$NODE_IP:$NODE_PORT
  {{- end }}
  {{- else }}
+ If you are running on IBM Cloud Kubernetes Service, get the application address by running these commands:
  ibmcloud cs workers $(kubectl config current-context)
  export NODE_IP=<Worker node public IP from the first command>
  export NODE_PORT=$(kubectl get --namespace {{ $root.Release.Namespace }} -o jsonpath="{.spec.ports[0].nodePort}" services {{ include "slt.utils.fullname" (list $root) }})
  {{- if $root.Values.ssl.enabled }}
  echo Application Address: https://$NODE_IP:$NODE_PORT
  {{- else }}
  echo Application Address: http://$NODE_IP:$NODE_PORT
  {{- end }}

Otherwise, run the following commands:
  export NODE_IP=$(kubectl get nodes -l proxy=true -o jsonpath="{.items[0].status.addresses[?(@.type==\"Hostname\")].address}")
  export NODE_PORT=$(kubectl get --namespace {{ $root.Release.Namespace }} -o jsonpath="{.spec.ports[0].nodePort}" services {{ include "slt.utils.fullname" (list $root) }})
  {{- if $root.Values.ssl.enabled }}
  echo Application Address: https://$NODE_IP:$NODE_PORT
  {{- else }}
  echo Application Address: http://$NODE_IP:$NODE_PORT
  {{- end }}

  {{- end }}
{{- else if contains "ClusterIP"  $root.Values.service.type }}
+ Get the application URL by running these commands:
  export POD_NAME=$(kubectl get pods --namespace {{ $root.Release.Namespace }} -l "app={{ include "slt.utils.fullname" (list $root) }}" -o jsonpath="{.items[0].metadata.name}")
  {{- if $root.Values.ssl.enabled }}
  echo "Visit https://127.0.0.1:8080 to use your application"
  {{- else }}
  echo "Visit http://127.0.0.1:8080 to use your application"
  {{- end }}
  kubectl port-forward $POD_NAME 8080:{{ $root.Values.service.targetPort }}
{{- end }}
{{- end }}

{{- if $root.Values.jmsService.enabled }}
{{- if contains "NodePort" $root.Values.jmsService.type }}

-----

  {{- if $root.Values.isICP }}
+ Get the JMS address by running these commands:
  export JMS_NODE_PORT=$(kubectl get --namespace {{ $root.Release.Namespace }} -o jsonpath="{.spec.ports[0].nodePort}" services {{ include "slt.utils.fullname" (list $root) }}-jms)
  export NODE_IP=$(kubectl get nodes -l proxy=true -o jsonpath="{.items[0].status.addresses[?(@.type==\"Hostname\")].address}")
  echo JMS Address: $NODE_IP:$JMS_NODE_PORT
  {{- else }}
+ If you are running on IBM Cloud Kubernetes Service, get the JMS address by running these commands:
  ibmcloud cs workers $(kubectl config current-context)
  export NODE_IP=<Worker node public IP from the first command>
  export JMS_NODE_PORT=$(kubectl get --namespace {{ $root.Release.Namespace }} -o jsonpath="{.spec.ports[0].nodePort}" services {{ include "slt.utils.fullname" (list $root) }}-jms)
  echo JMS Address: $NODE_IP:$JMS_NODE_PORT

Otherwise, run the following commands:
  export NODE_IP=$(kubectl get nodes -l proxy=true -o jsonpath="{.items[0].status.addresses[?(@.type==\"Hostname\")].address}")
  export JMS_NODE_PORT=$(kubectl get --namespace {{ $root.Release.Namespace }} -o jsonpath="{.spec.ports[0].nodePort}" services {{ include "slt.utils.fullname" (list $root) }}-jms)
  echo JMS Address: $NODE_IP:$JMS_NODE_PORT
  {{- end }}

{{- else if contains "ClusterIP"  $root.Values.jmsService.type }}

-----

+ Get the JMS address by running these commands where clients run:
  export JMS_POD_NAME=$(kubectl get pods --namespace {{ $root.Release.Namespace }} -l "app={{ include "slt.utils.fullname" (list $root) }}" -o jsonpath="{.items[0].metadata.name}")
  echo JMS Address: 127.0.0.1:8081
  kubectl port-forward $JMS_POD_NAME 8081:{{ $root.Values.jmsService.targetPort }}
{{- end }}
{{- end }}

{{- if $root.Values.iiopService.enabled }}
{{- if contains "NodePort" $root.Values.iiopService.type }}

-----

  {{- if $root.Values.isICP }}
+ Get the IIOP address by running these commands:
  export IIOP_NODE_PORT=$(kubectl get --namespace {{ $root.Release.Namespace }} -o jsonpath="{.spec.ports[?(@.port=={{ $root.Values.iiopService.nonSecureTargetPort }})].nodePort}" services {{ include "slt.utils.fullname" (list $root) }}-iiop)
  export NODE_IP=$(kubectl get nodes -l proxy=true -o jsonpath="{.items[0].status.addresses[?(@.type==\"Hostname\")].address}")
  echo IIOP Address: $NODE_IP:$IIOP_NODE_PORT
  {{- else }}
+ If you are running on IBM Cloud Kubernetes Service, get the IIOP address by running these commands:
  ibmcloud cs workers $(kubectl config current-context)
  export NODE_IP=<Worker node public IP from the first command>
  export IIOP_NODE_PORT=$(kubectl get --namespace {{ $root.Release.Namespace }} -o jsonpath="{.spec.ports[?(@.port=={{ $root.Values.iiopService.nonSecureTargetPort }})].nodePort}" services {{ include "slt.utils.fullname" (list $root) }}-iiop)
  echo IIOP Address: $NODE_IP:$IIOP_NODE_PORT

Otherwise, run the following commands:
  export NODE_IP=$(kubectl get nodes -l proxy=true -o jsonpath="{.items[0].status.addresses[?(@.type==\"Hostname\")].address}")
  export IIOP_NODE_PORT=$(kubectl get --namespace {{ $root.Release.Namespace }} -o jsonpath="{.spec.ports[?(@.port=={{ $root.Values.iiopService.nonSecureTargetPort }})].nodePort}" services {{ include "slt.utils.fullname" (list $root) }}-iiop)
  echo IIOP Address: $NODE_IP:$IIOP_NODE_PORT
  {{- end }}
{{- else if contains "ClusterIP"  $root.Values.iiopService.type }}

-----

+ Get the IIOP address by running these commands where clients run:
  export IIOP_POD_NAME=$(kubectl get pods --namespace {{ $root.Release.Namespace }} -l "app={{ include "slt.utils.fullname" (list $root) }}" -o jsonpath="{.items[0].metadata.name}")
  echo IIOP Address: 127.0.0.1:8082
  kubectl port-forward $IIOP_POD_NAME 8082:{{ $root.Values.iiopService.nonSecureTargetPort }}
{{- end }}
{{- end }}
{{- end -}}

