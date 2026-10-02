package config

import (
	"strings"
	"testing"
)

func TestValidateRejectsBlankSilenceMatchers(t *testing.T) {
	cases := map[string]struct {
		rule SilenceRule
		want string
	}{
		"pod pattern": {
			SilenceRule{PodNamePatterns: []string{""}},
			"silences[0].podNamePatterns[0] must not be empty",
		},
		"container message": {
			SilenceRule{ContainerMessages: []string{"oom", "  "}},
			"silences[0].containerMessages[1] must not be empty",
		},
		"event message": {
			SilenceRule{EventMessages: []string{""}},
			"silences[0].eventMessages[0] must not be empty",
		},
		"namespace": {
			SilenceRule{Namespaces: []string{""}},
			"silences[0].namespaces[0] must not be empty",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Silences = []SilenceRule{tc.rule}

			errs := prepareConfig(cfg)

			if !containsError(errs, tc.want) {
				t.Fatalf("errors %v do not contain %q", errs, tc.want)
			}
		})
	}
}

func TestValidateRejectsBlankIgnoreEntriesByFieldName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.IgnorePodNames = []string{"api-.*", ""}

	errs := prepareConfig(cfg)

	if !containsError(errs, "ignorePodNames[1] must not be empty") {
		t.Fatalf("errors = %v", errs)
	}
	for _, err := range errs {
		if strings.HasPrefix(err.Error(), "silences[") {
			t.Fatalf("synthetic rule reported as a silence: %v", err)
		}
	}
}

func TestValidateAcceptsNonBlankMatchers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Silences = []SilenceRule{{
		PodNamePatterns: []string{"^api-"}, EventMessages: []string{"x"},
	}}
	cfg.IgnoreContainerNames = []string{"sidecar"}

	if errs := prepareConfig(cfg); len(errs) > 0 {
		t.Fatalf("errors = %v", errs)
	}
}
