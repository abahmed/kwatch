package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIncidentPersistedRoundTripPreservesNewFields(t *testing.T) {
	assert := assert.New(t)

	inc := &Incident{
		Subject: Subject{
			ContainerName: "app",
			NodeName:      "node-1",
			Image:         "repo/app:v1",
		},
		Status: Status{
			Containers: map[string]bool{"app": true},
			Resources:  map[string]bool{},
		},
	}

	persisted := inc.ToPersisted()
	assert.Equal("app", persisted.ContainerName)
	assert.Equal(map[string]bool{"app": true}, persisted.Containers)
	assert.Equal("node-1", persisted.NodeName)
	assert.Equal("repo/app:v1", persisted.Image)

	restored := persisted.ToIncident()
	assert.Equal("app", restored.ContainerName)
	assert.Equal(map[string]bool{"app": true}, restored.Containers)
	assert.Equal("node-1", restored.NodeName)
	assert.Equal("repo/app:v1", restored.Image)
}

func TestPersistedIncidentOldFormatUnmarshalsCleanly(t *testing.T) {
	assert := assert.New(t)

	raw := `{"key":"ns:o:r:","reason":"r"}`

	var persisted PersistedIncident
	err := json.Unmarshal([]byte(raw), &persisted)
	assert.NoError(err)
	assert.Equal("", persisted.ContainerName)
	assert.Nil(persisted.Containers)

	inc := persisted.ToIncident()
	assert.NotNil(inc)
	assert.Equal("", inc.ContainerName)
	assert.NotNil(inc.Containers)
	assert.Empty(inc.Containers)
}
