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

func TestValidateEvaluationAcceptsLegacyRecordWithoutRequirementChecks(t *testing.T) {
	round := Round{PromptID: "p-1", EvidenceHash: "evidence-hash"}
	evaluation := validEvaluation("evidence-hash")
	if err := ValidateEvaluation(round, evaluation); err != nil {
		t.Fatalf("legacy evaluation rejected: %v", err)
	}
}

func TestValidateEvaluationChecksRequirementCheckFieldsAndStatus(t *testing.T) {
	round := Round{PromptID: "p-1", EvidenceHash: "evidence-hash"}
	tests := []struct {
		name  string
		check RequirementCheck
		want  string
	}{
		{"empty requirement", RequirementCheck{Status: "completed", Evidence: "service.go:10"}, "requirement"},
		{"invalid status", RequirementCheck{Requirement: "保存数据", Status: "unknown", Evidence: "service.go:10"}, "status"},
		{"empty evidence", RequirementCheck{Requirement: "保存数据", Status: "completed"}, "evidence"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evaluation := validEvaluation("evidence-hash")
			evaluation.RequirementChecks = []RequirementCheck{test.check}
			if err := ValidateEvaluation(round, evaluation); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateEvaluation() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateEvaluationRequiresBugForFailedRequirement(t *testing.T) {
	round := Round{PromptID: "p-1", EvidenceHash: "evidence-hash"}
	evaluation := validEvaluation("evidence-hash")
	evaluation.RequirementChecks = []RequirementCheck{{Requirement: "保存数据", Status: "failed", Evidence: "service.go:10 未实现写入"}}
	if err := ValidateEvaluation(round, evaluation); err == nil || !strings.Contains(err.Error(), "bug") {
		t.Fatalf("ValidateEvaluation() error = %v, want bug consistency error", err)
	}

	four := 4
	evaluation.Scores[0] = &four
	evaluation.Descriptions[0] = "保存功能未完成。"
	evaluation.DescriptionChecks[0] = DescriptionCheck{Judgment: "保存功能未完成", Location: "service.go:10", Behavior: "没有写入数据", Consequence: "重新打开后无法读取"}
	evaluation.Descriptions[0] = "在 service.go:10 的保存步骤，保存功能未完成：没有写入数据，导致重新打开后无法读取。"
	evaluation.Issues = []Issue{{Description: "未保存数据", Evidence: "service.go:10", Kind: "bug"}}
	evaluation.NextPrompt = "修复保存数据未落盘的问题，触发保存后应能重新读取"
	evaluation.NextPromptType = "Bug修复"
	if err := ValidateEvaluation(round, evaluation); err != nil {
		t.Fatalf("failed requirement with bug rejected: %v", err)
	}
}

func TestValidateEvaluationRequiresConcreteDescriptionChecksBelowFive(t *testing.T) {
	round := Round{PromptID: "p-1", EvidenceHash: "evidence-hash"}
	four := 4
	evaluation := validEvaluation("evidence-hash")
	evaluation.Scores[2] = &four
	evaluation.QualityVersion = 2
	evaluation.Descriptions[2] = "规划阶段存在遗漏。"
	if err := ValidateEvaluation(round, evaluation); err == nil || !strings.Contains(err.Error(), "description check 3") {
		t.Fatalf("ValidateEvaluation() error = %v, want structured description evidence error", err)
	}
	evaluation.DescriptionChecks[2] = DescriptionCheck{
		Judgment:    "规划阶段遗漏了前置检查",
		Location:    "第1轮执行 npm run build 前",
		Behavior:    "未先确认 package.json 中的构建脚本",
		Consequence: "首次构建使用了不存在的脚本并退出 1",
	}
	evaluation.Descriptions[2] = "第1轮执行 npm run build 前，规划阶段遗漏了前置检查，未先确认 package.json 中的构建脚本，首次构建使用了不存在的脚本并退出 1。"
	if err := ValidateEvaluation(round, evaluation); err != nil {
		t.Fatalf("concrete non-perfect description rejected: %v", err)
	}
}

func TestValidateEvaluationRequiresMissingEvidenceForUnverifiedRequirement(t *testing.T) {
	round := Round{PromptID: "p-1", EvidenceHash: "evidence-hash"}
	evaluation := validEvaluation("evidence-hash")
	evaluation.RequirementChecks = []RequirementCheck{{Requirement: "浏览器交互", Status: "unverified", Evidence: "静态检查无法确认运行时交互"}}
	if err := ValidateEvaluation(round, evaluation); err == nil || !strings.Contains(err.Error(), "needs_evidence") {
		t.Fatalf("ValidateEvaluation() error = %v, want needs_evidence consistency error", err)
	}

	evaluation.Status = "needs_evidence"
	evaluation.Missing = []string{"浏览器交互：缺少可定位的运行记录"}
	if err := ValidateEvaluation(round, evaluation); err == nil || !strings.Contains(err.Error(), "delivery score") {
		t.Fatalf("ValidateEvaluation() error = %v, want unverified delivery score error", err)
	}

	evaluation.Scores[0] = nil
	evaluation.Descriptions[0] = ""
	if err := ValidateEvaluation(round, evaluation); err != nil {
		t.Fatalf("unverified requirement with missing evidence rejected: %v", err)
	}
}

func TestValidateEvaluationAllowsFailedAndUnverifiedRequirementsTogether(t *testing.T) {
	round := Round{PromptID: "p-1", EvidenceHash: "evidence-hash"}
	evaluation := validEvaluation("evidence-hash")
	four := 4
	evaluation.Status = "needs_evidence"
	evaluation.Scores[0] = &four
	evaluation.Descriptions[0] = "保存功能未完成；浏览器交互缺少运行证据。"
	evaluation.Missing = []string{"浏览器交互：缺少可定位的运行记录"}
	evaluation.RequirementChecks = []RequirementCheck{
		{Requirement: "保存数据", Status: "failed", Evidence: "service.go:10 未实现写入"},
		{Requirement: "浏览器交互", Status: "unverified", Evidence: "静态检查无法确认运行时交互"},
	}
	evaluation.Issues = []Issue{{Description: "未保存数据", Evidence: "service.go:10", Kind: "bug"}}
	evaluation.NextPrompt = "修复保存数据未落盘的问题，触发保存后应能重新读取"
	evaluation.NextPromptType = "Bug修复"
	if err := ValidateEvaluation(round, evaluation); err != nil {
		t.Fatalf("mixed failed and unverified requirements rejected: %v", err)
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
