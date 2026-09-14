package annotation

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

var (
	fullSHA              = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	fivePointDeduction   = regexp.MustCompile(`扣\s*(?:1|一)\s*分`)
	negatedDeduction     = regexp.MustCompile(`(?:没有|并未|未|无需|不)\s*扣\s*(?:1|一)\s*分`)
	perfectClaimPatterns = []string{"无任何问题", "没有任何问题", "无任何不足", "没有任何不足", "全部完美", "完全无误", "满分表现"}
)

var allowedTaskTypes = map[string]struct{}{
	"Bug修复": {}, "0-1代码生成": {}, "feature迭代": {}, "代码理解": {},
	"代码重构": {}, "工程化": {}, "代码测试": {},
}

var allowedDifficulties = map[string]struct{}{
	"简单": {}, "中等": {}, "困难": {}, "地狱": {},
}

var allowedEnvironments = map[string]struct{}{
	"无外部依赖": {}, "有外部依赖，未容器化": {}, "已容器化，可一键起环境": {},
}

var allowedOperatingSystems = map[string]struct{}{
	"MacOS/Linux": {}, "Windows": {},
}

var allowedIssueKinds = map[string]struct{}{
	"bug": {}, "process": {}, "evidence": {},
}

var allowedRequirementCheckStatuses = map[string]struct{}{
	"completed": {}, "failed": {}, "unverified": {},
}

// ValidateEvaluation checks only structural, enum, evidence, and obvious
// score-description consistency. It deliberately does not invent a semantic
// score or claim that the underlying implementation was verified.
func ValidateEvaluation(round Round, evaluation Evaluation) error {
	if strings.TrimSpace(evaluation.ID) == "" {
		return fmt.Errorf("evaluation id is required")
	}
	if evaluation.CreatedAt <= 0 {
		return fmt.Errorf("evaluation createdAt must be positive")
	}
	if strings.TrimSpace(evaluation.SkillHash) == "" {
		return fmt.Errorf("evaluation skillHash is required")
	}
	if strings.TrimSpace(evaluation.Model) == "" {
		return fmt.Errorf("evaluation model is required")
	}
	if evaluation.EvidenceHash == "" || evaluation.EvidenceHash != round.EvidenceHash {
		return fmt.Errorf("evaluation evidenceHash does not match the round")
	}
	if evaluation.Status != "ready" && evaluation.Status != "needs_evidence" {
		return fmt.Errorf("evaluation status %q is invalid", evaluation.Status)
	}
	needsEvidence := evaluation.Status == "needs_evidence"
	for index, missing := range evaluation.Missing {
		if strings.TrimSpace(missing) == "" {
			return fmt.Errorf("evaluation missing evidence %d must be specific", index+1)
		}
	}
	if needsEvidence && len(evaluation.Missing) == 0 {
		return fmt.Errorf("needs_evidence evaluation requires specific missing evidence")
	}
	if evaluation.TaskType == "" && needsEvidence {
		// The missing list records why this metadata could not be established.
	} else if _, ok := allowedTaskTypes[evaluation.TaskType]; !ok {
		return fmt.Errorf("evaluation taskType %q is invalid", evaluation.TaskType)
	}
	if evaluation.Difficulty == "" && needsEvidence {
	} else if _, ok := allowedDifficulties[evaluation.Difficulty]; !ok {
		return fmt.Errorf("evaluation difficulty %q is invalid", evaluation.Difficulty)
	}
	if strings.TrimSpace(evaluation.Language) == "" && !needsEvidence {
		return fmt.Errorf("evaluation language is required")
	}
	if evaluation.Environment == "" && needsEvidence {
	} else if _, ok := allowedEnvironments[evaluation.Environment]; !ok {
		return fmt.Errorf("evaluation environment %q is invalid", evaluation.Environment)
	}
	if strings.TrimSpace(evaluation.HarnessVersion) == "" && !needsEvidence {
		return fmt.Errorf("evaluation harnessVersion is required")
	}
	if evaluation.OS == "" && needsEvidence {
	} else if _, ok := allowedOperatingSystems[evaluation.OS]; !ok {
		return fmt.Errorf("evaluation os %q is invalid", evaluation.OS)
	}

	nilScores := 0
	for index, score := range evaluation.Scores {
		description := strings.TrimSpace(evaluation.Descriptions[index])
		if score == nil {
			nilScores++
			continue
		}
		if *score < 1 || *score > 5 {
			return fmt.Errorf("evaluation score %d must be an integer from 1 to 5", index+1)
		}
		if description == "" {
			return fmt.Errorf("evaluation description %d is required for its score", index+1)
		}
		if *score == 5 && fivePointDeduction.MatchString(description) && !negatedDeduction.MatchString(description) {
			return fmt.Errorf("evaluation score and description %d contradict: score 5 claims a deduction", index+1)
		}
		if *score < 5 {
			for _, claim := range perfectClaimPatterns {
				if strings.Contains(description, claim) && !containsNegatedClaim(description, claim) {
					return fmt.Errorf("evaluation score and description %d contradict: non-perfect score claims no shortcoming", index+1)
				}
			}
			if evaluation.QualityVersion >= 2 {
				check := evaluation.DescriptionChecks[index]
				parts := []string{check.Judgment, check.Location, check.Behavior, check.Consequence}
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part == "" || !strings.Contains(description, part) {
						return fmt.Errorf("evaluation description check %d must include judgment, location, behavior, and consequence verbatim", index+1)
					}
				}
			}
		}
	}

	if len(evaluation.Evidence) == 0 && !needsEvidence {
		return fmt.Errorf("evaluation evidence is required")
	}
	for index, evidence := range evaluation.Evidence {
		if strings.TrimSpace(evidence) == "" {
			return fmt.Errorf("evaluation evidence %d is empty", index+1)
		}
	}
	hasFailedRequirement := false
	hasUnverifiedRequirement := false
	for index, check := range evaluation.RequirementChecks {
		if strings.TrimSpace(check.Requirement) == "" {
			return fmt.Errorf("evaluation requirement check %d requirement is required", index+1)
		}
		if _, ok := allowedRequirementCheckStatuses[check.Status]; !ok {
			return fmt.Errorf("evaluation requirement check %d status %q is invalid", index+1, check.Status)
		}
		if strings.TrimSpace(check.Evidence) == "" {
			return fmt.Errorf("evaluation requirement check %d evidence is required", index+1)
		}
		hasFailedRequirement = hasFailedRequirement || check.Status == "failed"
		hasUnverifiedRequirement = hasUnverifiedRequirement || check.Status == "unverified"
	}
	if hasUnverifiedRequirement && !needsEvidence {
		return fmt.Errorf("unverified requirement requires needs_evidence status and specific missing evidence")
	}
	if hasUnverifiedRequirement && len(evaluation.Missing) == 0 {
		return fmt.Errorf("unverified requirement requires specific missing evidence")
	}
	if evaluation.Status == "ready" {
		if nilScores != 0 {
			return fmt.Errorf("ready evaluation has %d missing scores", nilScores)
		}
		if len(evaluation.Missing) != 0 {
			return fmt.Errorf("ready evaluation still lists missing evidence")
		}
	}

	hasBug := false
	for index, issue := range evaluation.Issues {
		if _, ok := allowedIssueKinds[issue.Kind]; !ok {
			return fmt.Errorf("evaluation issue %d kind %q is invalid", index+1, issue.Kind)
		}
		if strings.TrimSpace(issue.Description) == "" {
			return fmt.Errorf("evaluation issue %d description is required", index+1)
		}
		if strings.TrimSpace(issue.Evidence) == "" {
			return fmt.Errorf("evaluation issue %d evidence is required", index+1)
		}
		hasBug = hasBug || issue.Kind == "bug"
	}
	if hasFailedRequirement && !hasBug {
		return fmt.Errorf("failed requirement requires a corresponding bug issue")
	}
	if hasUnverifiedRequirement && !hasBug && evaluation.Scores[0] != nil {
		return fmt.Errorf("unverified requirement without a confirmed bug requires a null delivery score")
	}
	return ValidateRepairConsistency(evaluation)
}

// ValidateRepairConsistency also checks saved evaluations before export.
func ValidateRepairConsistency(e Evaluation) error {
	perfect := true
	for _, score := range e.Scores {
		if score == nil || *score != 5 {
			perfect = false
		}
	}
	if perfect && (strings.TrimSpace(e.NextPrompt) != "" || strings.TrimSpace(e.NextPromptType) != "") {
		return fmt.Errorf("五维满分不能包含修复提示词，请重新审核")
	}
	hasBug := false
	for _, issue := range e.Issues {
		if issue.Kind != "bug" {
			continue
		}
		hasBug = true
		if e.Scores[0] == nil || *e.Scores[0] >= 5 {
			return fmt.Errorf("存在已确认的功能遗漏或 Bug，交付完整性必须有依据地评为低于 5 分，请重新审核")
		}
		if !strings.HasPrefix(strings.TrimSpace(e.NextPrompt), "修复") || strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(e.NextPrompt), "修复")) == "" || e.NextPromptType != "Bug修复" {
			return fmt.Errorf("存在 Bug 时必须提供以“修复”开头、说明具体问题的 Bug修复提示词，请重新审核")
		}
	}
	if !hasBug && (strings.TrimSpace(e.NextPrompt) != "" || strings.TrimSpace(e.NextPromptType) != "") {
		return fmt.Errorf("修复提示词缺少已确认的代码问题依据，不能仅因评分未满分生成，请重新审核")
	}
	return nil
}

func containsNegatedClaim(description, claim string) bool {
	for _, prefix := range []string{"并非", "不是", "并不是", "不能说", "并无证据表明"} {
		if strings.Contains(description, prefix+claim) {
			return true
		}
	}
	return false
}

// Preflight verifies that every real, non-excluded turn in each completed task
// has a current, ready evaluation and that its frozen evidence is addressable.
func Preflight(cases []Case) Report {
	report := Report{Tasks: len(cases), Issues: make([]string, 0)}
	seenRounds := make(map[string]string)
	globalConflict := false
	for _, annotationCase := range cases {
		caseLabel := annotationCase.TaskID
		if caseLabel == "" {
			caseLabel = "<missing taskId>"
		}
		caseIssues := make([]string, 0)
		addIssue := func(format string, args ...any) {
			caseIssues = append(caseIssues, fmt.Sprintf("task %s: "+format, append([]any{caseLabel}, args...)...))
		}
		if annotationCase.TaskID == "" {
			addIssue("taskId is required")
		}
		if !annotationCase.Completed {
			addIssue("task is not completed")
		}
		if !fullSHA.MatchString(annotationCase.InitialSHA) {
			addIssue("initial snapshot SHA must be a full 40-character commit")
		}
		if !snapshotURLMatches(annotationCase.SnapshotURL, annotationCase.InitialSHA) {
			addIssue("snapshot URL does not reference the recorded full SHA")
		}
		captures := make(map[string]Capture, len(annotationCase.Captures))
		for _, capture := range annotationCase.Captures {
			captures[capture.ID] = capture
		}
		validRounds := 0
		readyRounds := 0
		for _, round := range annotationCase.Rounds {
			if round.Status == "excluded" {
				if strings.TrimSpace(round.Reason) == "" {
					addIssue("excluded round %q has no reason", round.PromptID)
				}
				continue
			}
			report.Rounds++
			validRounds++
			if round.Status != "complete" {
				addIssue("round %q has blocking status %q", round.PromptID, round.Status)
				continue
			}
			if round.PromptID == "" {
				addIssue("complete round is missing promptId")
				continue
			}
			identity := round.SessionID + "\x00" + round.PromptID
			if previousTask, exists := seenRounds[identity]; exists {
				report.Issues = append(report.Issues, fmt.Sprintf("duplicate SessionID + promptId appears in tasks %s and %s", previousTask, caseLabel))
				globalConflict = true
			} else {
				seenRounds[identity] = caseLabel
			}
			if round.CaptureID != "" {
				capture, ok := captures[round.CaptureID]
				if !ok {
					addIssue("round %q references missing capture %q", round.PromptID, round.CaptureID)
				} else {
					validateCaptureArtifacts(round, capture, addIssue)
				}
			}
			evaluation := latestMatchingEvaluation(round)
			if evaluation == nil {
				addIssue("round %q has no evaluation matching its evidence", round.PromptID)
				continue
			}
			if evaluation.Status != "ready" {
				addIssue("round %q latest matching evaluation is not ready", round.PromptID)
				continue
			}
			if err := ValidateEvaluation(round, *evaluation); err != nil {
				addIssue("round %q evaluation is invalid: %v", round.PromptID, err)
				continue
			}
			readyRounds++
		}
		if validRounds == 0 {
			addIssue("task has no non-excluded rounds")
		}
		if validRounds > 10 {
			addIssue("task has %d real rounds, exceeding the 10-round limit", validRounds)
		}
		if len(caseIssues) == 0 {
			report.Ready += readyRounds
		}
		report.Issues = append(report.Issues, caseIssues...)
	}
	if globalConflict {
		report.Ready = 0
	}
	return report
}

func snapshotURLMatches(rawURL, sha string) bool {
	if !fullSHA.MatchString(sha) {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 4 || parts[len(parts)-2] != "commit" {
		return false
	}
	return parts[len(parts)-1] == sha && fullSHA.MatchString(parts[len(parts)-1])
}

func latestMatchingEvaluation(round Round) *Evaluation {
	var latest *Evaluation
	latestIndex := -1
	for index := range round.Evaluations {
		evaluation := &round.Evaluations[index]
		if evaluation.EvidenceHash != round.EvidenceHash {
			continue
		}
		if latest == nil || evaluation.CreatedAt > latest.CreatedAt || (evaluation.CreatedAt == latest.CreatedAt && index > latestIndex) {
			latest = evaluation
			latestIndex = index
		}
	}
	return latest
}

func validateCaptureArtifacts(round Round, capture Capture, addIssue func(string, ...any)) {
	if capture.Dir == "" {
		addIssue("round %q capture %q has no capture directory", round.PromptID, capture.ID)
	} else if info, err := os.Stat(capture.Dir); err != nil || !info.IsDir() {
		addIssue("round %q capture directory is unavailable: %s", round.PromptID, capture.Dir)
	}
	if strings.TrimSpace(capture.Hash) == "" {
		addIssue("round %q capture %q has no code tree hash", round.PromptID, capture.ID)
	}
	if capture.TracePath == "" {
		addIssue("round %q capture %q has no trace path", round.PromptID, capture.ID)
	} else if info, err := os.Stat(capture.TracePath); err != nil || info.IsDir() {
		addIssue("round %q trace artifact is unavailable: %s", round.PromptID, capture.TracePath)
	}
	if capture.CodePath == "" {
		addIssue("round %q capture %q has no code path", round.PromptID, capture.ID)
	} else if info, err := os.Stat(capture.CodePath); err != nil || !info.IsDir() {
		addIssue("round %q code artifact is unavailable: %s", round.PromptID, capture.CodePath)
	}
}
