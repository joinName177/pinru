package annotation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateEvaluationAcceptsFiveIndependentReadyScores(t *testing.T) {
	round := Round{PromptID: "p-1", EvidenceHash: "evidence-hash"}
	evaluation := validEvaluation("evidence-hash")
	if err := ValidateEvaluation(round, evaluation); err != nil {
		t.Fatalf("ValidateEvaluation() error = %v", err)
	}
}

func TestValidateEvaluationAcceptsOnlySpecificMissingEvidenceForNullableScores(t *testing.T) {
	round := Round{PromptID: "p-1", EvidenceHash: "evidence-hash"}
	evaluation := validEvaluation("evidence-hash")
	evaluation.Status = "needs_evidence"
	evaluation.Scores[2] = nil
	evaluation.Descriptions[2] = ""
	evaluation.Missing = []string{"任务规划：缺少本轮工具调用与计划事件"}
	if err := ValidateEvaluation(round, evaluation); err != nil {
		t.Fatalf("ValidateEvaluation() error = %v", err)
	}

	evaluation.Missing = []string{"  "}
	if err := ValidateEvaluation(round, evaluation); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("ValidateEvaluation() error = %v, want specific missing evidence error", err)
	}
}

func TestValidateEvaluationNeedsEvidenceCanKeepAllSupportedScores(t *testing.T) {
	round := Round{PromptID: "p-1", EvidenceHash: "evidence-hash"}
	evaluation := validEvaluation("evidence-hash")
	evaluation.Status = "needs_evidence"
	evaluation.HarnessVersion = ""
	evaluation.Missing = []string{"Harness 版本：轨迹事件未记录 version"}
	if err := ValidateEvaluation(round, evaluation); err != nil {
		t.Fatalf("ValidateEvaluation() error = %v", err)
	}
}

func TestValidateEvaluationRejectsBadEnumsScoreAndEvidenceMismatch(t *testing.T) {
	round := Round{PromptID: "p-1", EvidenceHash: "round-hash"}
	tests := []struct {
		name   string
		mutate func(*Evaluation)
		want   string
	}{
		{"old difficulty enum", func(e *Evaluation) { e.Difficulty = "一般" }, "difficulty"},
		{"task type case mismatch", func(e *Evaluation) { e.TaskType = "Feature迭代" }, "taskType"},
		{"environment enum", func(e *Evaluation) { e.Environment = "docker" }, "environment"},
		{"os enum", func(e *Evaluation) { e.OS = "Linux" }, "os"},
		{"score zero", func(e *Evaluation) { zero := 0; e.Scores[1] = &zero }, "score"},
		{"missing description", func(e *Evaluation) { e.Descriptions[4] = "" }, "description"},
		{"wrong evidence", func(e *Evaluation) { e.EvidenceHash = "old-hash" }, "evidenceHash"},
		{"bad issue kind", func(e *Evaluation) { e.Issues = []Issue{{Description: "x", Evidence: "event 2", Kind: "quality"}} }, "kind"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evaluation := validEvaluation("round-hash")
			test.mutate(&evaluation)
			if err := ValidateEvaluation(round, evaluation); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateEvaluation() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateEvaluationRejectsSimpleScoreDescriptionContradictions(t *testing.T) {
	round := Round{PromptID: "p-1", EvidenceHash: "round-hash"}
	evaluation := validEvaluation("round-hash")
	evaluation.Descriptions[4] = "执行过程中遗漏了一个边界，因此扣1分。"
	if err := ValidateEvaluation(round, evaluation); err == nil || !strings.Contains(err.Error(), "contradict") {
		t.Fatalf("score 5 contradiction error = %v", err)
	}

	four := 4
	evaluation = validEvaluation("round-hash")
	evaluation.Scores[0] = &four
	evaluation.Descriptions[0] = "所有交付均完整，无任何问题。"
	if err := ValidateEvaluation(round, evaluation); err == nil || !strings.Contains(err.Error(), "contradict") {
		t.Fatalf("score 4 contradiction error = %v", err)
	}
}

func TestValidateEvaluationDoesNotTreatNegatedContradictionPhraseAsAClaim(t *testing.T) {
	round := Round{PromptID: "p-1", EvidenceHash: "round-hash"}
	evaluation := validEvaluation("round-hash")
	evaluation.Descriptions[4] = "已核对执行证据，没有扣1分的情形。"
	if err := ValidateEvaluation(round, evaluation); err != nil {
		t.Fatalf("negated deduction rejected: %v", err)
	}

	four := 4
	evaluation = validEvaluation("round-hash")
	evaluation.Scores[0] = &four
	evaluation.Descriptions[0] = "并非无任何问题，产物仍缺少关键实现。"
	if err := ValidateEvaluation(round, evaluation); err != nil {
		t.Fatalf("negated perfect claim rejected: %v", err)
	}
}

func TestPreflightCountsLowScoredReadyRoundsAndBlocksIncompleteMaterial(t *testing.T) {
	sha := strings.Repeat("a", 40)
	evaluation := validEvaluation("trace-hash")
	one := 1
	evaluation.Scores[0] = &one
	evaluation.Descriptions[0] = "交付缺少关键实现，证据见产物差异。"
	cases := []Case{{
		TaskID: "task-1", ProjectID: "project-1", Completed: true,
		InitialSHA: sha, SnapshotURL: "https://github.com/acme/repo/commit/" + sha,
		TracePath: "/home/node/.claude/projects/-workspace/session.jsonl",
		Rounds:    []Round{{PromptID: "p-1", SessionID: "s-1", Order: 1, Status: "complete", EvidenceHash: "trace-hash", Evaluations: []Evaluation{evaluation}}},
	}}
	report := Preflight(cases)
	if report.Tasks != 1 || report.Rounds != 1 || report.Ready != 1 || len(report.Issues) != 0 {
		t.Fatalf("Preflight() = %#v", report)
	}

	cases[0].Completed = false
	cases[0].Rounds[0].Status = "pending"
	report = Preflight(cases)
	if report.Ready != 0 || len(report.Issues) < 2 {
		t.Fatalf("blocked Preflight() = %#v", report)
	}
}

func TestPreflightRequiresFullMatchingSnapshotAndLatestMatchingEvaluation(t *testing.T) {
	sha := strings.Repeat("b", 40)
	old := validEvaluation("old-hash")
	currentNeedsEvidence := validEvaluation("new-hash")
	currentNeedsEvidence.Status = "needs_evidence"
	currentNeedsEvidence.Scores[0] = nil
	currentNeedsEvidence.Descriptions[0] = ""
	currentNeedsEvidence.Missing = []string{"交付完整性：缺本轮代码快照"}
	c := Case{
		TaskID: "task-1", Completed: true, InitialSHA: sha,
		SnapshotURL: "https://github.com/acme/repo/commit/" + strings.Repeat("c", 40),
		Rounds:      []Round{{PromptID: "p-1", SessionID: "s-1", Status: "complete", EvidenceHash: "new-hash", Evaluations: []Evaluation{old, currentNeedsEvidence}}},
	}
	report := Preflight([]Case{c})
	if report.Ready != 0 || !containsIssue(report.Issues, "snapshot") || !containsIssue(report.Issues, "ready") {
		t.Fatalf("Preflight() = %#v", report)
	}
}

func TestPreflightBlocksMoreThanTenRealRoundsWithoutDroppingThem(t *testing.T) {
	sha := strings.Repeat("d", 40)
	rounds := make([]Round, 11)
	for index := range rounds {
		hash := "hash-" + string(rune('a'+index))
		rounds[index] = Round{PromptID: "prompt-" + string(rune('a'+index)), SessionID: "s", Order: index + 1, Status: "complete", EvidenceHash: hash, Evaluations: []Evaluation{validEvaluation(hash)}}
	}
	report := Preflight([]Case{{TaskID: "task", Completed: true, InitialSHA: sha, SnapshotURL: "https://github.com/acme/repo/commit/" + sha, Rounds: rounds}})
	if report.Rounds != 11 || report.Ready != 0 || !containsIssue(report.Issues, "10") {
		t.Fatalf("Preflight() = %#v", report)
	}
}

func TestPreflightRequiresFrozenCaptureDirectoryAndArtifacts(t *testing.T) {
	root := t.TempDir()
	tracePath := filepath.Join(root, "trace.jsonl")
	codePath := filepath.Join(root, "code")
	mustWriteFile(t, tracePath, "{}\n", 0644)
	if err := os.Mkdir(codePath, 0755); err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("e", 40)
	report := Preflight([]Case{{
		TaskID: "task", Completed: true, InitialSHA: sha,
		SnapshotURL: "https://github.com/acme/repo/commit/" + sha,
		Captures:    []Capture{{ID: "capture", Dir: filepath.Join(root, "missing-capture"), TracePath: tracePath, CodePath: codePath, Hash: "code-hash"}},
		Rounds:      []Round{{PromptID: "prompt", SessionID: "session", Status: "complete", EvidenceHash: "trace-hash", CaptureID: "capture", Evaluations: []Evaluation{validEvaluation("trace-hash")}}},
	}})
	if report.Ready != 0 || !containsIssue(report.Issues, "capture directory") {
		t.Fatalf("Preflight() = %#v", report)
	}
}

func validEvaluation(evidenceHash string) Evaluation {
	five := 5
	return Evaluation{
		ID: "eval-1", CreatedAt: 10, SkillHash: "skill-hash", Model: "model",
		EvidenceHash: evidenceHash, Status: "ready",
		Scores:       [5]*int{&five, &five, &five, &five, &five},
		Descriptions: [5]string{"交付完整。", "遵循指令。", "规划合理。", "推理准确。", "执行有效。"},
		TaskType:     "feature迭代", Difficulty: "中等", Language: "Go",
		Environment: "无外部依赖", HarnessVersion: "2.1.0", OS: "MacOS/Linux",
		Evidence: []string{"trace.jsonl:1-4", "code/tree"},
	}
}

func containsIssue(issues []string, fragment string) bool {
	for _, issue := range issues {
		if strings.Contains(issue, fragment) {
			return true
		}
	}
	return false
}
