package feature

import (
	"reflect"
	"testing"
)

func TestEntitlements(t *testing.T) {
	tests := []struct {
		name    string
		e       Entitlements
		edition Edition
		limits  Limits
		hasPro  bool
	}{
		{"community", CommunityEntitlements(), CommunityEdition,
			Limits{MaxProviders: 1, MaxReplicas: 1, ProHints: true}, false},
		{"pro", ProEntitlements(), ProEdition,
			Limits{RootCauseDetail: true, RetentionDays: 90, StatusJSON: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.e.Edition() != tt.edition {
				t.Errorf("edition = %q", tt.e.Edition())
			}
			if tt.e.Limits() != tt.limits {
				t.Errorf("limits = %+v, want %+v", tt.e.Limits(), tt.limits)
			}
			for _, definition := range Catalog() {
				want := tt.hasPro || TierOf(definition.ID) == Community
				if got := tt.e.Has(definition.ID); got != want {
					t.Errorf("Has(%q) = %v, want %v", definition.ID, got, want)
				}
			}
		})
	}
}

func TestInactive(t *testing.T) {
	tests := []struct {
		name       string
		e          Entitlements
		configured []ID
		want       []ID
	}{
		{"community sorted", CommunityEntitlements(),
			[]ID{Runbooks, PodOOM, AuditLog, Impact}, []ID{Impact, AuditLog, Runbooks}},
		{"community all entitled", CommunityEntitlements(), []ID{PodOOM, Scope}, nil},
		{"pro none inactive", ProEntitlements(), []ID{Runbooks, AuditLog}, nil},
		{"empty", CommunityEntitlements(), nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := append([]ID(nil), tt.configured...)
			got := Inactive(tt.e, tt.configured)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Inactive() = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(input, append([]ID(nil), tt.configured...)) {
				t.Error("Inactive mutated its input")
			}
		})
	}
}
