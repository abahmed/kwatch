//go:build e2e

package harness

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var panicMarkers = []string{
	"panic:", "fatal error", "runtime error", "invalid memory address",
	"nil pointer",
}

func (e *Environment) AssertNoRuntimePanic(ctx context.Context) error {
	pods, err := e.Client.CoreV1().Pods(e.Config.KwatchNamespace).List(
		ctx, metav1.ListOptions{LabelSelector: "app=kwatch"},
	)
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		for _, previous := range []bool{false, true} {
			body, logErr := e.podLogs(ctx, pod.Name, previous)
			if logErr != nil {
				continue
			}
			lower := strings.ToLower(string(body))
			for _, marker := range panicMarkers {
				if strings.Contains(lower, marker) {
					return fmt.Errorf(
						"Kwatch Pod %s contains runtime failure marker %q",
						pod.Name, marker,
					)
				}
			}
		}
	}
	return nil
}
