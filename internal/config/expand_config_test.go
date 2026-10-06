package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestExpandConfigDocumentEnvVar(t *testing.T) {
	t.Setenv("TEST_VAR", "expanded-value")

	raw := "app:\n  clusterName: ${TEST_VAR}"
	doc, err := expandConfigDocument(raw)

	assert.Nil(t, err)
	assert.NotNil(t, doc)

	var result Config
	err = doc.Decode(&result)
	assert.Nil(t, err)
	assert.Equal(t, "expanded-value", result.App.ClusterName)
}

func TestExpandConfigDocumentEnvVarWithSpecialChars(t *testing.T) {
	t.Setenv("SPECIAL_VAR", `value with "quote" : colon`+"\nnewline")

	raw := `app:
  clusterName: ${SPECIAL_VAR}`
	doc, err := expandConfigDocument(raw)

	assert.Nil(t, err)
	assert.NotNil(t, doc)

	var result Config
	err = doc.Decode(&result)
	assert.Nil(t, err)
	assert.Equal(t,
		`value with "quote" : colon`+"\nnewline",
		result.App.ClusterName)
}

func TestExpandConfigDocumentIntField(t *testing.T) {
	t.Setenv("PORT_VAR", "8080")

	raw := `healthCheck:
  port: ${PORT_VAR}`
	doc, err := expandConfigDocument(raw)

	assert.Nil(t, err)
	assert.NotNil(t, doc)

	var result Config
	err = doc.Decode(&result)
	assert.Nil(t, err)
	assert.Equal(t, 8080, result.HealthCheck.Port)
}

func TestExpandConfigDocumentUnsetVarError(t *testing.T) {
	os.Unsetenv("NONEXISTENT_VAR")

	raw := `app:
  clusterName: ${NONEXISTENT_VAR}`
	_, err := expandConfigDocument(raw)

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "NONEXISTENT_VAR")
}

func TestExpandConfigDocumentIgnoresVarInComment(t *testing.T) {
	raw := `app:
  clusterName: value
  # comment with ${UNDEFINED_VAR}`
	doc, err := expandConfigDocument(raw)

	assert.Nil(t, err)
	assert.NotNil(t, doc)
}

func TestExpandConfigDocumentFileRef(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config-*")
	assert.Nil(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("file-content")
	assert.Nil(t, err)
	tmpFile.Close()

	raw := "app:\n  clusterName: ${file:" + tmpFile.Name() + "}"
	doc, err := expandConfigDocument(raw)

	assert.Nil(t, err)
	assert.NotNil(t, doc)

	var result Config
	err = doc.Decode(&result)
	assert.Nil(t, err)
	assert.Equal(t, "file-content", result.App.ClusterName)
}

func TestExpandConfigDocumentQuotedStringStaysString(t *testing.T) {
	t.Setenv("NUM", "42")

	raw := `app:
  clusterName: "${NUM}"`
	doc, err := expandConfigDocument(raw)

	assert.Nil(t, err)
	assert.NotNil(t, doc)

	var result Config
	err = doc.Decode(&result)
	assert.Nil(t, err)
	assert.Equal(t, "42", result.App.ClusterName)
}

func TestExpandConfigDocumentEmptyDocument(t *testing.T) {
	doc, err := expandConfigDocument("")
	assert.Nil(t, err)
	assert.Nil(t, doc)
}

func TestExpandConfigDocumentMultipleVars(t *testing.T) {
	t.Setenv("VAR1", "value1")
	t.Setenv("VAR2", "value2")

	raw := `app:
  clusterName: ${VAR1}-${VAR2}`
	doc, err := expandConfigDocument(raw)

	assert.Nil(t, err)
	assert.NotNil(t, doc)

	var result Config
	err = doc.Decode(&result)
	assert.Nil(t, err)
	assert.Equal(t, "value1-value2", result.App.ClusterName)
}

func TestExpandConfigDocumentPreservesValuesWithSpecialChars(t *testing.T) {
	raw := `app:
  clusterName: "key=value"
healthCheck:
  port: 8080`
	doc, err := expandConfigDocument(raw)

	assert.Nil(t, err)
	assert.NotNil(t, doc)

	var result Config
	err = doc.Decode(&result)
	assert.Nil(t, err)
	assert.Equal(t, "key=value", result.App.ClusterName)
	assert.Equal(t, 8080, result.HealthCheck.Port)
}

func TestYamlNodeUnmarshalAndExpand(t *testing.T) {
	t.Setenv("CLUSTER", "my-cluster")

	raw := `app:
  clusterName: ${CLUSTER}`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(raw), &node)
	assert.Nil(t, err)

	unset := map[string]bool{}
	err = expandNode(&node, unset)
	assert.Nil(t, err)
	assert.Empty(t, unset)
}

func TestExpandConfigDocumentDoubleDollarKeepsLiteral(t *testing.T) {
	os.Unsetenv("NOT_SET_ANYWHERE")

	raw := "app:\n  clusterName: 'a-$${NOT_SET_ANYWHERE}-b'"
	doc, err := expandConfigDocument(raw)

	assert.NoError(t, err)
	var result Config
	assert.NoError(t, doc.Decode(&result))
	assert.Equal(t, "a-${NOT_SET_ANYWHERE}-b", result.App.ClusterName)
}
