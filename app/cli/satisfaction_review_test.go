package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSatisfactionPromptKeepsFiveDimensionRulesAndEvidenceBoundaries(t *testing.T) {
	p := buildSatisfactionPrompt(SatisfactionReviewRequest{InputPath: "/review/input.json", SkillDir: "/review/skill"})
	for _, want := range []string{"SKILL.md", "input.json", "原始 Prompt", "五维", "自然", "不得", "副本"} {
		if !strings.Contains(p, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{"90 分", "过程不满意：", "最多三句"} {
		if strings.Contains(p, bad) {
			t.Errorf("legacy rule %q leaked", bad)
		}
	}
}

func TestSatisfactionSchemaRequiresExactScoreAndDescriptionCounts(t *testing.T) {
	schema := satisfactionSchema()
	props := schema["properties"].(map[string]any)
	for _, name := range []string{"scores", "descriptions"} {
		p := props[name].(map[string]any)
		if p["minItems"] != 5 || p["maxItems"] != 5 {
			t.Fatalf("%s not exactly five", name)
		}
	}
	requirementChecks := props["requirementChecks"].(map[string]any)
	if requirementChecks["minItems"] != 1 {
		t.Fatalf("requirementChecks minItems = %#v, want 1", requirementChecks["minItems"])
	}
}

func TestSatisfactionSchemaRestrictsOperatingSystemToExportValues(t *testing.T) {
	schema := satisfactionSchema()
	props := schema["properties"].(map[string]any)
	osSchema := props["os"].(map[string]any)
	values, ok := osSchema["enum"].([]string)
	if !ok {
		t.Fatalf("os enum = %#v, want []string", osSchema["enum"])
	}
	want := []string{"", "MacOS/Linux", "Windows"}
	if strings.Join(values, "|") != strings.Join(want, "|") {
		t.Fatalf("os enum = %#v, want %#v", values, want)
	}
}

func TestRunSatisfactionReviewRejectsEmptyRequirementChecks(t *testing.T) {
	workDir := t.TempDir()
	payload := validReviewJSON(t, 5)
	var document map[string]any
	if err := json.Unmarshal([]byte(payload), &document); err != nil {
		t.Fatal(err)
	}
	document["requirementChecks"] = []any{}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	binary := writeFakeCodex(t, fakeCodexWritesReview(t, string(raw), "", ""))
	service := NewWithResolver(func(string) (string, error) { return binary, nil })
	if evaluation, err := service.RunSatisfactionReview(context.Background(), SatisfactionReviewRequest{
		WorkDir: workDir, SkillDir: filepath.Join(workDir, "skill"), InputPath: filepath.Join(workDir, "input.json"),
	}, nil); err == nil || !strings.Contains(err.Error(), "逐项需求核验") {
		t.Fatalf("RunSatisfactionReview() = %#v, %v; want empty requirement checks rejection", evaluation, err)
	}
}

func TestRunSatisfactionReviewAcceptsExactlyFiveItemsAndRejectsFourOrSix(t *testing.T) {
	for _, count := range []int{5, 4, 6} {
		t.Run(fmt.Sprintf("count_%d", count), func(t *testing.T) {
			workDir := t.TempDir()
			binary := writeFakeCodex(t, fakeCodexWritesReview(t, validReviewJSON(t, count), "", ""))
			service := NewWithResolver(func(string) (string, error) { return binary, nil })

			evaluation, err := service.RunSatisfactionReview(context.Background(), SatisfactionReviewRequest{
				WorkDir: workDir, SkillDir: filepath.Join(workDir, "skill"), InputPath: filepath.Join(workDir, "input.json"),
			}, nil)
			if count == 5 {
				if err != nil {
					t.Fatalf("RunSatisfactionReview() error = %v", err)
				}
				if evaluation == nil || evaluation.Scores[4] == nil || *evaluation.Scores[4] != 5 || evaluation.Descriptions[4] != "dimension 5" {
					t.Fatalf("evaluation = %#v", evaluation)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "恰好包含五项") {
				t.Fatalf("RunSatisfactionReview() error = %v, want exact-count rejection", err)
			}
		})
	}
}

func TestRunSatisfactionReviewCancellationReturnsWithoutHanging(t *testing.T) {
	workDir := t.TempDir()
	binary := writeFakeCodex(t, "exec sleep 30\n")
	service := NewWithResolver(func(string) (string, error) { return binary, nil })
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	started := time.Now()
	_, err := service.RunSatisfactionReview(ctx, SatisfactionReviewRequest{
		WorkDir: workDir, SkillDir: filepath.Join(workDir, "skill"), InputPath: filepath.Join(workDir, "input.json"),
	}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunSatisfactionReview() error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("cancellation took %v, runner appears hung", elapsed)
	}
}

func TestRunSatisfactionReviewPreservesEvaluatorToolAndTestOutput(t *testing.T) {
	workDir := t.TempDir()
	binary := writeFakeCodex(t, fakeCodexWritesReview(
		t, validReviewJSON(t, 5),
		`{"type":"item.completed","item":{"type":"command_execution","command":"go test ./...","aggregated_output":"PASS"}}`,
		"verification stderr: integration test passed",
	))
	service := NewWithResolver(func(string) (string, error) { return binary, nil })
	if _, err := service.RunSatisfactionReview(context.Background(), SatisfactionReviewRequest{
		WorkDir: workDir, SkillDir: filepath.Join(workDir, "skill"), InputPath: filepath.Join(workDir, "input.json"),
	}, nil); err != nil {
		t.Fatalf("RunSatisfactionReview() error = %v", err)
	}
	logData, err := os.ReadFile(filepath.Join(workDir, "evaluator.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"command_execution", "go test ./...", "PASS", "verification stderr", "integration test passed"} {
		if !strings.Contains(string(logData), want) {
			t.Fatalf("evaluator.log = %q, missing %q", logData, want)
		}
	}
}

func TestRunSatisfactionReviewRejectsTrailingDocumentAfterJSON(t *testing.T) {
	workDir := t.TempDir()
	payload := validReviewJSON(t, 5) + "\n# evaluator notes must not be accepted\n"
	binary := writeFakeCodex(t, fakeCodexWritesReview(t, payload, "", ""))
	service := NewWithResolver(func(string) (string, error) { return binary, nil })
	if evaluation, err := service.RunSatisfactionReview(context.Background(), SatisfactionReviewRequest{
		WorkDir: workDir, SkillDir: filepath.Join(workDir, "skill"), InputPath: filepath.Join(workDir, "input.json"),
	}, nil); err == nil {
		t.Fatalf("RunSatisfactionReview() = %#v, nil; want trailing-content rejection", evaluation)
	}
}

func TestRunSatisfactionReviewAcceptsSingleJSONCodeFence(t *testing.T) {
	workDir := t.TempDir()
	payload := "```json\n" + validReviewJSON(t, 5) + "\n```\n"
	binary := writeFakeCodex(t, fakeCodexWritesReview(t, payload, "", ""))
	service := NewWithResolver(func(string) (string, error) { return binary, nil })
	evaluation, err := service.RunSatisfactionReview(context.Background(), SatisfactionReviewRequest{
		WorkDir: workDir, SkillDir: filepath.Join(workDir, "skill"), InputPath: filepath.Join(workDir, "input.json"),
	}, nil)
	if err != nil {
		t.Fatalf("RunSatisfactionReview() error = %v", err)
	}
	if evaluation == nil || evaluation.Status != "ready" || evaluation.Scores[4] == nil || *evaluation.Scores[4] != 5 {
		t.Fatalf("evaluation = %#v", evaluation)
	}
}

func TestRunSatisfactionReviewReportsMalformedJSONBeforeShapeErrors(t *testing.T) {
	workDir := t.TempDir()
	binary := writeFakeCodex(t, fakeCodexWritesReview(t, "```json\n{broken}\n```\n", "", ""))
	service := NewWithResolver(func(string) (string, error) { return binary, nil })
	_, err := service.RunSatisfactionReview(context.Background(), SatisfactionReviewRequest{
		WorkDir: workDir, SkillDir: filepath.Join(workDir, "skill"), InputPath: filepath.Join(workDir, "input.json"),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "评分 JSON 无效") || strings.Contains(err.Error(), "恰好包含五项") {
		t.Fatalf("RunSatisfactionReview() error = %v, want an accurate JSON parse error", err)
	}
}

func TestDeepSeekCodexConfigUsesIsolatedHomeAndDoesNotExposeKeyInArgs(t *testing.T) {
	home, cleanup, err := prepareDeepSeekCodexHome(DeepSeekCodexConfig{
		Model: "deepseek-v4-flash", BaseURL: "https://api.deepseek.com", APIKey: "secret-key", ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	config, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	models, err := os.ReadFile(filepath.Join(home, "models.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`model = "deepseek-v4-flash"`, `model_provider = "deepseek"`, `wire_api = "responses"`, `model_reasoning_effort = "high"`, `experimental_bearer_token = "secret-key"`} {
		if !strings.Contains(string(config), want) {
			t.Fatalf("config.toml missing %q: %s", want, config)
		}
	}
	if !strings.Contains(string(models), `"slug": "deepseek-v4-flash"`) {
		t.Fatalf("models.json = %s", models)
	}
}

func TestDeepSeekCodexConfigIsAcceptedByInstalledCodex(t *testing.T) {
	binary, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("codex CLI is not installed")
	}
	requestSeen := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case requestSeen <- struct{}{}:
		default:
		}
		http.Error(w, `{"error":{"message":"test endpoint"}}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	home, cleanup, err := prepareDeepSeekCodexHome(DeepSeekCodexConfig{Model: "deepseek-v4-flash", BaseURL: server.URL, APIKey: "test-key", ReasoningEffort: "high"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "exec", "-", "--skip-git-repo-check", "--ephemeral", "--json")
	cmd.Dir = t.TempDir()
	cmd.Env = applyEnvOverrides(os.Environ(), map[string]string{"CODEX_HOME": home})
	cmd.Stdin = strings.NewReader("reply with ok")
	out, _ := cmd.CombinedOutput()
	select {
	case <-requestSeen:
	case <-ctx.Done():
		t.Fatalf("Codex did not reach the configured endpoint: %s", out)
	}
	for _, bad := range []string{"model catalog", "models.json", "config.toml parse", "unknown field"} {
		if strings.Contains(strings.ToLower(string(out)), strings.ToLower(bad)) {
			t.Fatalf("Codex rejected generated configuration: %s", out)
		}
	}
}

func validReviewJSON(t *testing.T, count int) string {
	t.Helper()
	scores := make([]int, count)
	descriptions := make([]string, count)
	for index := 0; index < count; index++ {
		scores[index] = 5
		descriptions[index] = fmt.Sprintf("dimension %d", index+1)
	}
	payload := map[string]any{
		"status": "ready", "scores": scores, "descriptions": descriptions,
		"taskType": "feature迭代", "difficulty": "中等", "language": "Go",
		"environment": "无外部依赖", "harnessVersion": "2.1.0", "os": "MacOS/Linux",
		"evidence": []string{"trace.jsonl:1-3"}, "missing": []string{}, "issues": []any{},
		"requirementChecks": []any{map[string]any{"requirement": "完成用户要求", "status": "completed", "evidence": "code/service.go:10; go test ./... PASS"}},
		"nextPrompt":        "", "nextPromptType": "",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func fakeCodexWritesReview(t *testing.T, payload, stdout, stderr string) string {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "evaluation.json")
	if err := os.WriteFile(fixture, []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	return strings.Join([]string{
		`out=""`,
		`while [ "$#" -gt 0 ]; do`,
		`  if [ "$1" = "-o" ]; then shift; out="$1"; fi`,
		`  shift`,
		`done`,
		`cp ` + shellSingleQuote(fixture) + ` "$out"`,
		`printf '%s\n' ` + shellSingleQuote(stdout),
		`printf '%s\n' ` + shellSingleQuote(stderr) + ` >&2`,
	}, "\n") + "\n"
}

func writeFakeCodex(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
