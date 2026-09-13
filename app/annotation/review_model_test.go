package annotation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultReviewModelFingerprintInvalidatesCacheAndCurrentState(t *testing.T) {
	s, trace, _ := annotationFixture(t)
	configHome := t.TempDir()
	t.Setenv("CODEX_HOME", configHome)
	configPath := filepath.Join(configHome, "config.toml")
	if err := os.WriteFile(configPath, []byte("model = \"first-model\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cli, calls := fakeReviewCLI(t)
	s.cli = cli
	if _, err := s.Capture(CaptureRequest{TaskID: "题目-1", TracePath: trace}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Review(ReviewRequest{TaskID: "题目-1", PromptID: "p1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Review(ReviewRequest{TaskID: "题目-1", PromptID: "p1"}); err != nil {
		t.Fatal(err)
	}
	assertReviewCallCount(t, calls, 1)

	if err := os.WriteFile(configPath, []byte("model = \"second-model\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases, err := s.ListCases("batch")
	if err != nil {
		t.Fatal(err)
	}
	if current := cases[0].Rounds[0].Evaluations[0].Current; current == nil || *current {
		t.Fatalf("evaluation current = %v after default model config changed", current)
	}
	if _, err := s.Review(ReviewRequest{TaskID: "题目-1", PromptID: "p1"}); err != nil {
		t.Fatal(err)
	}
	assertReviewCallCount(t, calls, 2)
}

func TestReviewModelUsesExplicitSettingWithoutReadingCodexConfig(t *testing.T) {
	s, _, _ := annotationFixture(t)
	if err := s.store.SetConfig("annotation_review_model", " configured-model "); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "missing"))
	cliModel, label, err := s.reviewModel()
	if err != nil {
		t.Fatal(err)
	}
	if cliModel != "configured-model" || label != "configured-model" {
		t.Fatalf("review model = %q / %q", cliModel, label)
	}
}

func TestReviewModelUsesStableMissingConfigLabel(t *testing.T) {
	s, _, _ := annotationFixture(t)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "missing"))
	cliModel, label, err := s.reviewModel()
	if err != nil {
		t.Fatal(err)
	}
	if cliModel != "" || label != defaultReviewModelLabel {
		t.Fatalf("review model = %q / %q", cliModel, label)
	}
}

func TestReviewModelReportsUnreadableConfigWithoutItsContents(t *testing.T) {
	s, _, _ := annotationFixture(t)
	configHome := t.TempDir()
	t.Setenv("CODEX_HOME", configHome)
	configPath := filepath.Join(configHome, "config.toml")
	if err := os.Mkdir(configPath, 0o700); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.reviewModel()
	if err == nil || !strings.Contains(err.Error(), "读取 Codex 默认模型配置") {
		t.Fatalf("reviewModel error = %v", err)
	}
}

func assertReviewCallCount(t *testing.T, path string, want int) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, value := range raw {
		if value == '\n' {
			count++
		}
	}
	if count != want {
		t.Fatalf("review calls = %d, want %d", count, want)
	}
}
