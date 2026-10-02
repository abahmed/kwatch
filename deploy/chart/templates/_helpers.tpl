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
Tolerations: user entries first, then each default whose key and effect
are not already covered by a user entry, so custom tolerations never
remove fast failover. A user entry with no effect tolerates every effect
of its key and so replaces the defaults for that key.
*/}}
{{- define "kwatch.tolerations" -}}
{{- $seen := dict -}}
{{- $out := list -}}
{{- range .Values.tolerations -}}
{{- if .key }}{{- $_ := set $seen (printf "%s|%s" .key (.effect | default "*")) true -}}{{- end -}}
{{- $out = append $out . -}}
{{- end -}}
{{- range .Values.defaultTolerations -}}
{{- $exact := printf "%s|%s" .key (.effect | default "*") -}}
{{- $any := printf "%s|*" .key -}}
{{- if not (or (hasKey $seen $exact) (hasKey $seen $any)) -}}{{- $out = append $out . -}}{{- end -}}
{{- end -}}
{{- if $out }}{{ toYaml $out }}{{- end -}}
{{- end }}

{{/*
Health port: the port kwatch's health server listens on, taken from
config.healthCheck.port (kwatch's own default is 8060). The liveness and
readiness probes need the health server, so disabling it is rejected.
With configSecretName the chart cannot read the Secret: keep
config.healthCheck.port in values equal to the port in the Secret.
*/}}
{{- define "kwatch.healthPort" -}}
{{- $config := default dict .Values.config -}}
{{- $health := default dict (index $config "healthCheck") -}}
{{- if and (hasKey $health "enabled") (not $health.enabled) -}}
{{- fail "config.healthCheck.enabled=false is not supported: the liveness and readiness probes need the health server" -}}
{{- end -}}
{{- $health.port | default 8060 -}}
{{- end }}

{{/*
Leader-election Lease name. The Role grants get and update on this name
only, and the deployment passes it to kwatch.
*/}}
{{- define "kwatch.leaseName" -}}
{{- printf "%s-leader" (include "kwatch.fullname" .) -}}
{{- end }}

{{/*
watch.secrets as "true" or "false". Missing means true. hasKey is used
because Helm's default treats false as unset.
*/}}
{{- define "kwatch.watchSecrets" -}}
{{- $watch := default dict .Values.watch -}}
{{- if and (hasKey $watch "secrets") (not $watch.secrets) -}}
false
{{- else -}}
true
{{- end -}}
{{- end }}

{{/*
Name of the cluster-scoped ClusterRole and ClusterRoleBinding. They have
no namespace, so the release namespace is part of the name: two releases
with the same name in different namespaces must not share one role.
*/}}
{{- define "kwatch.clusterRoleName" -}}
{{- $fullname := include "kwatch.fullname" . -}}
{{- printf "%s-%s" $fullname .Release.Namespace | trunc 253 | trimSuffix "-" -}}
{{- end }}
