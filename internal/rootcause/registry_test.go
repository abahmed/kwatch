package rootcause

import "testing"

func TestRegistryHostDefaultsToDockerHub(t *testing.T) {
	cases := map[string]string{
		"nginx":             "docker.io",
		"library/nginx":     "docker.io",
		"ghcr.io/org/app:1": "ghcr.io",
		"localhost/app":     "localhost",
		"registry:5000/app": "registry:5000",
		"myorg/app":         "docker.io",
	}
	for image, want := range cases {
		if got := RegistryHost(image); got != want {
			t.Errorf("RegistryHost(%q) = %q want %q", image, got, want)
		}
	}
}
