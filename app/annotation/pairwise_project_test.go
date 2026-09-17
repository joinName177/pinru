package annotation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectPairwiseProjectLaunchUsesDeclaredPNPMDevScript(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"scripts":{"dev":"vite"},"devDependencies":{"vite":"1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "pnpm-lock.yaml"), []byte("lockfileVersion: '9.0'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	launch, err := detectPairwiseProjectLaunch(repo)
	if err != nil {
		t.Fatal(err)
	}
	if launch.Command != "corepack pnpm run dev" || launch.Port != 5173 || launch.Host != "::1" {
		t.Fatalf("launch = %#v", launch)
	}
}

func TestPairwiseProjectContainerCommandMakesPNPMAvailableToNestedScripts(t *testing.T) {
	command := pairwiseProjectContainerCommand("corepack pnpm run dev")
	if !strings.Contains(command, "/tmp/pinru-project-bin/pnpm") || !strings.Contains(command, "export PATH=/tmp/pinru-project-bin:$PATH") {
		t.Fatalf("command = %s", command)
	}
}

func TestValidatePairwiseProjectRevisionRequiresExpectedBranchAndCommit(t *testing.T) {
	repo := t.TempDir()
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=PINRU", "GIT_AUTHOR_EMAIL=pinru@test", "GIT_COMMITTER_NAME=PINRU", "GIT_COMMITTER_EMAIL=pinru@test")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	runGit("init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "result.txt"), []byte("A result\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "result.txt")
	runGit("commit", "-qm", "result")
	runGit("branch", "-M", "A")
	sha := runGit("rev-parse", "HEAD")
	if err := validatePairwiseProjectRevision(context.Background(), repo, "A", sha); err != nil {
		t.Fatal(err)
	}
	if err := validatePairwiseProjectRevision(context.Background(), repo, "B", sha); err == nil || !strings.Contains(err.Error(), "B") {
		t.Fatalf("wrong branch error = %v", err)
	}
}
