//go:build e2e

package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ArtifactWriter struct {
	Root string
}

func NewArtifactWriter(root string) (*ArtifactWriter, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact directory: %w", err)
	}
	return &ArtifactWriter{Root: root}, nil
}

func (a *ArtifactWriter) WriteJSON(name string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return a.write(name, append(payload, '\n'))
}

func (a *ArtifactWriter) Write(name string, payload []byte) error {
	return a.write(name, payload)
}

func (a *ArtifactWriter) write(name string, payload []byte) error {
	path := filepath.Join(a.Root, filepath.Clean(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o600)
}

func (e *Environment) CaptureDiagnostics(ctx context.Context) error {
	commands := []struct {
		name string
		args []string
	}{
		{"kubernetes/pods.yaml", []string{"get", "pods", "-A", "-o", "yaml"}},
		{
			"kubernetes/events.yaml",
			[]string{"get", "events", "-A", "--sort-by=.lastTimestamp", "-o", "yaml"},
		},
		{"kubernetes/nodes.yaml", []string{"get", "nodes", "-o", "yaml"}},
		{"kubernetes/leases.yaml", []string{"get", "leases", "-A", "-o", "yaml"}},
		{
			"kubernetes/deployments.yaml",
			[]string{"get", "deployments", "-A", "-o", "yaml"},
		},
		{"kubernetes/services.yaml", []string{"get", "services", "-A", "-o", "yaml"}},
		{
			"kubernetes/endpointslices.yaml",
			[]string{"get", "endpointslices", "-A", "-o", "yaml"},
		},
		{"kubernetes/namespaces.yaml", []string{"get", "namespaces", "-o", "yaml"}},
	}
	for _, command := range commands {
		output, runErr := kubectl(ctx, e.Config, command.args...)
		if runErr != nil {
			output = append(output, []byte("\ncommand error: "+runErr.Error()+"\n")...)
		}
		if err := e.Artifacts.Write(command.name, output); err != nil {
			return err
		}
	}
	pods, err := e.Client.CoreV1().Pods(e.Config.KwatchNamespace).List(
		ctx, metav1.ListOptions{LabelSelector: "app=kwatch"},
	)
	if err == nil {
		for _, pod := range pods.Items {
			for _, previous := range []bool{false, true} {
				body, logErr := e.podLogs(ctx, pod.Name, previous)
				if logErr != nil {
					body = []byte(logErr.Error())
				}
				suffix := ".log"
				if previous {
					suffix = ".previous.log"
				}
				if writeErr := e.Artifacts.Write(
					"kwatch/logs-"+pod.Name+suffix, body,
				); writeErr != nil {
					return writeErr
				}
			}
		}
	}
	for _, endpoint := range []string{
		"health", "readyz", "availabilityz", "incidents", "deadletters",
		"informer", "persistence", "metrics", "kubelet", "controlplane",
	} {
		body, status, getErr := e.Health.Get(ctx, "/"+endpoint,
			endpoint != "health" && endpoint != "readyz" && endpoint != "availabilityz")
		if getErr != nil {
			body = []byte(getErr.Error())
		}
		if err := e.Artifacts.Write(
			"kwatch/"+endpoint+".response", append(body, '\n'),
		); err != nil {
			return err
		}
		if err := e.Artifacts.WriteJSON(
			"kwatch/"+endpoint+".status.json", map[string]any{
				"status": status,
				"error":  getErrString(getErr),
			},
		); err != nil {
			return err
		}
	}
	if requests, requestErr := e.Receiver.Requests(ctx); requestErr == nil {
		if err := e.Artifacts.WriteJSON(
			"receiver/requests.json", requests,
		); err != nil {
			return err
		}
	}
	if policy, policyErr := e.Receiver.Policy(ctx); policyErr == nil {
		if err := e.Artifacts.WriteJSON("receiver/control.json", policy); err != nil {
			return err
		}
	}
	return nil
}

func (e *Environment) podLogs(
	ctx context.Context,
	name string,
	previous bool,
) ([]byte, error) {
	request := e.Client.CoreV1().Pods(e.Config.KwatchNamespace).
		GetLogs(name, &corev1.PodLogOptions{Previous: previous})
	stream, err := request.Stream(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	return io.ReadAll(io.LimitReader(stream, 8<<20))
}

func getErrString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func kubectl(
	ctx context.Context,
	config Config,
	args ...string,
) ([]byte, error) {
	commandArgs := make([]string, 0, len(args)+4)
	if config.Kubeconfig != "" {
		commandArgs = append(commandArgs, "--kubeconfig", config.Kubeconfig)
	}
	if config.Context != "" {
		commandArgs = append(commandArgs, "--context", config.Context)
	}
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(ctx, "kubectl", commandArgs...)
	output, err := command.CombinedOutput()
	return output, err
}
