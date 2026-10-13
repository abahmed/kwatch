package architecture

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// maxGoLines is the length at which a Go file must be split.
const maxGoLines = 400

// longGoFiles are files allowed to exceed maxGoLines, each capped at its
// current length. It is empty: split a file instead of adding to it.
var longGoFiles = map[string]int{}

// forbiddenName matches scratch and leftover files that must never be
// committed or left in the tree.
var forbiddenName = regexp.MustCompile(
	`(^|/)zz_[^/]*_test\.go$|_tmp_test\.go$|_debug_test\.go$|\.orig$|\.rej$`)

// forbiddenWord is built from parts so this file never contains it.
var forbiddenWord = regexp.MustCompile("(?i)" + "cla" + "ude")

// hygieneProblems inspects the given files (paths relative to root) and
// returns one line per violation.
func hygieneProblems(root string, files []string) []string {
	var problems []string
	for _, f := range files {
		if forbiddenName.MatchString(f) {
			problems = append(problems, f+": forbidden file name")
		}
		data, err := os.ReadFile(filepath.Join(root, f))
		if err != nil || bytes.IndexByte(data, 0) >= 0 {
			continue
		}
		if forbiddenWord.Match(data) {
			problems = append(problems, f+": contains the forbidden word")
		}
		if path.Ext(f) == ".go" {
			problems = append(problems, goFileProblems(f, data)...)
		}
	}
	return problems
}

func goFileProblems(f string, data []byte) []string {
	var problems []string
	if formatted, err := format.Source(data); err != nil ||
		!bytes.Equal(formatted, data) {
		problems = append(problems, f+": not gofmt-formatted")
	}
	lines := bytes.Count(data, []byte("\n"))
	allowed, listed := longGoFiles[f]
	switch {
	case listed && lines < maxGoLines:
		problems = append(problems, fmt.Sprintf(
			"%s: now %d lines; remove it from longGoFiles", f, lines))
	case listed && lines > allowed:
		problems = append(problems, fmt.Sprintf(
			"%s: grew to %d lines (was %d); split it", f, lines, allowed))
	case !listed && lines >= maxGoLines:
		problems = append(problems, fmt.Sprintf(
			"%s: %d lines (limit %d); split it", f, lines, maxGoLines-1))
	}
	return problems
}

// workflowProblems rejects a second place that pins the linter version.
func workflowProblems(root string) []string {
	files, _ := filepath.Glob(filepath.Join(root, ".github", "workflows",
		"*.yml"))
	var problems []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err == nil && bytes.Contains(data, []byte("golangci-lint-action")) {
			problems = append(problems, f+": the linter version is pinned "+
				"in the Makefile (GOLANGCI_VERSION), not in a workflow")
		}
	}
	return problems
}

// repoFiles lists tracked and untracked files that are not ignored.
func repoFiles(t *testing.T, root string) []string {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others",
		"--exclude-standard")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if os.Getenv("CI") == "true" {
			t.Fatalf("git ls-files: %v", err)
		}
		t.Skipf("not a git checkout: %v", err)
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f == "" || strings.HasPrefix(f, "vendor/") {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			files = append(files, f)
		}
	}
	return files
}

func TestRepositoryHygiene(t *testing.T) {
	root := repositoryRoot(t)
	problems := hygieneProblems(root, repoFiles(t, root))
	problems = append(problems, workflowProblems(root)...)
	for _, p := range problems {
		t.Error(p)
	}
}

func TestHygieneRejectsBadFiles(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a/zz_x_test.go", "package a\n")
	write("a/b_tmp_test.go", "package a\n")
	write("a/b_debug_test.go", "package a\n")
	write("a/x.orig", "x\n")
	write("a/x.rej", "x\n")
	write("notes.md", "written by "+"Cla"+"ude\n")
	write("ugly.go", "package a\nfunc  f( ) {}\n")
	write("big.go", "package a\n\n"+strings.Repeat("// x\n", maxGoLines))
	write("good.go", "package a\n\nfunc f() {}\n")
	files := []string{"a/zz_x_test.go", "a/b_tmp_test.go",
		"a/b_debug_test.go", "a/x.orig", "a/x.rej", "notes.md", "ugly.go",
		"big.go", "good.go"}
	got := strings.Join(hygieneProblems(root, files), "\n")
	for _, want := range []string{
		"a/zz_x_test.go: forbidden file name",
		"a/b_tmp_test.go: forbidden file name",
		"a/b_debug_test.go: forbidden file name",
		"a/x.orig: forbidden file name", "a/x.rej: forbidden file name",
		"notes.md: contains the forbidden word",
		"ugly.go: not gofmt-formatted", "big.go: ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing problem %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "good.go") {
		t.Errorf("good.go flagged:\n%s", got)
	}
}

func TestHygieneLongFileAllowlistIsEmpty(t *testing.T) {
	if len(longGoFiles) != 0 {
		t.Fatalf("longGoFiles must stay empty, has %v", longGoFiles)
	}
	root := t.TempDir()
	name := "internal/pipeline/long.go"
	full := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}
	body := "package a\n\n" + strings.Repeat("// x\n", maxGoLines)
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := hygieneProblems(root, []string{name}); len(got) != 1 ||
		!strings.Contains(got[0], "split it") {
		t.Errorf("a long file must fail: %v", got)
	}
}

func TestHygieneRejectsWorkflowLinterPin(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	body := "uses: golangci/golangci-lint-action@v9\n"
	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(body),
		0o600); err != nil {
		t.Fatal(err)
	}
	if len(workflowProblems(root)) != 1 {
		t.Error("a workflow linter pin must fail")
	}
}
