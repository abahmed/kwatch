{{/*
  Kubernetes common labels
*/}}

{{- define "kwatch.commonLabels" -}}
app.kubernetes.io/name: kwatch
app.kubernetes.io/version: {{ .Chart.AppVersion }}
{{- end -}}


{{- define "kwatch.matchLabels" -}}
app.kubernetes.io/name: kwatch
{{- end -}}
