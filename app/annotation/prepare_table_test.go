package annotation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "github.com/blueship581/pinru/internal/annotation"
)

func TestBatchCaptureAndTableStartsIndependentReviewsTogether(t *testing.T) {
	s, trace, _ := annotationFixture(t)
	if _, err := s.Capture(CaptureRequest{TaskID: "题目-1", TracePath: trace}); err != nil {
		t.Fatal(err)
	}

	sourceTwo := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceTwo, "main.py"), []byte("def add(a,b): return a+b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	taskTwo, err := s.store.GetTask("题目-1")
	if err != nil {
		t.Fatal(err)
	}
	taskTwo.ID = "题目-2"
	taskTwo.ProjectName = "示例题二"
	taskTwo.LocalPath = &sourceTwo
	if err := s.store.CreateTask(*taskTwo); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareCase(PrepareRequest{TaskID: taskTwo.ID}); err != nil {
		t.Fatal(err)
	}
	traceTwo := filepath.Join(t.TempDir(), "session.jsonl")
	writeFixtureTrace(t, traceTwo, sourceTwo, 1)
	if _, err := s.Capture(CaptureRequest{TaskID: taskTwo.ID, TracePath: traceTwo}); err != nil {
		t.Fatal(err)
	}

	s.cli, _ = fakeReviewCLIWithDelay(t, "1")
	payload, _ := json.Marshal(map[string]string{"projectId": "batch"})
	started := time.Now()
	output, err := s.ExecuteJob(context.Background(), "annotation_batch_capture_table", string(payload))
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	result := output.(*BatchPrepareResult)
	if result.Prepared != 2 || result.Failed != 0 {
		t.Fatalf("batch = %+v", result)
	}
	if elapsed >= 1800*time.Millisecond {
		t.Fatalf("reviews ran one after another: elapsed %s", elapsed)
	}
}

func TestBatchCaptureAndTablePreparesThenSkipsValidSavedData(t *testing.T) {
	s, trace, _ := annotationFixture(t)
	if _, err := s.Capture(CaptureRequest{TaskID: "题目-1", TracePath: trace}); err != nil {
		t.Fatal(err)
	}
	cli, count := fakeReviewCLI(t)
	s.cli = cli
	payload, _ := json.Marshal(map[string]string{"projectId": "batch"})

	output, err := s.ExecuteJob(context.Background(), "annotation_batch_capture_table", string(payload))
	if err != nil {
		t.Fatal(err)
	}
	var first struct {
		Total    int `json:"total"`
		Prepared int `json:"prepared"`
		Failed   int `json:"failed"`
	}
	raw, _ := json.Marshal(output)
	if err := json.Unmarshal(raw, &first); err != nil {
		t.Fatal(err)
	}
	if first.Total != 1 || first.Prepared != 1 || first.Failed != 0 {
		t.Fatalf("first batch = %+v", first)
	}

	// A functionally complete low-score review without a repair prompt is final
	// table data and must not spend another evaluator call in the next batch.
	c, err := s.store.GetAnnotationCase("题目-1")
	if err != nil {
		t.Fatal(err)
	}
	c.Rounds[0].Evaluations[0].NextPrompt = ""
	c.Rounds[0].Evaluations[0].NextPromptType = ""
	c.Rounds[0].Evaluations[0].Issues = nil
	if _, err := s.store.SaveAnnotationCase(*c, c.Revision); err != nil {
		t.Fatal(err)
	}

	output, err = s.ExecuteJob(context.Background(), "annotation_batch_capture_table", string(payload))
	if err != nil {
		t.Fatal(err)
	}
	var second struct {
		Skipped int `json:"skipped"`
	}
	raw, _ = json.Marshal(output)
	if err := json.Unmarshal(raw, &second); err != nil {
		t.Fatal(err)
	}
	if second.Skipped != 1 {
		t.Fatalf("second batch = %+v", second)
	}
	runs, _ := os.ReadFile(count)
	if string(runs) != "called\n" {
		t.Fatalf("batch reran valid saved review: %q", runs)
	}
}

func TestCaptureAndTableCachesGradesWithoutRequiringAnotherRound(t *testing.T) {
	s, trace, source := annotationFixture(t)
	cli, count := fakeReviewCLI(t)
	s.cli = cli
	payload, _ := json.Marshal(CaptureRequest{TaskID: "题目-1", TracePath: trace})
	for attempt := 0; attempt < 2; attempt++ {
		output, err := s.ExecuteJob(context.Background(), "annotation_capture_table", string(payload))
		if err != nil {
			t.Fatal(err)
		}
		c := output.(*domain.Case)
		if len(c.Rounds) != 1 || len(c.Rounds[0].Evaluations) != 1 || *c.Rounds[0].Evaluations[0].Scores[0] != 4 {
			t.Fatalf("did not retain single-round low-score table: %+v", c)
		}
	}
	runs, _ := os.ReadFile(count)
	if string(runs) != "called\n" {
		t.Fatalf("reran cached review: %s", runs)
	}
	// A later real round is analyzed separately and preserves the first review.
	writeFixtureTrace(t, trace, source, 2)
	output, err := s.ExecuteJob(context.Background(), "annotation_capture_table", string(payload))
	if err != nil {
		t.Fatal(err)
	}
	c := output.(*domain.Case)
	if len(c.Rounds) != 2 || len(c.Rounds[0].Evaluations) != 1 || len(c.Rounds[1].Evaluations) != 1 {
		t.Fatalf("rounds %+v", c.Rounds)
	}
	runs, _ = os.ReadFile(count)
	if string(runs) != "called\ncalled\n" {
		t.Fatalf("unexpected reviews: %s", runs)
	}
}

func TestCaptureAndTableKeepsCaptureOnReviewFailure(t *testing.T) {
	s, trace, _ := annotationFixture(t)
	payload, _ := json.Marshal(CaptureRequest{TaskID: "题目-1", TracePath: trace})
	_, err := s.ExecuteJob(context.Background(), "annotation_capture_table", string(payload))
	if err == nil || !strings.Contains(err.Error(), "采集已保存") {
		t.Fatalf("unclear error: %v", err)
	}
	c, err := s.store.GetAnnotationCase("题目-1")
	if err != nil || len(c.Rounds) != 1 || len(c.Captures) != 1 {
		t.Fatalf("lost capture: %+v %v", c, err)
	}
}

func TestResumeUsesFrozenEvidenceAndKeepsCompletedRounds(t *testing.T) {
	s, trace, source := annotationFixture(t)
	s.cli, _ = fakeReviewCLI(t)
	if _, err := s.captureAndPrepareTable(context.Background(), CaptureRequest{TaskID: "题目-1", TracePath: trace}); err != nil {
		t.Fatal(err)
	}
	writeFixtureTrace(t, trace, source, 2)
	if _, err := s.Capture(CaptureRequest{TaskID: "题目-1", TracePath: trace}); err != nil {
		t.Fatal(err)
	}
	s.cli = nil
	if _, err := s.resumeTable(context.Background(), "题目-1"); err == nil {
		t.Fatal("expected missing evaluator failure")
	}
	cli, count := fakeReviewCLI(t)
	s.cli = cli
	// The original source can disappear after collection; retries use only saved evidence.
	if err := os.Rename(trace, filepath.Join(filepath.Dir(trace), "moved.jsonl")); err != nil {
		t.Fatal(err)
	}
	c, err := s.resumeTable(context.Background(), "题目-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Rounds) != 2 || len(c.Rounds[0].Evaluations) != 1 || len(c.Rounds[1].Evaluations) != 1 {
		t.Fatalf("lost rounds: %+v", c.Rounds)
	}
	data, _ := os.ReadFile(count)
	if string(data) != "called\n" {
		t.Fatalf("reran completed round: %q", data)
	}
}
