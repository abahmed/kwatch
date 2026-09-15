package incident

import "testing"

func TestGroupKeyCodecPreservesExistingShapes(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want ParsedGroupKey
	}{
		{
			name: "owner",
			key:  "CrashLoopBackOff|default|api",
			want: ParsedGroupKey{
				Reason: "CrashLoopBackOff", Namespace: "default",
				Owner: "api", OwnerForm: true,
			},
		},
		{
			name: "node",
			key:  "NodeNotReady|node|worker-1",
			want: ParsedGroupKey{
				Reason: "NodeNotReady", Kind: "node", Value: "worker-1",
			},
		},
		{
			name: "image",
			key:  "ErrImagePull|img|repo/app:v1|ns|default",
			want: ParsedGroupKey{
				Reason: "ErrImagePull", Kind: "img",
				Value: "repo/app:v1", Namespace: "default",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := groupKeyCodec.Parse(tc.key)
			if !ok || got != tc.want {
				t.Fatalf("parseGroupKey(%q) = %#v, want %#v",
					tc.key, got, tc.want)
			}
		})
	}
}

func TestGroupKeyEncodersPreserveExistingStrings(t *testing.T) {
	if got := ownerGroupKey("r", "ns", "owner"); got != "r|ns|owner" {
		t.Fatalf("unexpected owner key %q", got)
	}
	if got := encodeScopedGroupKey(
		"r", "global", "timeout",
	); got != "r|global|timeout" {
		t.Fatalf("unexpected scoped key %q", got)
	}
	if got := encodeImageGroupKey("r", "image", "ns"); got != "r|img|image|ns|ns" {
		t.Fatalf("unexpected image key %q", got)
	}
}

func TestGroupKeyCodecIdentifiesAndValidatesKinds(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want GroupKeyKind
	}{
		{name: "owner", key: "r|ns|owner", want: GroupKeyOwner},
		{name: "signature", key: "r|sig|failure", want: GroupKeySignature},
		{name: "global", key: "r|global|scope", want: GroupKeyGlobal},
		{name: "node", key: "r|node|worker", want: GroupKeyNode},
		{name: "service", key: "r|svc|ns/service", want: GroupKeyService},
		{name: "control plane", key: "r|cp|ns", want: GroupKeyControl},
		{name: "image", key: "r|img|repo:v1|ns|default", want: GroupKeyImage},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind, ok := groupKeyCodec.Kind(test.key)
			if !ok || kind != test.want {
				t.Fatalf("Kind(%q) = %q, %v; want %q, true",
					test.key, kind, ok, test.want)
			}
			if err := groupKeyCodec.Validate(test.key); err != nil {
				t.Fatalf("Validate(%q) returned error: %v", test.key, err)
			}
		})
	}
}

func TestGroupKeyCodecRejectsUnsupportedShape(t *testing.T) {
	if _, ok := groupKeyCodec.Kind("not-a-group-key"); ok {
		t.Fatal("Kind accepted an unsupported key")
	}
	if err := groupKeyCodec.Validate("not-a-group-key"); err == nil {
		t.Fatal("Validate accepted an unsupported key")
	}
}
