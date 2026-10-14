package app

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// managedEntry is one repeated managedFields chunk.
const managedEntry = `"f:spec":{"f:containers":` +
	`{"k:{\"name\":\"app\"}":{".":{}}}},`

// realisticPod is a pod as an API server returns it: three containers
// (one init), 45 environment variables, three managedFields entries with
// field sets, six volumes, probes, lifecycle hooks and the usual
// annotations. Every string is freshly built, as decoding makes them.
func realisticPod(i int, now time.Time) *corev1.Pod {
	ns := fmt.Sprintf("ns-%d", i%50)
	name := fmt.Sprintf("checkout-%d-7f9c6b5d4-x%05d", i%900, i)
	yes := true
	meta := metav1.ObjectMeta{
		Name: name, Namespace: ns, UID: types.UID(fmt.Sprintf("uid-%d", i)),
		ResourceVersion:   fmt.Sprintf("%d", 100000+i),
		CreationTimestamp: metav1.NewTime(now.Add(-48 * time.Hour)),
		Labels: map[string]string{
			"app": "checkout", "pod-template-hash": "7f9c6b5d4",
			"app.kubernetes.io/name": "checkout", "team": "shop",
		},
		Annotations: map[string]string{
			"kubectl.kubernetes.io/restartedAt": "2026-09-30T10:00:00Z",
			"prometheus.io/scrape":              "true",
			"prometheus.io/port":                "9090",
			"cni.projectcalico.org/podIP": fmt.Sprintf(
				"10.1.%d.%d/32", i/250%250, i%250),
			"kubectl.kubernetes.io/last-applied-configuration": strings.Repeat(
				"x", 1500),
		},
		OwnerReferences: memOwner("ReplicaSet", fmt.Sprintf("checkout-%d-7f", i%900)),
	}
	for m := 0; m < 3; m++ {
		at := metav1.NewTime(now.Add(-time.Duration(m+1) * time.Hour))
		meta.ManagedFields = append(meta.ManagedFields, metav1.ManagedFieldsEntry{
			Manager: fmt.Sprintf("manager-%d", m), Operation: "Update",
			APIVersion: "v1", Time: &at,
			FieldsV1: &metav1.FieldsV1{Raw: []byte(
				`{"f:metadata":{"f:labels":{".":{},"f:app":{}}},` +
					strings.Repeat(managedEntry, 12) + `"f:x":{}}`)},
		})
	}
	return &corev1.Pod{
		ObjectMeta: meta,
		Spec: corev1.PodSpec{
			NodeName:       fmt.Sprintf("node-%d", i%memNodes),
			InitContainers: []corev1.Container{realisticInit(i)},
			Containers: []corev1.Container{
				realisticApp(i, yes), realisticSidecar(i)},
			Volumes: realisticVolumes(i),
		},
		Status: memPodStatus(false, now),
	}
}

func realisticInit(i int) corev1.Container {
	return corev1.Container{
		Name: "migrate", Image: "registry.example/checkout-migrate:2026.09",
		Command: []string{
			"/bin/migrate", "--verbose", "--target=latest"},
		ImagePullPolicy:        corev1.PullIfNotPresent,
		TerminationMessagePath: "/dev/termination-log",
		Env: []corev1.EnvVar{
			{Name: "DB_HOST", Value: "orders-db"},
			{Name: "STEP", Value: fmt.Sprintf("migration-%d", i%7)}},
	}
}

func realisticApp(i int, yes bool) corev1.Container {
	env := make([]corev1.EnvVar, 0, 45)
	for e := 0; e < 33; e++ {
		env = append(env, corev1.EnvVar{
			Name:  fmt.Sprintf("CHECKOUT_FEATURE_FLAG_%02d", e),
			Value: fmt.Sprintf("enabled-for-region-eu-west-%d-%d", e, i%5)})
	}
	for e := 0; e < 6; e++ {
		env = append(env, corev1.EnvVar{
			Name: fmt.Sprintf("SECRET_%d", e), ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "checkout-creds"},
					Key: fmt.Sprintf("key-%d", e)}}})
	}
	for e := 0; e < 4; e++ {
		env = append(env, corev1.EnvVar{
			Name: fmt.Sprintf("POD_FIELD_%d", e), ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1", FieldPath: "metadata.name"}}})
	}
	env = append(env,
		corev1.EnvVar{Name: "DATABASE_URL",
			Value: "postgres://app:pw@db.example.com:5432/orders"},
		corev1.EnvVar{Name: "CACHE_ADDR", Value: "redis:6379"})
	return corev1.Container{
		Name: "app", Image: "registry.example/checkout:2026.09.30",
		Command: []string{"/srv/checkout"},
		Args: []string{"--config=/etc/checkout/config.yaml", "--log-level=info",
			"--listen=:8080", "--metrics=:9090", "--shutdown-grace=30s"},
		Env: env,
		EnvFrom: []corev1.EnvFromSource{{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "checkout-config"}}}},
		Ports: []corev1.ContainerPort{{
			Name: "http", ContainerPort: 8080, Protocol: "TCP"}},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("512Mi")},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("250m")}},
		ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{Path: "/ready"}}},
		LivenessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{Path: "/live"}}},
		Lifecycle: &corev1.Lifecycle{PreStop: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{Command: []string{"sleep", "10"}}}},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "config", MountPath: "/etc/checkout", ReadOnly: yes},
			{Name: "scratch", MountPath: "/tmp"},
			{Name: "kube-api-access", ReadOnly: yes,
				MountPath: "/var/run/secrets/kubernetes.io/serviceaccount"}},
		ImagePullPolicy:          corev1.PullIfNotPresent,
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
	}
}

func realisticSidecar(i int) corev1.Container {
	return corev1.Container{
		Name: "proxy", Image: "registry.example/proxy:1.30",
		Args: []string{
			"--upstream=127.0.0.1:8080", "--admin=:15000"},
		Env: []corev1.EnvVar{{
			Name: "PROXY_ID", Value: fmt.Sprintf("p-%d", i)}},
		ImagePullPolicy:        corev1.PullIfNotPresent,
		TerminationMessagePath: "/dev/termination-log",
	}
}

func realisticVolumes(i int) []corev1.Volume {
	expiry := int64(3607)
	return []corev1.Volume{
		{Name: "config", VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "checkout-config"},
				Items: []corev1.KeyToPath{{Key: "config.yaml", Path: "config.yaml"}}}}},
		{Name: "scratch", VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		{Name: "logs", VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{Path: "/var/log/checkout"}}},
		{Name: "kube-api-access", VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				Sources: []corev1.VolumeProjection{
					{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
						Path: "token", ExpirationSeconds: &expiry}},
					{ConfigMap: &corev1.ConfigMapProjection{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "kube-root-ca.crt"},
						Items: []corev1.KeyToPath{{Key: "ca.crt", Path: "ca.crt"}}}},
					{DownwardAPI: &corev1.DownwardAPIProjection{
						Items: []corev1.DownwardAPIVolumeFile{{Path: "namespace",
							FieldRef: &corev1.ObjectFieldSelector{
								APIVersion: "v1", FieldPath: "metadata.namespace"}}}}}}}}},
	}
}
