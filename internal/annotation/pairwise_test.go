package annotation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPairwiseCaseJSONRoundTripAndLegacyDefault(t *testing.T) {
	c := Case{
		TaskID:     "task-pair",
		Mode:       CaseModePairwiseGSB,
		InitialSHA: strings.Repeat("a", 40),
		Pairwise: &PairwiseData{
			Prompt:  "实现筛选功能",
			RunA:    PairwiseRun{Side: PairwiseSideA, Branch: "A", SessionID: "session-a"},
			RunB:    PairwiseRun{Side: PairwiseSideB, Branch: "B", SessionID: "session-b"},
			Reviews: []PairwiseReview{},
		},
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Case
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	NormalizeCase(&decoded)
	if decoded.Mode != CaseModePairwiseGSB || decoded.Pairwise == nil || decoded.Pairwise.RunA.Branch != "A" {
		t.Fatalf("decoded pairwise case = %#v", decoded)
	}

	legacy := Case{TaskID: "legacy"}
	NormalizeCase(&legacy)
	if legacy.Mode != CaseModeLegacy {
		t.Fatalf("legacy mode = %q, want %q", legacy.Mode, CaseModeLegacy)
	}
}

func TestValidatePairwiseCaseRequiresDistinctSingleTurnSides(t *testing.T) {
	c := completePairwiseCase()
	c.Pairwise.RunB.SessionID = c.Pairwise.RunA.SessionID
	c.Pairwise.RunB.TurnCount = 2
	issues := ValidatePairwiseCase(c, false)
	assertPairwiseIssueContains(t, issues, "SessionID")
	assertPairwiseIssueContains(t, issues, "一轮")
}

func TestValidatePairwiseCaseRequiresPortalFields(t *testing.T) {
	c := completePairwiseCase()
	c.Pairwise.Language = ""
	c.Pairwise.Validity = ""
	c.PromptDifficulty = "中等"
	issues := ValidatePairwiseCase(c, true)
	assertPairwiseIssueContains(t, issues, "语言/框架")
	assertPairwiseIssueContains(t, issues, "有效性")
	assertPairwiseIssueContains(t, issues, "困难或地狱")
}

func TestValidatePairwiseFormalRequiresCurrentReviewButAllowsMissingVideos(t *testing.T) {
	c := completePairwiseCase()
	c.Pairwise.RunA.VideoStatus = PairwiseVideoMissing
	c.Pairwise.RunA.VideoURL = ""
	c.Pairwise.Reviews = nil

	draftIssues := ValidatePairwiseCase(c, false)
	if containsPairwiseIssue(draftIssues, "视频") || containsPairwiseIssue(draftIssues, "GSB") {
		t.Fatalf("draft issues = %#v, should not require final materials", draftIssues)
	}
	formalIssues := ValidatePairwiseCase(c, true)
	if containsPairwiseIssue(formalIssues, "视频") {
		t.Fatalf("formal issues = %#v, should allow missing videos", formalIssues)
	}
	assertPairwiseIssueContains(t, formalIssues, "GSB")
}

func TestCurrentPairwiseReviewRequiresMatchingSourceHashes(t *testing.T) {
	c := completePairwiseCase()
	if review := CurrentPairwiseReview(c); review == nil {
		t.Fatal("expected current review")
	}
	c.Pairwise.RunB.CaptureHash = "changed"
	if review := CurrentPairwiseReview(c); review != nil {
		t.Fatalf("stale review = %#v, want nil", review)
	}
}

func TestCurrentPairwiseReviewIgnoresVideoChanges(t *testing.T) {
	c := completePairwiseCase()
	c.Pairwise.RunA.VideoStatus = PairwiseVideoManualRequired
	c.Pairwise.RunA.VideoURL = ""
	c.Pairwise.RunA.VideoPath = ""
	c.Pairwise.RunB.VideoURL = "https://example.com/replaced-b.mp4"

	if review := CurrentPairwiseReview(c); review == nil {
		t.Fatal("video-only changes should not invalidate the current review")
	}
}

func TestCurrentPairwiseReviewAcceptsLegacyVideoAwareHashes(t *testing.T) {
	c := completePairwiseCase()
	c.Pairwise.Reviews[0].SourceHashA = legacyPairwiseRunSourceHash(c.Pairwise.RunA)
	c.Pairwise.Reviews[0].SourceHashB = legacyPairwiseRunSourceHash(c.Pairwise.RunB)

	if review := CurrentPairwiseReview(c); review == nil {
		t.Fatal("legacy review hashes should remain current after upgrade")
	}
}

func TestPairwiseReasonDecorativeBracketDetectionAndStripping(t *testing.T) {
	for _, symbol := range []string{"『", "』", "「", "」", "【", "】", "《", "》", "〔", "〕", "〖", "〗", "〘", "〙", "〚", "〛", "〈", "〉", "｢", "｣"} {
		reason := "A 修好" + symbol + "筛选" + symbol + "，B 未改动，本题最看重结果一致，判同级。"
		if !PairwiseReasonHasDecorativeBrackets(reason) {
			t.Fatalf("decorative bracket %q was not detected", symbol)
		}
		stripped := StripPairwiseReasonDecorations(reason)
		if PairwiseReasonHasDecorativeBrackets(stripped) {
			t.Fatalf("decorative bracket %q survived stripping: %q", symbol, stripped)
		}
		if stripped != "A 修好筛选，B 未改动，本题最看重结果一致，判同级。" {
			t.Fatalf("stripping %q changed more than the brackets: %q", symbol, stripped)
		}
	}

	clean := "A 修好筛选，B 未改动，本题最看重结果一致，判同级。"
	if PairwiseReasonHasDecorativeBrackets(clean) {
		t.Fatal("clean reason was flagged as containing decorative brackets")
	}
	if StripPairwiseReasonDecorations(clean) != clean {
		t.Fatal("stripping rewrote a reason without decorative brackets")
	}
}

func completePairwiseCase() Case {
	sha := strings.Repeat("a", 40)
	c := Case{
		TaskID:           "task-pair",
		TaskName:         "Pair",
		PromptDifficulty: "困难",
		Mode:             CaseModePairwiseGSB,
		InitialSHA:       sha,
		SnapshotURL:      "https://github.com/example/repo/commit/" + sha,
		Pairwise: &PairwiseData{
			Prompt:         "实现筛选功能",
			Language:       "Go",
			Harness:        "Codex CLI",
			HarnessVersion: "1.0.0",
			OS:             "MacOS/Linux",
			Validity:       PairwiseValidityValid,
			RunA: PairwiseRun{
				Side: PairwiseSideA, Branch: "A", ContainerID: "container-a", WorkspacePath: "/workspace-a", RepoRelativePath: "repo", SessionID: "session-a", TurnCount: 1,
				CaptureID: "capture-a", CaptureHash: "capture-hash-a", TraceHash: "trace-hash-a",
				DeliverableSHA: strings.Repeat("b", 40), DeliverableURL: "https://github.com/example/repo/commit/" + strings.Repeat("b", 40),
				VideoStatus: PairwiseVideoReady, VideoURL: "https://example.com/a.mp4",
			},
			RunB: PairwiseRun{
				Side: PairwiseSideB, Branch: "B", ContainerID: "container-b", WorkspacePath: "/workspace-b", RepoRelativePath: "repo", SessionID: "session-b", TurnCount: 1,
				CaptureID: "capture-b", CaptureHash: "capture-hash-b", TraceHash: "trace-hash-b",
				DeliverableSHA: strings.Repeat("c", 40), DeliverableURL: "https://github.com/example/repo/commit/" + strings.Repeat("c", 40),
				VideoStatus: PairwiseVideoReady, VideoURL: "https://example.com/b.mp4",
			},
		},
	}
	c.Pairwise.Reviews = []PairwiseReview{{
		ID: "review-1", Status: PairwiseReviewReady, Conclusion: PairwiseConclusionA,
		Reason:      "A 在 internal/filter.go 完成筛选并通过测试；B 只修改了界面，缺少服务端过滤。",
		SourceHashA: PairwiseRunSourceHash(c.Pairwise.RunA),
		SourceHashB: PairwiseRunSourceHash(c.Pairwise.RunB),
	}}
	return c
}

func assertPairwiseIssueContains(t *testing.T, issues []string, part string) {
	t.Helper()
	if !containsPairwiseIssue(issues, part) {
		t.Fatalf("issues = %#v, want substring %q", issues, part)
	}
}

func containsPairwiseIssue(issues []string, part string) bool {
	for _, issue := range issues {
		if strings.Contains(issue, part) {
			return true
		}
	}
	return false
}
