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
