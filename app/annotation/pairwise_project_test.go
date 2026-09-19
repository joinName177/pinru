package annotation

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/blueship581/pinru/internal/annotation"
)

func TestRandomPairwiseProjectProxyPortStaysFourDigits(t *testing.T) {
	low, err := randomPairwiseProjectProxyPort(bytes.NewReader([]byte{0, 0}))
	if err != nil {
		t.Fatal(err)
	}
	high, err := randomPairwiseProjectProxyPort(bytes.NewReader([]byte{255, 255}))
	if err != nil {
		t.Fatal(err)
	}
	for _, port := range []int{low, high} {
		if port < 1024 || port > 9999 {
			t.Fatalf("port = %d", port)
		}
	}
	if low == high {
		t.Fatalf("ports did not vary: %d", low)
	}
}

func TestParsePairwiseProjectProxyPortRejectsValuesOutsideFourDigitRange(t *testing.T) {
	if port, err := parsePairwiseProjectProxyPort("4821\n"); err != nil || port != 4821 {
		t.Fatalf("valid port = %d, %v", port, err)
	}
	for _, value := range []string{"", "999", "10000", "not-a-port"} {
		if _, err := parsePairwiseProjectProxyPort(value); err == nil {
			t.Fatalf("accepted proxy port %q", value)
		}
	}
}

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

func TestPairwiseProjectManualCommandRunsForegroundInBoundContainer(t *testing.T) {
	command := pairwiseProjectManualCommand(
		"container-a",
		"/workspace/project-a",
		pairwiseProjectLaunch{Command: "npm run dev", Port: 5173, Host: "::1"},
		4821,
	)
	for _, want := range []string{
		"container='container-a'",
		"docker exec -it -w '/workspace/project-a' \"$container\" sh -lc",
		"项目地址：http://127.0.0.1:4821",
		"docker inspect --format",
		"proxy_pid=$!",
		"trap cleanup EXIT HUP INT TERM",
		`s.listen(4821,"127.0.0.1"`,
		"exec npm run dev -- --host 0.0.0.0 --port 4821 --strictPort",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("command missing %q: %s", want, command)
		}
	}
	for _, forbidden := range []string{"setsid", "pinru-project"} {
		if strings.Contains(command, forbidden) {
			t.Fatalf("manual command contains %q: %s", forbidden, command)
		}
	}
	if output, err := exec.Command("sh", "-n", "-c", command).CombinedOutput(); err != nil {
		t.Fatalf("manual command is not valid shell: %v: %s", err, output)
	}
}

func TestRandomPairwiseProjectAvailablePortSkipsOccupiedPort(t *testing.T) {
	checked := []int{}
	port, err := randomPairwiseProjectAvailablePort(bytes.NewReader([]byte{0, 0, 255, 255}), func(candidate int) bool {
		checked = append(checked, candidate)
		return len(checked) == 2
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(checked) != 2 || port != checked[1] || checked[0] == checked[1] {
		t.Fatalf("port = %d, checked = %#v", port, checked)
	}
}

func TestPairwiseProjectManualCommandUsesNextFlags(t *testing.T) {
	command := pairwiseProjectManualCommand(
		"container-b",
		"/workspace/project-b",
		pairwiseProjectLaunch{Command: "corepack pnpm run dev", Port: 3000, Host: "127.0.0.1"},
		7351,
	)
	if !strings.Contains(command, "corepack pnpm run dev -- -H 0.0.0.0 -p 7351") {
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
			if strings.Contains(strings.Join(args, " "), "cat ") {
				return []byte("4821\n"), nil
			}
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
	if len(commands) != 3 || commands[1] != "/usr/bin/open http://127.0.0.1:4821" || !strings.HasPrefix(commands[2], "/usr/sbin/screencapture ") {
		t.Fatalf("commands = %#v", commands)
	}
}
