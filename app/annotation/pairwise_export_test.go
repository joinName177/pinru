package annotation

import (
	"archive/zip"
	"io"
	"strings"
	"testing"

	domain "github.com/blueship581/pinru/internal/annotation"
)

func pairwiseExportCase(t *testing.T, complete bool) (*AnnotationService, *domain.Case) {
	t.Helper()
	s, _, _ := annotationFixture(t)
	c, err := s.EnablePairwise(EnablePairwiseRequest{
		TaskID: "题目-1", Harness: "Codex", HarnessVersion: "1.2.3", OS: "MacOS/Linux", Environment: "本地仓库",
	})
	if err != nil {
		t.Fatal(err)
	}
	c.Pairwise.Prompt = "实现加法功能"
	c.SnapshotURL = "https://github.com/example/repo/commit/" + c.InitialSHA
	for side, run := range map[domain.PairwiseSide]*domain.PairwiseRun{
		domain.PairwiseSideA: &c.Pairwise.RunA,
		domain.PairwiseSideB: &c.Pairwise.RunB,
	} {
		label := strings.ToLower(string(side))
		run.SessionID = "session-" + label
		run.TracePath = "/evidence/" + label + "/session.jsonl"
		run.TurnCount = 1
		run.CaptureID = "capture-" + label
		run.CaptureHash = "code-hash-" + label
		run.TraceHash = "trace-hash-" + label
		run.DeliverableSHA = strings.Repeat(map[domain.PairwiseSide]string{domain.PairwiseSideA: "a", domain.PairwiseSideB: "b"}[side], 40)
		run.DeliverableURL = "https://github.com/example/repo/commit/" + run.DeliverableSHA
		if complete {
			run.VideoStatus = domain.PairwiseVideoReady
			run.VideoURL = "https://example.com/" + label + ".mp4"
		}
	}
	if complete {
		execution, err := s.reviewExecution()
		if err != nil {
			t.Fatal(err)
		}
		c.Pairwise.Reviews = append(c.Pairwise.Reviews, domain.PairwiseReview{
			ID: "review-1", Status: domain.PairwiseReviewReady, Conclusion: domain.PairwiseConclusionA,
			Reason:      "A 保留了加法返回值并覆盖正常输入；B 删除返回值，调用方无法取得计算结果，因此 A 更完整。",
			Model:       execution.Label,
			SourceHashA: domain.PairwiseRunSourceHash(c.Pairwise.RunA),
			SourceHashB: domain.PairwiseRunSourceHash(c.Pairwise.RunB),
		})
	}
	saved, err := s.store.SaveAnnotationCase(*c, c.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return s, saved
}

func TestPairwisePreflightReportsMissingSideMaterials(t *testing.T) {
	s, _ := pairwiseExportCase(t, false)
	report, err := s.PreflightPairwise("batch")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(report.Issues, "；")
	for _, want := range []string{"A 运行视频尚未就绪", "B 运行视频尚未就绪", "GSB"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("issues %q do not contain %q", joined, want)
		}
	}
	if report.Tasks != 1 || report.Ready != 0 {
		t.Fatalf("report = %#v", report)
	}
}

func TestPairwiseExportWritesOnePairPerRow(t *testing.T) {
	s, c := pairwiseExportCase(t, true)
	result, err := s.ExportPairwise(PairwiseExportRequest{
		ProjectID: "batch", TaskID: c.TaskID, Submitter: "标注员", SubmittedAt: "2026-09-17",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows != 1 || result.OutputPath == "" || result.ReportPath == "" {
		t.Fatalf("result = %#v", result)
	}
	z, err := zip.OpenReader(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var workbook strings.Builder
	for _, file := range z.File {
		if !strings.HasPrefix(file.Name, "xl/") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(&workbook, reader)
		_ = reader.Close()
	}
	for _, want := range []string{"session-a", "session-b", "https://example.com/a.mp4", "https://example.com/b.mp4"} {
		if !strings.Contains(workbook.String(), want) {
			t.Errorf("workbook does not contain %q", want)
		}
	}
}
