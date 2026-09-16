package annotation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/blueship581/pinru/internal/annotation"
)

func TestCapturePairwiseSideStoresOneTurnWithoutReplacingOtherSide(t *testing.T) {
	s, trace, source := annotationFixture(t)
	c, err := s.EnablePairwise(EnablePairwiseRequest{TaskID: "题目-1", Harness: "Codex", HarnessVersion: "1.0.0", OS: "MacOS/Linux"})
	if err != nil {
		t.Fatal(err)
	}
	c.SnapshotURL = "https://github.com/example/repo/commit/" + c.InitialSHA
	if _, err := s.store.SaveAnnotationCase(*c, c.Revision); err != nil {
		t.Fatal(err)
	}
	s.pushPairwise = func(context.Context, string, string, string) error { return nil }

	if _, err := s.PreparePairwiseSide(context.Background(), PairwiseSideRequest{TaskID: c.TaskID, Side: domain.PairwiseSideA}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	traceA := filepath.Join(filepath.Dir(trace), "session-a.jsonl")
	if err := os.WriteFile(traceA, []byte(strings.ReplaceAll(string(raw), `"session"`, `"session-a"`)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "a.txt"), []byte("A result"), 0o600); err != nil {
		t.Fatal(err)
	}
	capturedA, err := s.CapturePairwiseSide(context.Background(), PairwiseCaptureRequest{TaskID: c.TaskID, Side: domain.PairwiseSideA, TracePath: traceA})
	if err != nil {
		t.Fatal(err)
	}
	aCapture := capturedA.Pairwise.RunA.CaptureID
	if aCapture == "" || capturedA.Pairwise.RunA.SessionID != "session-a" || capturedA.Pairwise.RunA.TurnCount != 1 {
		t.Fatalf("captured A = %#v", capturedA.Pairwise.RunA)
	}
	if _, err := s.CommitPairwiseSide(context.Background(), PairwiseCommitRequest{TaskID: c.TaskID, Side: domain.PairwiseSideA, SessionID: "session-a"}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.PreparePairwiseSide(context.Background(), PairwiseSideRequest{TaskID: c.TaskID, Side: domain.PairwiseSideB}); err != nil {
		t.Fatal(err)
	}
	traceB := filepath.Join(filepath.Dir(trace), "session-b.jsonl")
	if err := os.WriteFile(traceB, []byte(strings.ReplaceAll(string(raw), `"session"`, `"session-b"`)), 0o600); err != nil {
		t.Fatal(err)
	}
	capturedB, err := s.CapturePairwiseSide(context.Background(), PairwiseCaptureRequest{TaskID: c.TaskID, Side: domain.PairwiseSideB, TracePath: traceB})
	if err != nil {
		t.Fatal(err)
	}
	if capturedB.Pairwise.RunA.CaptureID != aCapture || capturedB.Pairwise.RunB.CaptureID == "" {
		t.Fatalf("captures after B = A:%#v B:%#v", capturedB.Pairwise.RunA, capturedB.Pairwise.RunB)
	}
}

func TestCapturePairwiseSideRejectsDuplicateSessionAndMultipleTurns(t *testing.T) {
	s, trace, source := annotationFixture(t)
	c, err := s.EnablePairwise(EnablePairwiseRequest{TaskID: "题目-1", Harness: "Codex", HarnessVersion: "1", OS: "MacOS/Linux"})
	if err != nil {
		t.Fatal(err)
	}
	c.Pairwise.RunA.SessionID = "session"
	if _, err := s.store.SaveAnnotationCase(*c, c.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CapturePairwiseSide(context.Background(), PairwiseCaptureRequest{TaskID: c.TaskID, Side: domain.PairwiseSideB, TracePath: trace}); err == nil || !strings.Contains(err.Error(), "SessionID") {
		t.Fatalf("duplicate session error = %v", err)
	}
	multi := filepath.Join(filepath.Dir(trace), "multi.jsonl")
	writeFixtureTrace(t, multi, source, 2)
	loaded, _ := s.loadCase(c.TaskID)
	loaded.Pairwise.RunA.SessionID = ""
	if _, err := s.store.SaveAnnotationCase(*loaded, loaded.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CapturePairwiseSide(context.Background(), PairwiseCaptureRequest{TaskID: c.TaskID, Side: domain.PairwiseSideA, TracePath: multi}); err == nil || !strings.Contains(err.Error(), "一轮") {
		t.Fatalf("multi-turn error = %v", err)
	}
}

func TestSavePairwiseMaterialsRequiresHTTPVideoURL(t *testing.T) {
	s, _, _ := annotationFixture(t)
	c, err := s.EnablePairwise(EnablePairwiseRequest{TaskID: "题目-1", Harness: "Codex", HarnessVersion: "1", OS: "MacOS/Linux"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SavePairwiseMaterials(PairwiseMaterialsRequest{TaskID: c.TaskID, Side: domain.PairwiseSideA, VideoURL: "file:///tmp/a.mp4"}); err == nil {
		t.Fatal("accepted local file URL")
	}
	updated, err := s.SavePairwiseMaterials(PairwiseMaterialsRequest{TaskID: c.TaskID, Side: domain.PairwiseSideA, VideoURL: "https://example.com/a.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Pairwise.RunA.VideoStatus != domain.PairwiseVideoReady || updated.Pairwise.RunA.VideoURL != "https://example.com/a.mp4" {
		t.Fatalf("video state = %#v", updated.Pairwise.RunA)
	}
}
