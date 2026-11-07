package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

var joinAt = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func sharedConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "api-config", Namespace: "shop"},
		Data: map[string]string{
			"DB_HOST":     "db.example.com:5432",
			"DB_PASSWORD": "pw.example.com:1",
			"LOG_LEVEL":   "debug",
			"BANNER":      "welcome to cache.example.com:6379 today",
			"LOCAL":       "localhost:8080",
		},
	}
}

func readerPod(name string, env ...corev1.EnvVar) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "shop"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "app", Env: env}}},
	}
}

func keyRef(name, key string) corev1.EnvVar {
	return corev1.EnvVar{Name: "X_" + key, ValueFrom: &corev1.EnvVarSource{
		ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: name},
			Key:                  key}}}
}

type joinHarness struct {
	t     *testing.T
	model *inventory.Model
	pods  *Translator
	maps  *Translator
}

func newJoinHarness(t *testing.T) *joinHarness {
	join := NewConfigJoin()
	return &joinHarness{t: t, model: inventory.NewModel(inventory.Options{}),
		pods: NewTranslator(PodSchema{}).WithConfigJoin(join),
		maps: NewTranslator(NewConfigMapSchema(NewDigester(nil))).
			WithConfigJoin(join)}
}

func (h *joinHarness) apply(observations []inventory.Observation) {
	for _, o := range observations {
		_, err := h.model.Apply(o)
		require.NoError(h.t, err)
	}
}

func (h *joinHarness) addMap(cm *corev1.ConfigMap) {
	out, err := NewDigester(nil).HashConfigMapData(cm)
	require.NoError(h.t, err)
	h.apply(h.maps.Added(out, true, joinAt))
}

func (h *joinHarness) calls(pod string) []string {
	var out []string
	id := inventory.CoreID(KindPod, "shop", pod)
	for _, t := range h.model.Related(id, inventory.Calls,
		inventory.Outgoing) {
		out = append(out, t.Name)
	}
	return out
}

func TestConfigTargetsKeepOnlyEndpointsOfPlainKeys(t *testing.T) {
	got := configMapTargets(sharedConfigMap())
	assert.Equal(t, "DB_HOST=db.example.com:5432", targetsText(got))
}

func TestConfigMapTransformKeepsNoRawValue(t *testing.T) {
	out, err := NewDigester(nil).HashConfigMapData(sharedConfigMap())
	require.NoError(t, err)
	cm := out.(*corev1.ConfigMap)
	assert.NotContains(t, cm.Data["DB_HOST"], "example.com")
	assert.Equal(t, "DB_HOST=db.example.com:5432",
		cm.Annotations[configTargetsAnnotation])
}

func TestConfigJoinGivesReadersAnEdgeInAnyOrder(t *testing.T) {
	h := newJoinHarness(t)
	reader := readerPod("api-1", keyRef("api-config", "DB_HOST"))
	other := readerPod("web-1", keyRef("api-config", "LOG_LEVEL"))
	h.apply(h.pods.Added(reader, true, joinAt))
	h.apply(h.pods.Added(other, true, joinAt))
	assert.Empty(t, h.calls("api-1"))

	h.addMap(sharedConfigMap())

	assert.Equal(t, []string{"db.example.com:5432"}, h.calls("api-1"))
	assert.Empty(t, h.calls("web-1"), "reads another key")
	endpoint := inventory.CoreID(KindExternalEndpoint, "",
		"db.example.com:5432")
	e, ok := h.model.Entity(endpoint)
	require.True(t, ok)
	assert.Equal(t, "shop/api-config#DB_HOST",
		e.Attributes[AttrEndpointSources].Value.AsText())
}

func TestConfigJoinFollowsThePodArrivingLast(t *testing.T) {
	h := newJoinHarness(t)
	h.addMap(sharedConfigMap())
	whole := readerPod("api-1")
	whole.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
		ConfigMapRef: &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "api-config"}}}}
	h.apply(h.pods.Added(whole, false, joinAt))
	assert.Equal(t, []string{"db.example.com:5432"}, h.calls("api-1"))
}

func TestConfigJoinDropsEdgesWhenTheValueNoLongerNamesAHost(t *testing.T) {
	h := newJoinHarness(t)
	h.apply(h.pods.Added(readerPod("api-1", keyRef("api-config",
		"DB_HOST")), true, joinAt))
	h.addMap(sharedConfigMap())
	require.NotEmpty(t, h.calls("api-1"))

	changed := sharedConfigMap()
	changed.Data["DB_HOST"] = "db-1"
	out, err := NewDigester(nil).HashConfigMapData(changed)
	require.NoError(t, err)
	h.apply(h.maps.Updated(sharedConfigMap(), out, joinAt))

	assert.Empty(t, h.calls("api-1"))
}
