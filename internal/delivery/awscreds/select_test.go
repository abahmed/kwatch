package awscreds

import (
	"strings"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func selectWith(
	t *testing.T, settings map[string]interface{},
	values map[string]string,
) (Source, error) {
	t.Helper()
	return Select(settings, Options{Provider: "SNS",
		Region: "cn-north-1", Now: newFakeClock().Now,
		Getenv: env(values)})
}

var bothEnvs = map[string]string{
	EnvRoleARN: "arn:aws:iam::1:role/kwatch", EnvTokenFile: "/t",
	EnvAgentURL: "http://169.254.170.23/v1/credentials", EnvAgentToken: "/a",
}

func TestSelectOrder(t *testing.T) {
	keys := map[string]interface{}{"accessKeyId": "AKIA",
		"secretAccessKey": "s"}
	src, err := selectWith(t, keys, bothEnvs)
	if _, ok := src.(Static); !ok || err != nil {
		t.Fatalf("config keys: %T %v", src, err)
	}
	src, err = selectWith(t, nil, bothEnvs)
	web, ok := src.(*cached)
	if !ok || err != nil || web.name != "web identity" {
		t.Fatalf("web identity: %T %v", src, err)
	}
	pod := map[string]string{EnvAgentURL: bothEnvs[EnvAgentURL],
		EnvAgentToken: "/a"}
	src, err = selectWith(t, nil, pod)
	if c, ok := src.(*cached); !ok || err != nil || c.name != "pod identity" {
		t.Fatalf("pod identity: %T %v", src, err)
	}
	src, err = selectWith(t, nil, nil)
	if src != nil || err != nil {
		t.Fatalf("nothing configured: %T %v", src, err)
	}
}

func TestSelectRefusesHalfAKeyPair(t *testing.T) {
	for _, settings := range []map[string]interface{}{
		{"accessKeyId": "AKIA"}, {"secretAccessKey": testSecretKey},
	} {
		if _, err := selectWith(t, settings, bothEnvs); err == nil {
			t.Errorf("%v accepted", settings)
		}
	}
}

func TestSelectRefusesAnUnlistedAgentHost(t *testing.T) {
	_, err := selectWith(t, nil, map[string]string{
		EnvAgentURL: "http://169.254.169.254/", EnvAgentToken: "/a"})
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("err = %v", err)
	}
}

func TestSelectWarnsAboutAStaticSessionToken(t *testing.T) {
	logs := captureKlog(t)
	_, err := selectWith(t, map[string]interface{}{"accessKeyId": "AKIA",
		"secretAccessKey": testSecretKey, "sessionToken": testSession},
		nil)
	klogFlush()
	if err != nil || !strings.Contains(logs.String(),
		"kwatch cannot refresh it") {
		t.Fatalf("no warning: %v %s", err, logs.String())
	}
	assertNoSecrets(t, "log", logs.String())
}

func TestSelectUsesThePartitionsSTSEndpoint(t *testing.T) {
	logs := captureKlog(t)
	if _, err := selectWith(t, nil, bothEnvs); err != nil {
		t.Fatal(err)
	}
	klogFlush()
	if !strings.Contains(logs.String(),
		"https://sts.cn-north-1.amazonaws.com.cn") {
		t.Fatalf("sts endpoint not derived: %s", logs.String())
	}
	_, err := selectWith(t, nil, map[string]string{
		EnvRoleARN: "arn", EnvTokenFile: "/t",
		EnvSTSEndpoint: "http://sts.local/"})
	if err == nil {
		t.Fatal("a plain http STS override was accepted")
	}
}

func TestEndpoints(t *testing.T) {
	for region, want := range map[string]string{
		"cn-north-1":    "https://sns.cn-north-1.amazonaws.com.cn/",
		"us-gov-west-1": "https://sns.us-gov-west-1.amazonaws.com/",
		"eu-west-1":     "https://sns.eu-west-1.amazonaws.com/",
	} {
		if got, _ := Endpoint("", "sns", region); got != want {
			t.Errorf("%s: %s, want %s", region, got, want)
		}
	}
	got, err := Endpoint(" https://vpce.example.com/ ", "sns", "x")
	if err != nil || got != "https://vpce.example.com/" {
		t.Fatalf("override = %q, %v", got, err)
	}
	if _, err := Endpoint("vpce.example.com", "sns", "x"); err == nil {
		t.Fatal("an endpoint without a scheme was accepted")
	}
}
