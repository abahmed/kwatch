{{/*
Dashboard settings. The chart never exposes the dashboard outside the
cluster: the kwatch.sh Dashboard menu, or you, point an Ingress, route
or Service at the ClusterIP Service <release>-dashboard.
  mode       local (port-forward) or shared (https only, Secure cookie)
  trustProxy TLS ends at a proxy that sets X-Forwarded-Proto
  tls        kwatch serves https itself from a kubernetes.io/tls Secret
*/}}

{{- define "kwatch.dashboardCheck" -}}
{{- $d := default dict .Values.dashboard -}}
{{- $tls := default dict $d.tls -}}
{{- if and (eq (default "local" $d.mode) "shared") (not $d.trustProxy) (not $tls.secretName) }}
{{- fail "dashboard.mode=shared is https only: set dashboard.trustProxy=true (TLS ends at your Ingress or proxy) or dashboard.tls.secretName (kwatch serves https itself)" }}
{{- end }}
{{- if $d.enabled }}
{{- $hp := include "kwatch.healthPort" . | int }}
{{- $dp := $d.port | default 8061 | int }}
{{- if eq $dp $hp }}
{{- fail (printf "dashboard.port (%d) must differ from config.healthCheck.port (%d): kwatch refuses to start when both are equal" $dp $hp) }}
{{- end }}
{{- end }}
{{- end }}
