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

{{/*
Topology label of the StorageClass's CSI driver, when that driver is
registered on only some nodes and the class binds WaitForFirstConsumer. Such
a claim is provisioned for the node the scheduler picks; on a node without
the driver it never binds, and the Pod stays Pending. Nodes carry the
driver's own topology label only once the driver is registered there, so
requiring that label keeps kwatch where its volume can exist. Labels every
node has (kubernetes.io/*, topology.kubernetes.io/*) are skipped. The driver
is learned from the cluster: a CSI class names it as the provisioner; an
in-tree class is served by whatever driver provisioned a volume of that
class before, else by Kubernetes' in-tree-to-CSI translation. Empty when
the driver is everywhere, nowhere or unknown, when the class binds
Immediately, when the volume is not a claim, or when the render cannot look
the cluster up.
*/}}
{{- define "kwatch.storageTopologyKey" -}}
{{- if .Values.persistence.emptyDir -}}
{{- else if .Values.persistence.storageDriverTopologyKey -}}
{{- .Values.persistence.storageDriverTopologyKey -}}
{{- else if and .Values.persistence.pinToStorageDriverNodes (not .Values.persistence.existingClaim) -}}
{{- $class := "" -}}
{{- $provisioner := "" -}}
{{- $binding := "" -}}
{{- with (lookup "storage.k8s.io/v1" "StorageClass" "" "") -}}
{{- range .items -}}
{{- $ann := .metadata.annotations | default dict -}}
{{- $chosen := false -}}
{{- if $.Values.persistence.storageClass -}}
{{- $chosen = eq .metadata.name $.Values.persistence.storageClass -}}
{{- else -}}
{{- $chosen = or (eq (index $ann "storageclass.kubernetes.io/is-default-class" | default "") "true") (eq (index $ann "storageclass.beta.kubernetes.io/is-default-class" | default "") "true") -}}
{{- end -}}
{{- if $chosen -}}
{{- $class = .metadata.name -}}
{{- $provisioner = .provisioner -}}
{{- $binding = .volumeBindingMode | default "Immediate" -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- $registered := dict -}}
{{- $total := 0 -}}
{{- with (lookup "storage.k8s.io/v1" "CSINode" "" "") -}}
{{- range .items -}}
{{- $total = add1 $total -}}
{{- with .spec.drivers -}}
{{- range . -}}
{{- $_ := set $registered .name (append (index $registered .name | default list) .) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- $driver := "" -}}
{{- if hasKey $registered $provisioner -}}
{{- $driver = $provisioner -}}
{{- else if $provisioner -}}
{{- with (lookup "v1" "PersistentVolume" "" "") -}}
{{- range .items -}}
{{- if and (not $driver) (eq (.spec.storageClassName | default "") $class) .spec.csi -}}
{{- $driver = .spec.csi.driver -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- if not $driver -}}
{{- $translation := dict "kubernetes.io/aws-ebs" "ebs.csi.aws.com" "kubernetes.io/gce-pd" "pd.csi.storage.gke.io" "kubernetes.io/azure-disk" "disk.csi.azure.com" "kubernetes.io/azure-file" "file.csi.azure.com" "kubernetes.io/cinder" "cinder.csi.openstack.org" "kubernetes.io/vsphere-volume" "csi.vsphere.vmware.com" "kubernetes.io/portworx-volume" "pxd.portworx.com" "kubernetes.io/rbd" "rbd.csi.ceph.com" -}}
{{- $driver = index $translation $provisioner | default "" -}}
{{- end -}}
{{- end -}}
{{- $key := "" -}}
{{- if and $driver (eq $binding "WaitForFirstConsumer") (hasKey $registered $driver) -}}
{{- $entries := index $registered $driver -}}
{{- if lt (len $entries) $total -}}
{{- range $entries -}}
{{- with .topologyKeys -}}
{{- range . -}}
{{- if and (not $key) (not (hasPrefix "kubernetes.io/" .)) (not (hasPrefix "topology.kubernetes.io/" .)) (not (hasPrefix "node.kubernetes.io/" .)) -}}
{{- $key = . -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- $key -}}
{{- end -}}
{{- end -}}
