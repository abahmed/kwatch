{{/*
  Kubernetes common labels
*/}}

{{- define "kwatch.commonLabels" -}}
app.kubernetes.io/name: kwatch
app.kubernetes.io/version: {{ .Chart.AppVersion }}
{{- end -}}

