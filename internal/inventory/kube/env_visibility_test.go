package kube

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

func envDiff(name, before, after string) (string, string) {
	fields := boundFields(envChanges("containers[app]",
		[]corev1.EnvVar{{Name: name, Value: before}},
		[]corev1.EnvVar{{Name: name, Value: after}}))
	return fields[0].Before, fields[0].After
}

// Names that hide a credential must never show a value, whatever the
// name looks like.
func TestEnvChangesHideEveryNameThatIsNotAllowListed(t *testing.T) {
	names := []string{"DB_PASS", "DB_PWD", "MYSQL_PWD", "SMTP_PASS",
		"PGPASS", "SALT", "JWT_SIGNING", "SESSION_SIGNING", "BEARER",
		"CREDS", "PASSPHRASE", "PRIVATE_PEM", "OTP_SEED", "DB_PASSWORD",
		"API_KEY", "DATABASE_URL", "ENCRYPTION_IV", "MASTER_PIN",
		"AUTH_HEADER", "BASIC", "COOKIE_SECRET", "SECRET_HOST",
		"WEBHOOK_URL", "FOO", "USERNAME", "HOSTNAME"}
	for _, name := range names {
		before, after := envDiff(name, "hunter2-old-9x", "hunter2-new-7y")
		assert.Equal(t, "changed", before, name)
		assert.Equal(t, "changed", after, name)
	}
}

func TestEnvChangesHideAWebhookWithASecretPath(t *testing.T) {
	hook := "https://hooks.example.com/services/T000/B000/XXXXXXXXXXXX"
	for _, name := range []string{"WEBHOOK_URL", "NOTIFY_URL", "API_URL"} {
		before, _ := envDiff(name, hook, "")
		assert.Equal(t, "changed", before, name)
	}
}

func TestEnvChangesShowTheAllowListedNames(t *testing.T) {
	cases := []struct{ name, before, after string }{
		{"DB_HOST", "db-old.internal", "db-new.internal"},
		{"LOG_LEVEL", "info", "debug"},
		{"PORT", "8080", "9090"},
		{"API_URL", "http://api:8080", "http://api-v2:8080"},
		{"AWS_REGION", "eu-west-1", "eu-west-2"},
		{"APP_ENV", "staging", "prod"},
		{"FEATURE_ENABLED", "false", "true"},
		{"REQUEST_TIMEOUT", "30s", "60s"},
		{"WORKER_REPLICAS", "2", "4"},
		{"RUN_MODE", "fast", "safe"},
		{"LISTEN_ADDR", ":8080", "0.0.0.0:9090"},
		{"logLevel", "info", "debug"},
	}
	for _, c := range cases {
		before, after := envDiff(c.name, c.before, c.after)
		assert.Equal(t, c.before, before, c.name)
		assert.Equal(t, c.after, after, c.name)
	}
}

func TestEnvChangesHideValuesThatDoNotLookPlain(t *testing.T) {
	for _, value := range []string{"user:pw@host", "https://u:p@h/x",
		"a b", "p@ss", strings.Repeat("a", 65), "x?y=1", "ab%40"} {
		before, _ := envDiff("DB_HOST", value, "")
		assert.Equal(t, "changed", before, value)
	}
}

func TestEnvChangesKeepReferencesAndHideTheLiteralSide(t *testing.T) {
	ref := corev1.EnvVar{Name: "DB_PASS", ValueFrom: &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "db"},
			Key:                  "pass"}}}
	literal := corev1.EnvVar{Name: "DB_PASS", Value: "hunter2"}
	fields := envChanges("c", []corev1.EnvVar{literal},
		[]corev1.EnvVar{ref})
	assert.Equal(t, "changed", fields[0].Before)
	assert.Equal(t, "from secret db/pass", fields[0].After)
}

func argsText(before, after []string) (string, string) {
	fields := appendArgs(nil, "containers[app].args", before, after)
	return fields[0].Before, fields[0].After
}

func TestArgsChangesHideEveryValueAfterAFlagThatIsNotAllowListed(t *testing.T) {
	before, after := argsText(
		[]string{"--db-pass", "hunter2", "--verbose"},
		[]string{"--db-pass", "hunter3", "--verbose"})
	assert.Equal(t, "changed", before, "only hidden values changed")
	assert.Equal(t, "changed", after)

	before, after = argsText(
		[]string{"--url=https://user:pw@h/x", "-p", "hunter2", "-phunter2"},
		[]string{"--workers", "4"})
	for _, text := range []string{before, after} {
		assert.NotContains(t, text, "hunter")
		assert.NotContains(t, text, "pw@")
	}
	assert.Equal(t, "--url=changed -p changed -pchanged", before)
}

func TestArgsChangesShowAllowListedFlagsAndPlainWords(t *testing.T) {
	before, after := argsText(
		[]string{"/app", "serve", "--log-level", "info", "--mode=old"},
		[]string{"/app", "serve", "--log-level", "debug", "--mode=new",
			"--fast"})
	assert.Equal(t, "/app serve --log-level info --mode=old", before)
	assert.Equal(t,
		"/app serve --log-level debug --mode=new --fast", after)
}

func TestExecProbeAndQueryCarryNoSecrets(t *testing.T) {
	probe := &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
		Exec: &corev1.ExecAction{Command: []string{"check", "--token",
			"abc123"}}}}
	assert.NotContains(t, probeText(probe), "abc123")
	assert.Equal(t, "/ready?changed", safePath("/ready?key=abc"))
}

// An edit of hidden values must survive the revision diff: "env FOO
// changed" is the only thing a reader learns of it.
func TestEnvAndArgEditsOfHiddenValuesSurviveTheRevisionDiff(t *testing.T) {
	env := envChanges("containers[app]",
		[]corev1.EnvVar{{Name: "FOO", Value: "1"}},
		[]corev1.EnvVar{{Name: "FOO", Value: "2"}})
	args := appendArgs(nil, "containers[app].args",
		[]string{"--db-pass", "a"}, []string{"--db-pass", "b"})

	got := inventory.RevisionDiff([]inventory.Change{
		{Fields: append(env, args...)}})

	var paths []string
	for _, field := range got {
		paths = append(paths, field.Path)
	}
	assert.ElementsMatch(t, []string{
		"containers[app].env.FOO", "containers[app].args"}, paths)
}

// A literal that merely starts with "from " is not a reference: it is a
// value like any other and stays hidden.
func TestEnvChangesHideALiteralThatLooksLikeAReference(t *testing.T) {
	for _, value := range []string{"from secret db/pass", "from hunter2"} {
		fields := envChanges("c",
			[]corev1.EnvVar{{Name: "DB_PASS", Value: value}},
			[]corev1.EnvVar{{Name: "DB_PASS", Value: "x"}})
		assert.Equal(t, "changed", fields[0].Before, value)
	}
}

// The same words as a real reference to a Secret are a different edit.
func TestEnvChangesTellALiteralFromAReferenceWithTheSameWords(t *testing.T) {
	literal := corev1.EnvVar{Name: "FOO", Value: "from secret db/pass"}
	ref := corev1.EnvVar{Name: "FOO", ValueFrom: &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "db"},
			Key:                  "pass"}}}

	fields := envChanges("c", []corev1.EnvVar{literal}, []corev1.EnvVar{ref})

	assert.Len(t, fields, 1)
	assert.Equal(t, "changed", fields[0].Before)
	assert.Equal(t, "from secret db/pass", fields[0].After)
}
