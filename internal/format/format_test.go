package format

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShortImage(t *testing.T) {
	cases := map[string]string{
		"":                                    "",
		"nginx":                               "nginx",
		"nginx:1.25":                          "nginx:1.25",
		"library/nginx:1.25":                  "nginx:1.25",
		"registry.example.com/team/api:1.2.0": "api:1.2.0",
		"localhost:5000/api:dev":              "api:dev",
		"ghcr.io/org/tool@sha256:0123456789abcdef0123": "tool@0123456789ab",
		"ghcr.io/org/tool@sha256:abc":                  "tool@abc",
	}
	for in, want := range cases {
		assert.Equal(t, want, ShortImage(in), in)
	}
}

func TestShortNode(t *testing.T) {
	assert.Equal(t, "ip-10-0-81-7", ShortNode("ip-10-0-81-7.us-east-1.compute.internal"))
	assert.Equal(t, "worker-1", ShortNode("worker-1"))
	assert.Equal(t, "", ShortNode("  "))
}

func TestDuration(t *testing.T) {
	assert.Equal(t, "5s", Duration(5e9))
	assert.Equal(t, "2m30s", Duration(150e9))
	assert.Equal(t, "2h15m", Duration(8100e9))
}
