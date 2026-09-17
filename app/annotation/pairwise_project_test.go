package annotation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/blueship581/pinru/internal/annotation"
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

func TestRecordPairwiseVideoStoresAbsolutePathAndMarksVideoReady(t *testing.T) {
	s, _, _ := annotationFixture(t)
	s.pairwiseVideoDir = filepath.Join(t.TempDir(), "pairwise-videos")
	c, err := s.EnablePairwise(EnablePairwiseRequest{TaskID: "题目-1"})
	if err != nil {
		t.Fatal(err)
	}
	c.Pairwise.RunA.DeliverableSHA = strings.Repeat("a", 40)
	c.Pairwise.RunA.ContainerID = "container-a"
	if _, err := s.store.SaveAnnotationCase(*c, c.Revision); err != nil {
		t.Fatal(err)
	}
	var recordedArgs []string
	var commands []string
	s.command = func(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		switch name {
		case "docker":
			return []byte("172.18.0.8\n"), nil
		case "/usr/bin/open":
			return nil, nil
		case "/usr/sbin/screencapture":
			recordedArgs = append([]string(nil), args...)
			output := args[len(args)-1]
			return nil, os.WriteFile(output, []byte("quicktime-video"), 0o600)
		default:
			t.Fatalf("command = %s", name)
			return nil, nil
		}
	}
	updated, err := s.RecordPairwiseVideo(context.Background(), PairwiseSideRequest{TaskID: c.TaskID, Side: domain.PairwiseSideA})
	if err != nil {
		t.Fatal(err)
	}
	run := updated.Pairwise.RunA
	if run.VideoStatus != domain.PairwiseVideoReady || !filepath.IsAbs(run.VideoPath) {
		t.Fatalf("video state = %#v", run)
	}
	if !strings.HasPrefix(run.VideoPath, s.pairwiseVideoDir+string(filepath.Separator)) {
		t.Fatalf("video path = %s", run.VideoPath)
	}
	joined := strings.Join(recordedArgs, " ")
	if !strings.Contains(joined, "-v -V30 -T3 -D1 -k -x") {
		t.Fatalf("screencapture args = %q", joined)
	}
	if len(commands) != 3 || commands[1] != "/usr/bin/open http://172.18.0.8:4173" || !strings.HasPrefix(commands[2], "/usr/sbin/screencapture ") {
		t.Fatalf("commands = %#v", commands)
	}
}
