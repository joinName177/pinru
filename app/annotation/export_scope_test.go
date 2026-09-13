package annotation

import (
	domain "github.com/blueship581/pinru/internal/annotation"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportSelectionKeepsReviewedRoundsAcrossTasks(t *testing.T) {
	makeRound := func(id, hash string, evaluated bool) domain.Round {
		r := domain.Round{PromptID: id, EvidenceHash: hash, Status: "complete"}
		if evaluated {
			r.Evaluations = []domain.Evaluation{{ID: id, Status: "ready", EvidenceHash: hash}}
		}
		return r
	}
	cases := []domain.Case{{TaskID: "a", Rounds: []domain.Round{makeRound("a1", "h1", true), makeRound("a2", "h2", false)}}, {TaskID: "b", Rounds: []domain.Round{makeRound("b1", "h3", true)}}}
	got, err := selectExportCases(cases, ExportRequest{ReviewedOnly: true})
	if err != nil || len(got) != 2 || len(got[0].Rounds) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	if len(cases[0].Rounds) != 2 {
		t.Fatal("changed original case")
	}
	single, err := selectExportCases(cases, ExportRequest{TaskID: "b", ReviewedOnly: true})
	if err != nil || len(single) != 1 || single[0].TaskID != "b" {
		t.Fatalf("%+v %v", single, err)
	}
	if _, err := selectExportCases(cases, ExportRequest{TaskID: "other", ReviewedOnly: true}); err == nil {
		t.Fatal("accepted unrelated task")
	}
	cases[0].Rounds[0].EvidenceHash = "changed"
	if _, err := selectExportCases(cases[:1], ExportRequest{ReviewedOnly: true}); err == nil {
		t.Fatal("accepted stale evaluation")
	}
}

func TestReviewedExportRejectsLatestMissingOrStaleEvenWithOlderReady(t *testing.T) {
	old := domain.Evaluation{Status: "ready", EvidenceHash: "h", CreatedAt: 1}
	stale := false
	for _, latest := range []domain.Evaluation{
		{Status: "needs_evidence", EvidenceHash: "h", CreatedAt: 2},
		{Status: "ready", EvidenceHash: "h", CreatedAt: 2, Current: &stale},
	} {
		c := domain.Case{TaskID: "a", Rounds: []domain.Round{{Status: "complete", EvidenceHash: "h", Evaluations: []domain.Evaluation{old, latest}}}}
		if _, err := selectExportCases([]domain.Case{c}, ExportRequest{ReviewedOnly: true}); err == nil {
			t.Fatal("export accepted stale or incomplete latest review")
		}
	}
}

func TestExportDirectoryUsesGlobalConfig(t *testing.T) {
	s, _, _ := annotationFixture(t)
	configured := filepath.Join(t.TempDir(), "shared exports")
	if err := s.store.SetConfig("annotation_export_directory", configured); err != nil {
		t.Fatal(err)
	}
	got, err := s.exportDirectory()
	if err != nil || got != configured {
		t.Fatalf("%s %v", got, err)
	}
	s.store.SetConfig("annotation_export_directory", "relative/path")
	if _, err := s.exportDirectory(); err == nil {
		t.Fatal("accepted relative output directory")
	}
}

func TestCaseAlwaysReadsCurrentCardTypeInsteadOfSavedReviewType(t *testing.T) {
	s, trace, _ := annotationFixture(t)
	s.cli, _ = fakeReviewCLI(t)
	if _, err := s.Capture(CaptureRequest{TaskID: "题目-1", TracePath: trace}); err != nil {
		t.Fatal(err)
	}
	c, err := s.Review(ReviewRequest{TaskID: "题目-1", PromptID: "p1"})
	if err != nil {
		t.Fatal(err)
	}
	if c.TaskType != "0-1代码生成" || c.Rounds[0].Evaluations[0].TaskType != "0-1代码生成" {
		t.Fatalf("card type lost: %+v", c)
	}
	if err := s.store.UpdateTaskType("题目-1", "Feature迭代"); err != nil {
		t.Fatal(err)
	}
	c, err = s.loadCase("题目-1")
	if err != nil {
		t.Fatal(err)
	}
	if c.TaskType != "Feature迭代" {
		t.Fatalf("stale case type: %s", c.TaskType)
	}
	if c.Rounds[0].Evaluations[0].TaskType != "0-1代码生成" {
		t.Fatal("reading card metadata rewrote historical review")
	}
}

func TestExportExplicitSubsetRequiresAvailableReviewedTasks(t *testing.T) {
	r := domain.Round{Status: "complete", EvidenceHash: "h", Evaluations: []domain.Evaluation{{Status: "ready", EvidenceHash: "h"}}}
	cases := []domain.Case{{TaskID: "a", Rounds: []domain.Round{r}}, {TaskID: "b", Rounds: []domain.Round{r}}, {TaskID: "c", Rounds: []domain.Round{r}}, {TaskID: "empty"}}
	got, err := selectExportCases(cases, ExportRequest{TaskIDs: []string{"c", "a", "a"}, ReviewedOnly: true})
	if err != nil || len(got) != 2 || got[0].TaskID != "a" || got[1].TaskID != "c" {
		t.Fatalf("wrong subset: %+v %v", got, err)
	}
	for _, req := range []ExportRequest{
		{TaskIDs: []string{}, ReviewedOnly: true},
		{TaskIDs: []string{" "}, ReviewedOnly: true},
		{TaskIDs: []string{"other"}, ReviewedOnly: true},
		{TaskIDs: []string{"a", "other"}, ReviewedOnly: true},
		{TaskIDs: []string{"a", "empty"}, ReviewedOnly: true},
		{TaskIDs: []string{"a"}, TaskID: "b", ReviewedOnly: true},
		{TaskIDs: []string{"a"}},
	} {
		if _, err := selectExportCases(cases, req); err == nil {
			t.Fatalf("accepted invalid selection: %+v", req)
		}
	}
	cases[0].Rounds = []domain.Round{{Status: "complete", EvidenceHash: "new", Evaluations: r.Evaluations}}
	if _, err := selectExportCases(cases, ExportRequest{TaskIDs: []string{"a"}, ReviewedOnly: true}); err == nil {
		t.Fatal("accepted stale selected data")
	}
}

func TestReviewedExportWritesOnlySelectedTaskToConfiguredDirectory(t *testing.T) {
	s, trace, _ := annotationFixture(t)
	s.cli, _ = fakeReviewCLI(t)
	if _, err := s.Capture(CaptureRequest{TaskID: "题目-1", TracePath: trace}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Review(ReviewRequest{TaskID: "题目-1", PromptID: "p1"}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "unified")
	s.store.SetConfig("annotation_export_directory", root)
	result, err := s.Export(ExportRequest{ProjectID: "batch", TaskID: "题目-1", ReviewedOnly: true, Draft: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows != 1 {
		t.Fatalf("rows %d", result.Rows)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(resolvedRoot, result.OutputPath)
	if err != nil || filepath.IsAbs(relative) || strings.HasPrefix(relative, "..") {
		t.Fatalf("wrong output %s", result.OutputPath)
	}
}
