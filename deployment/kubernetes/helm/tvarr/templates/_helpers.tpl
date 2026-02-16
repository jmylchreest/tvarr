{{/*
Expand the name of the chart.
*/}}
{{- define "tvarr.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "tvarr.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "tvarr.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "tvarr.labels" -}}
helm.sh/chart: {{ include "tvarr.chart" . }}
{{ include "tvarr.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "tvarr.selectorLabels" -}}
app.kubernetes.io/name: {{ include "tvarr.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "tvarr.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "tvarr.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the PVC name
*/}}
{{- define "tvarr.pvcName" -}}
{{- if .Values.persistence.existingClaim }}
{{- .Values.persistence.existingClaim }}
{{- else }}
{{- include "tvarr.fullname" . }}-data
{{- end }}
{{- end }}

{{/*
Return the coordinator image repository based on mode.
In "distributed" mode, uses the lightweight coordinator-only image.
In "aio" mode (default), uses the all-in-one image with local FFmpeg.
User can override with image.repository.
*/}}
{{- define "tvarr.coordinatorImage" -}}
{{- if .Values.image.repository }}
{{- .Values.image.repository }}
{{- else if eq (default "aio" .Values.mode) "distributed" }}
{{- "ghcr.io/jmylchreest/tvarr-coordinator" }}
{{- else }}
{{- "ghcr.io/jmylchreest/tvarr" }}
{{- end }}
{{- end }}

{{/*
Return whether transcoder workers should be deployed.
In "distributed" mode, always true. Otherwise, check transcoder.enabled or legacy ffmpegd.enabled.
*/}}
{{- define "tvarr.transcoderEnabled" -}}
{{- if eq (default "aio" .Values.mode) "distributed" }}
{{- true }}
{{- else if .Values.transcoder.enabled }}
{{- true }}
{{- else if .Values.ffmpegd.enabled }}
{{- true }}
{{- else }}
{{- false }}
{{- end }}
{{- end }}

{{/*
Return whether gRPC should be enabled.
True if TVARR_GRPC_ENABLED is "true" or transcoders are enabled.
*/}}
{{- define "tvarr.grpcEnabled" -}}
{{- if eq (default "true" .Values.env.TVARR_GRPC_ENABLED) "true" }}
{{- true }}
{{- else if eq (include "tvarr.transcoderEnabled" .) "true" }}
{{- true }}
{{- else }}
{{- false }}
{{- end }}
{{- end }}
