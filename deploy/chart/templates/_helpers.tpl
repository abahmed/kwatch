{{/*
Expand the name of the chart.
*/}}
{{- define "kwatch.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kwatch.fullname" -}}
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
{{- define "kwatch.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kwatch.labels" -}}
helm.sh/chart: {{ include "kwatch.chart" . }}
{{ include "kwatch.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kwatch.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kwatch.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Shared LLM sidecar container spec used by both plain-container and native-sidecar forms.
*/}}
{{- define "kwatch.llmContainer" -}}
image: "{{ .Values.llm.repository }}:{{ .Values.llm.tag }}"
imagePullPolicy: IfNotPresent
env:
  - { name: OLLAMA_HOST, value: "127.0.0.1:11434" }
  - { name: OLLAMA_MODELS, value: /models }
  - { name: OLLAMA_NUM_PARALLEL, value: "1" }
  - { name: OLLAMA_MAX_LOADED_MODELS, value: "1" }
  - { name: OLLAMA_KEEP_ALIVE, value: "5m" }
startupProbe:
  exec: { command: ["/bin/sh","-c","ollama list >/dev/null 2>&1"] }
  failureThreshold: 30
  periodSeconds: 2
readinessProbe:
  exec: { command: ["/bin/sh","-c","ollama list | grep -q kwatch-triage"] }
  periodSeconds: 10
livenessProbe:
  exec: { command: ["/bin/sh","-c","ollama list >/dev/null 2>&1"] }
  periodSeconds: 30
  failureThreshold: 3
resources:
  requests: { cpu: "500m", memory: "1Gi" }
  limits:   { cpu: "2", memory: "2Gi" }
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  runAsGroup: 1000
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: false
  capabilities: { drop: ["ALL"] }
  seccompProfile: { type: RuntimeDefault }
{{- end -}}
