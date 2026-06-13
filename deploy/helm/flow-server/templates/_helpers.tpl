{{/*
Expand the name of the chart.
*/}}
{{- define "flow-server.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
Truncated at 63 chars because some Kubernetes name fields are limited to that.
If release name contains chart name we strip the duplicate.
*/}}
{{- define "flow-server.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Chart name and version, joined for the helm.sh/chart label.
*/}}
{{- define "flow-server.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to every resource.
*/}}
{{- define "flow-server.labels" -}}
helm.sh/chart: {{ include "flow-server.chart" . }}
{{ include "flow-server.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels — the subset that goes into a Service/Deployment selector.
Must stay stable across releases or the Deployment update will fail.
*/}}
{{- define "flow-server.selectorLabels" -}}
app.kubernetes.io/name: {{ include "flow-server.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
ServiceAccount name. Honors serviceAccount.create + serviceAccount.name.
The skeleton has serviceAccount.create=false; Task 4 may enable it.
*/}}
{{- define "flow-server.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "flow-server.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Container image reference. Uses .Values.image.tag if set, otherwise the
chart's appVersion. Centralizing the logic here keeps the Deployment readable.
*/}}
{{- define "flow-server.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag -}}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}
