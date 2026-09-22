//go:build e2e

package harness

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var forwardedPortPattern = regexp.MustCompile(`127\.0\.0\.1:([0-9]+)`)

type PortForward struct {
	Command *exec.Cmd
	Port    int
}

func StartPortForward(
	ctx context.Context,
	kubeconfig, kubeContext, namespace, pod string,
) (*PortForward, error) {
	return StartPortForwardTarget(
		ctx, kubeconfig, kubeContext, namespace, "pod/"+pod, 8060,
	)
}

func StartPortForwardTarget(
	ctx context.Context,
	kubeconfig, kubeContext, namespace, target string,
	remotePort int,
) (*PortForward, error) {
	args := []string{
		"-n", namespace, "port-forward", target,
		fmt.Sprintf("0:%d", remotePort),
	}
	if kubeconfig != "" {
		args = append([]string{"--kubeconfig", kubeconfig}, args...)
	}
	if kubeContext != "" {
		args = append([]string{"--context", kubeContext}, args...)
	}
	command := exec.CommandContext(ctx, "kubectl", args...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("port-forward stdout: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("port-forward stderr: %w", err)
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start port-forward: %w", err)
	}
	port, err := readForwardedPort(stdout, stderr)
	if err != nil {
		_ = command.Process.Kill()
		return nil, err
	}
	return &PortForward{Command: command, Port: port}, nil
}

func (p *PortForward) Close() error {
	if p == nil || p.Command == nil || p.Command.Process == nil {
		return nil
	}
	if err := p.Command.Process.Kill(); err != nil {
		return err
	}
	return p.Command.Wait()
}

func readForwardedPort(stdout, stderr io.Reader) (int, error) {
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	lines := make(chan string, 2)
	read := func(reader io.Reader) {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}
	go read(stdout)
	go read(stderr)
	for {
		select {
		case line := <-lines:
			match := forwardedPortPattern.FindStringSubmatch(line)
			if len(match) == 2 {
				port, err := strconv.Atoi(match[1])
				if err == nil {
					return port, nil
				}
			}
		case <-deadline.C:
			return 0, fmt.Errorf("port-forward did not report a local port")
		}
	}
}

func (p *PortForward) URL(path string) string {
	path = strings.TrimPrefix(path, "/")
	return fmt.Sprintf("http://127.0.0.1:%d/%s", p.Port, path)
}
