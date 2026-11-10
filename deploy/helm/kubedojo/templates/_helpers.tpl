{{- define "kubedojo.name" -}}{{ .Release.Name | trunc 50 | trimSuffix "-" }}{{- end -}}
{{- define "kubedojo.labels" -}}
app.kubernetes.io/name: kubedojo
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}
{{- define "kubedojo.selector" -}}
app.kubernetes.io/name: kubedojo
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: server
{{- end -}}
{{- define "kubedojo.otlpEndpoint" -}}
{{- if .Values.telemetry.otlpEndpoint -}}{{ .Values.telemetry.otlpEndpoint }}
{{- else if .Values.lgtm.enabled -}}http://{{ include "kubedojo.name" . }}-lgtm:4318
{{- end -}}
{{- end -}}
