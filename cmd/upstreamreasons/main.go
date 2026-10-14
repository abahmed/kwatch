package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "upstreamreasons:", err)
		os.Exit(exitCode(err))
	}
}

// cloneTimeout bounds the git clone, so a stuck network cannot hang CI.
const cloneTimeout = 5 * time.Minute

// driftError is -check finding the generated files out of date.
type driftError struct{ names []string }

func (e *driftError) Error() string {
	return "generated files differ from the tag: " +
		strings.Join(e.names, ", ") + " (run make upstream-reasons)"
}

// exitCode is 1 for drift and 2 for any other failure (setup, network),
// so CI can tell an out-of-date corpus from a broken run.
func exitCode(err error) int {
	var drift *driftError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &drift):
		return 1
	default:
		return 2
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("upstreamreasons", flag.ContinueOnError)
	gomod := fs.String("gomod", "go.mod", "go.mod that pins k8s.io/api")
	tag := fs.String("tag", "", "Kubernetes tag (default: from go.mod)")
	src := fs.String("src", "", "existing checkout (default: clone)")
	out := fs.String("out", "internal/detection/upstream", "output dir")
	check := fs.Bool("check", false, "compare instead of write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tag == "" {
		t, err := tagFromGoMod(*gomod)
		if err != nil {
			return err
		}
		*tag = t
	}
	root := *src
	if root == "" {
		dir, err := os.MkdirTemp("", "upstreamreasons-")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(dir) }()
		root = dir
		ctx, cancel := context.WithTimeout(context.Background(),
			cloneTimeout)
		defer cancel()
		if err := clone(ctx, *tag, root); err != nil {
			return err
		}
	}
	files, err := generate(root, *tag)
	if err != nil {
		return err
	}
	return write(*out, files, *check)
}

var apiVersion = regexp.MustCompile(
	`(?m)^\s*k8s\.io/api v0\.(\d+\.\d+)(\S*)`)

// tagFromGoMod maps k8s.io/api v0.N.P to the Kubernetes tag v1.N.P.
func tagFromGoMod(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	m := apiVersion.FindSubmatch(data)
	if m == nil {
		return "", fmt.Errorf("%s: no k8s.io/api requirement", file)
	}
	if len(m[2]) > 0 {
		return "", fmt.Errorf("%s: k8s.io/api is not a release "+
			"version (v0.%s%s): pass -tag", file, m[1], m[2])
	}
	return "v1." + string(m[1]), nil
}

func clone(ctx context.Context, tag, dir string) error {
	steps := [][]string{
		{"-c", "advice.detachedHead=false", "clone", "--quiet",
			"--depth", "1", "--filter=blob:none", "--sparse",
			"--branch", tag,
			"https://github.com/kubernetes/kubernetes", dir},
		append([]string{"-C", dir, "sparse-checkout", "set"},
			sparsePaths...),
	}
	for _, s := range steps {
		cmd := exec.CommandContext(ctx, "git", s...)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git %s: %w", s[len(s)-1], err)
		}
	}
	return nil
}

// generate returns the generated files by name.
func generate(root, tag string) (map[string][]byte, error) {
	c, err := load(root, append(append([]string{}, scanDirs...),
		constDirs...))
	if err != nil {
		return nil, err
	}
	c.collectCalls(func(file string) bool {
		return isGenerated(file) || !underAny(file, scanDirs)
	})
	events := eventReasons(c.calls)
	if err := c.addDynamic(events); err != nil {
		return nil, err
	}
	groups := map[string][]string{}
	for name, g := range map[string]group{
		"waiting": waiting, "terminated": terminated, "pod": pod,
		"podCondition": podCondition,
	} {
		v, err := c.values(g)
		if err != nil {
			return nil, err
		}
		groups[name] = v
	}
	hpa, err := c.conditionReasons(hpaConditionCalls)
	if err != nil {
		return nil, err
	}
	groups["hpaCondition"] = hpa
	return map[string][]byte{
		"states_generated.go": renderStates(tag, groups),
		"events_generated.go": renderEvents(tag, events),
	}, nil
}

func underAny(file string, dirs []string) bool {
	for _, d := range dirs {
		if strings.HasPrefix(file, d+"/") {
			return true
		}
	}
	return false
}

func write(dir string, files map[string][]byte, check bool) error {
	var drift []string
	for name, data := range files {
		p := filepath.Join(dir, name)
		if check {
			have, _ := os.ReadFile(p)
			if !bytes.Equal(have, data) {
				drift = append(drift, name)
			}
			continue
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return err
		}
	}
	if len(drift) > 0 {
		sort.Strings(drift)
		return &driftError{names: drift}
	}
	return nil
}
