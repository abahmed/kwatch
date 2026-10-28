{{/*
Dashboard exposure. dashboard.expose picks how people reach it:
  local        kubectl port-forward only (http://localhost)
  service      the ClusterIP Service only, for your own front door
  ingress      an Ingress (nginx, AWS ALB, GKE, Azure AGIC, Tailscale, ngrok)
  gateway      a Gateway API HTTPRoute
  istio        an Istio VirtualService
  route        an OpenShift Route
  loadBalancer a LoadBalancer Service; kwatch serves TLS itself
  nodePort     a NodePort Service; kwatch serves TLS itself
  external     TLS ends at a proxy outside the cluster
Every shared way is https only. kwatch.dashboardCheck fails the render
with a plain message when a required value is missing.
*/}}

{{- define "kwatch.dashboard" -}}
{{- default dict .Values.dashboard | toYaml }}
{{- end }}

{{- define "kwatch.dashboardExpose" -}}
{{- $d := default dict .Values.dashboard -}}
{{- default "local" $d.expose -}}
{{- end }}

{{/* proxy: TLS ends at a proxy that sets X-Forwarded-Proto. */}}
{{- define "kwatch.dashboardProxied" -}}
{{- $e := include "kwatch.dashboardExpose" . -}}
{{- if has $e (list "ingress" "gateway" "istio" "route" "external") }}true{{ end -}}
{{- end }}

{{/* ownTLS: kwatch serves TLS on its port. */}}
{{- define "kwatch.dashboardOwnTLS" -}}
{{- $e := include "kwatch.dashboardExpose" . -}}
{{- if has $e (list "loadBalancer" "nodePort") }}true{{ end -}}
{{- end }}

{{- define "kwatch.dashboardMode" -}}
{{- if or (include "kwatch.dashboardProxied" .) (include "kwatch.dashboardOwnTLS" .) -}}
shared
{{- else -}}
local
{{- end -}}
{{- end }}

{{/* The Secret that holds the certificate kwatch serves itself. */}}
{{- define "kwatch.dashboardTLSSecret" -}}
{{- $tls := (default dict (default dict .Values.dashboard).tls) -}}
{{- default (printf "%s-dashboard-tls" (include "kwatch.fullname" .)) $tls.secretName -}}
{{- end }}

{{- define "kwatch.dashboardCheck" -}}
{{- $d := default dict .Values.dashboard -}}
{{- $e := include "kwatch.dashboardExpose" . -}}
{{- $ing := default dict $d.ingress -}}
{{- $tls := default dict $d.tls -}}
{{- $ranges := default list $d.allowedSourceRanges -}}
{{- $known := list "local" "service" "ingress" "gateway" "istio" "route" "loadBalancer" "nodePort" "external" -}}
{{- if not (has $e $known) }}
{{- fail (printf "dashboard.expose=%s is not one of %s" $e (join ", " $known)) }}
{{- end }}
{{- if and $ranges (has $e (list "local" "service" "gateway" "istio" "external")) }}
{{- fail (printf "dashboard.allowedSourceRanges is not supported with dashboard.expose=%s; limit sources in your gateway, mesh or proxy" $e) }}
{{- end }}
{{- if and (has $e (list "ingress" "gateway" "istio")) (not $d.host) (ne (default "" $ing.provider) "tailscale") }}
{{- fail (printf "dashboard.expose=%s needs dashboard.host, the https address people open" $e) }}
{{- end }}
{{- if eq $e "ingress" }}
{{- $p := default "" $ing.provider }}
{{- if not (has $p (list "" "nginx" "alb" "gce" "agic" "tailscale" "ngrok")) }}
{{- fail "dashboard.ingress.provider must be empty, nginx, alb, gce, agic, tailscale or ngrok" }}
{{- end }}
{{- if and $ranges (not (has $p (list "" "nginx" "alb"))) }}
{{- fail (printf "dashboard.allowedSourceRanges is not supported with dashboard.ingress.provider=%s; use the provider's own access rules" $p) }}
{{- end }}
{{- $alb := default dict $ing.alb }}
{{- $gce := default dict $ing.gce }}
{{- if and (eq $p "alb") (not $alb.certificateArn) }}
{{- fail "dashboard.ingress.provider=alb needs dashboard.ingress.alb.certificateArn (an ACM certificate); the ALB serves https only" }}
{{- end }}
{{- if and (has $p (list "" "nginx" "agic")) (not $ing.clusterIssuer) (not $ing.tlsSecretName) }}
{{- fail "dashboard.expose=ingress needs TLS: set dashboard.ingress.clusterIssuer (cert-manager) or dashboard.ingress.tlsSecretName (an existing certificate)" }}
{{- end }}
{{- if and (eq $p "gce") (not $gce.managedCertificate) (not $ing.clusterIssuer) (not $ing.tlsSecretName) }}
{{- fail "dashboard.ingress.provider=gce needs TLS: set dashboard.ingress.gce.managedCertificate=true, dashboard.ingress.clusterIssuer or dashboard.ingress.tlsSecretName" }}
{{- end }}
{{- end }}
{{- if eq $e "gateway" }}
{{- if not (default dict $d.gateway).name }}
{{- fail "dashboard.expose=gateway needs dashboard.gateway.name, the Gateway with an https listener" }}
{{- end }}
{{- end }}
{{- if and (eq $e "istio") (not (default dict $d.istio).gateway) }}
{{- fail "dashboard.expose=istio needs dashboard.istio.gateway (namespace/name of a Gateway with an https server)" }}
{{- end }}
{{- if and (eq $e "external") (not (default dict $d.external).trustProxy) }}
{{- fail "dashboard.expose=external needs dashboard.external.trustProxy=true: confirm that a proxy you control terminates TLS and sets X-Forwarded-Proto" }}
{{- end }}
{{- if include "kwatch.dashboardOwnTLS" . }}
{{- $ways := 0 }}
{{- if $tls.secretName }}{{ $ways = add1 $ways }}{{ end }}
{{- if $tls.issuer }}{{ $ways = add1 $ways }}{{ end }}
{{- if $tls.selfSigned }}{{ $ways = add1 $ways }}{{ end }}
{{- if ne (int $ways) 1 }}
{{- fail (printf "dashboard.expose=%s makes kwatch serve TLS itself: set exactly one of dashboard.tls.secretName, dashboard.tls.issuer or dashboard.tls.selfSigned=true" $e) }}
{{- end }}
{{- end }}
{{- end }}
