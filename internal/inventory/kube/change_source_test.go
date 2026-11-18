package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const testSHA = "1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b"

func sourceDeployment(own, template map[string]string) *appsv1.Deployment {
	d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "shop", Annotations: own}}
	d.Spec.Template.Annotations = template
	return d
}

func TestChangeSourceReadsObjectThenTemplate(t *testing.T) {
	d := sourceDeployment(
		map[string]string{sourceRepoAnnotation: "https://github.com/a/b"},
		map[string]string{sourceRevisionAnnotation: testSHA,
			sourceRepoAnnotation: "https://github.com/other/c"})
	who := Attribute(d, false)
	assert.Equal(t, "https://github.com/a/b", who.Repo)
	assert.Equal(t, testSHA, who.Commit)
}

func TestChangeSourceAbsentAnnotations(t *testing.T) {
	d := sourceDeployment(map[string]string{
		deploymentRevision:          "4",
		"app.kubernetes.io/version": "1.2.0",
	}, nil)
	who := Attribute(d, false)
	assert.Empty(t, who.Repo)
	assert.Empty(t, who.Commit)
}

func TestChangeSourceDropsInvalidValues(t *testing.T) {
	d := sourceDeployment(nil, map[string]string{
		sourceRepoAnnotation:     "https://user:token@github.com/a/b",
		sourceRevisionAnnotation: "not a revision",
	})
	repo, commit := changeSource(d)
	assert.Empty(t, repo)
	assert.Empty(t, commit)
}

func TestAttributedChangeCarriesSource(t *testing.T) {
	d := sourceDeployment(map[string]string{
		sourceRepoAnnotation:     "https://gitlab.com/team/api.git",
		sourceRevisionAnnotation: "v1.4.0",
	}, nil)
	change := attributedChange(d, false, attributionTime, nil)
	assert.Equal(t, "https://gitlab.com/team/api", change.Repo)
	assert.Equal(t, "v1.4.0", change.Commit)
}
