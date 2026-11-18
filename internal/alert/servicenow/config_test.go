package servicenow

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

func baseConfig(extra map[string]interface{}) map[string]interface{} {
	config := map[string]interface{}{
		"instanceURL": "https://acme.service-now.com/",
		"user":        "kwatch", "password": testPassword,
	}
	for k, v := range extra {
		config[k] = v
	}
	return config
}

func TestServiceNowSettingsDefaults(t *testing.T) {
	s, err := parseSettings(baseConfig(nil))
	if err != nil {
		t.Fatal(err)
	}
	if s.base != "https://acme.service-now.com" || s.table != "incident" ||
		s.noteField != "work_notes" || s.resolveState != "6" ||
		s.reopenState != "2" || s.closeCode != "Solution provided" {
		t.Fatalf("defaults = %+v", s)
	}
	if s.urgency["page"] != "1" || s.impact["low"] != "3" {
		t.Fatalf("levels = %v %v", s.urgency, s.impact)
	}
}

func TestServiceNowSettingsRejected(t *testing.T) {
	cases := map[string]struct {
		extra map[string]interface{}
		want  string
	}{
		"http": {map[string]interface{}{
			"instanceURL": "http://acme.service-now.com"}, "https"},
		"path": {map[string]interface{}{
			"instanceURL": "https://acme.service-now.com/x"}, "path"},
		"missing url": {map[string]interface{}{"instanceURL": ""},
			"required"},
		"both auth": {map[string]interface{}{
			"clientID": "c", "clientSecret": "s"}, "not both"},
		"half basic": {map[string]interface{}{"password": ""},
			"both user and password"},
		"bad table": {map[string]interface{}{"table": "inc;drop"},
			"table"},
		"bad note field": {map[string]interface{}{"noteField": "Work Notes"},
			"noteField"},
		"bad state": {map[string]interface{}{"resolveState": "6^x"},
			"resolveState"},
		"bad tier": {map[string]interface{}{
			"urgency": map[string]interface{}{"urgent": "1"}}, "not a tier"},
		"bad level": {map[string]interface{}{
			"impact": map[string]interface{}{"page": "5"}}, "1, 2 or 3"},
		"bad map": {map[string]interface{}{"urgency": "1"}, "mapping"},
		"bad field": {map[string]interface{}{
			"fields": map[string]interface{}{"U Field": "x"}},
			"column name"},
	}
	for name, tc := range cases {
		_, err := parseSettings(baseConfig(tc.extra))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
		if err != nil && strings.Contains(err.Error(), testPassword) {
			t.Errorf("%s: error quotes the password", name)
		}
	}
}

func TestNewServiceNowRefusesBadSettings(t *testing.T) {
	f := newFakeInstance(t)
	if p := NewServiceNow(map[string]interface{}{
		"instanceURL": f.server.URL}, "prod", f.dependencies()); p != nil {
		t.Fatal("a provider without credentials was built")
	}
}

func TestServiceNowLongTitleIsOneShortLine(t *testing.T) {
	f := newFakeInstance(t)
	p := f.provider(nil)
	msg := announceWithShort("🔴 line one\nline two " +
		strings.Repeat("x", 300))
	send(t, p, msg)
	title := f.row("sys0001")["short_description"]
	if len(title) > titleLimit || strings.Contains(title, "\n") {
		t.Fatalf("title = %q (%d bytes)", title, len(title))
	}
}

func announceWithShort(short string) notification.Message {
	m := providertest.Announce()
	m.Short = short
	return m
}
