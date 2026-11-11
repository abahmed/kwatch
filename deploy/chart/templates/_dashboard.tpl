{{/*
Dashboard settings. The chart never exposes the dashboard outside the
cluster: the kwatch.sh Dashboard menu, or you, point an Ingress, route
or Service at the ClusterIP Service <fullname>-dashboard.
  trustedProxies  proxies that end TLS and set X-Forwarded-*; any entry
                  (or tls) makes the dashboard https only
  tls             kwatch serves https itself from a kubernetes.io/tls Secret
*/}}

{{- define "kwatch.dashboardCheck" -}}
{{- $d := default dict .Values.dashboard -}}
{{- $tls := default dict $d.tls -}}
{{- if $d.enabled }}
{{- $hp := include "kwatch.healthPort" . | int }}
{{- $dp := $d.port | default 8061 | int }}
{{- if eq $dp $hp }}
{{- fail (printf "dashboard.port (%d) must differ from config.healthCheck.port (%d): kwatch refuses to start when both are equal" $dp $hp) }}
{{- end }}
{{- end }}
{{- end }}
