package annotation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/blueship581/pinru/internal/annotation"
)

func TestReviewPairwiseStoresReviewBoundToBothEvidenceSources(t *testing.T) {
	s, _, source := annotationFixture(t)
	payload := map[string]any{
		"status": "ready", "conclusion": "A_better",
		"reason": "A 在 code/main.py 中保留 add(a,b) 并通过静态核验；B 的 code/main.py 删除了返回值，调用方无法得到加法结果，因此 A 更完整。",
	}
	cli, _ := fakeReviewCLIWithEvaluation(t, payload, "")
	s.cli = cli
	c, err := s.EnablePairwise(EnablePairwiseRequest{TaskID: "题目-1", Harness: "Codex", HarnessVersion: "1", OS: "MacOS/Linux"})
	if err != nil {
		t.Fatal(err)
	}
	c.Pairwise.Prompt = "实现加法功能"
	c.SnapshotURL = "https://github.com/example/repo/commit/" + c.InitialSHA
	for _, side := range []domain.PairwiseSide{domain.PairwiseSideA, domain.PairwiseSideB} {
		captureID := "capture-" + strings.ToLower(string(side))
		dir := filepath.Join(s.caseDir(c.TaskID), "pairwise-review-fixture", captureID)
		code := filepath.Join(dir, "code")
		traces := filepath.Join(dir, "traces")
		if err := os.MkdirAll(code, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(traces, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(code, "main.py"), []byte("def add(a,b): return a+b\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(traces, "session.jsonl"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		codeHash, _ := domain.TreeHash(context.Background(), code)
		traceHash, _ := domain.TreeHash(context.Background(), traces)
		capture := domain.Capture{ID: captureID, Dir: dir, CodePath: code, TracePath: filepath.Join(traces, "session.jsonl"), Hash: codeHash, TraceHash: traceHash}
		c.Captures = append(c.Captures, capture)
		run, _ := pairwiseRun(c.Pairwise, side)
		run.SessionID = "session-" + strings.ToLower(string(side))
		run.TurnCount = 1
		run.CaptureID = captureID
		run.CaptureHash = codeHash
		run.TraceHash = traceHash
		run.DeliverableSHA = strings.Repeat(map[domain.PairwiseSide]string{domain.PairwiseSideA: "b", domain.PairwiseSideB: "c"}[side], 40)
		run.DeliverableURL = "https://github.com/example/repo/commit/" + run.DeliverableSHA
	}
	if _, err := s.store.SaveAnnotationCase(*c, c.Revision); err != nil {
		t.Fatal(err)
	}
	result, err := s.ReviewPairwise(context.Background(), PairwiseReviewRequest{TaskID: c.TaskID})
	if err != nil {
		t.Fatal(err)
	}
	review := domain.CurrentPairwiseReview(*result)
	if review == nil || review.Current == nil || !*review.Current || review.Conclusion != domain.PairwiseConclusionA || review.SourceHashA == review.SourceHashB {
		t.Fatalf("review = %#v", review)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal(err)
	}
}
