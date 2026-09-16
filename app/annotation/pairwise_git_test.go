package annotation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/blueship581/pinru/internal/annotation"
)

func TestPairwiseGitServicePreparesAndCommitsBothSidesFromInitial(t *testing.T) {
	s, _, source := annotationFixture(t)
	c, err := s.loadCase("题目-1")
	if err != nil {
		t.Fatal(err)
	}
	c.Mode = domain.CaseModePairwiseGSB
	c.Pairwise = domain.NewPairwiseData("实现加法功能")
	c.SnapshotURL = "https://github.com/example/repo/commit/" + c.InitialSHA
	s.pushPairwise = func(context.Context, string, string, string) error { return nil }
	if _, err := s.store.SaveAnnotationCase(*c, c.Revision); err != nil {
		t.Fatal(err)
	}

	preparedA, err := s.PreparePairwiseSide(context.Background(), PairwiseSideRequest{TaskID: c.TaskID, Side: domain.PairwiseSideA})
	if err != nil {
		t.Fatal(err)
	}
	if preparedA.Pairwise.RunA.Branch != "A" || preparedA.Pairwise.RunA.PreparedAt == 0 {
		t.Fatalf("prepared A = %#v", preparedA.Pairwise.RunA)
	}
	if err := os.WriteFile(filepath.Join(source, "a.txt"), []byte("A result"), 0o600); err != nil {
		t.Fatal(err)
	}
	committedA, err := s.CommitPairwiseSide(context.Background(), PairwiseCommitRequest{TaskID: c.TaskID, Side: domain.PairwiseSideA, SessionID: "session-a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(committedA.Pairwise.RunA.DeliverableSHA) != 40 || !strings.HasSuffix(committedA.Pairwise.RunA.DeliverableURL, committedA.Pairwise.RunA.DeliverableSHA) {
		t.Fatalf("committed A = %#v", committedA.Pairwise.RunA)
	}

	preparedB, err := s.PreparePairwiseSide(context.Background(), PairwiseSideRequest{TaskID: c.TaskID, Side: domain.PairwiseSideB})
	if err != nil {
		t.Fatal(err)
	}
	if preparedB.Pairwise.RunB.Branch != "B" || preparedB.Pairwise.RunB.PreparedAt == 0 {
		t.Fatalf("prepared B = %#v", preparedB.Pairwise.RunB)
	}
	if _, err := os.Stat(filepath.Join(source, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("B workspace retained A result: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "b.txt"), []byte("B result"), 0o600); err != nil {
		t.Fatal(err)
	}
	committedB, err := s.CommitPairwiseSide(context.Background(), PairwiseCommitRequest{TaskID: c.TaskID, Side: domain.PairwiseSideB, SessionID: "session-b"})
	if err != nil {
		t.Fatal(err)
	}
	if committedB.Pairwise.RunA.DeliverableSHA == "" || committedB.Pairwise.RunB.DeliverableSHA == "" {
		t.Fatalf("pairwise commits = %#v", committedB.Pairwise)
	}
	for _, side := range []string{"A", "B"} {
		got, err := runCommand(context.Background(), source, "git", "merge-base", "--is-ancestor", c.InitialSHA, side)
		if err != nil {
			t.Fatalf("%s does not descend from initial: %s %v", side, got, err)
		}
	}
}

func TestPairwiseGitServiceRejectsLegacyCaseAndWrongSide(t *testing.T) {
	s, _, _ := annotationFixture(t)
	if _, err := s.PreparePairwiseSide(context.Background(), PairwiseSideRequest{TaskID: "题目-1", Side: domain.PairwiseSideA}); err == nil {
		t.Fatal("legacy case accepted")
	}
	c, _ := s.loadCase("题目-1")
	c.Mode = domain.CaseModePairwiseGSB
	c.Pairwise = domain.NewPairwiseData("实现加法功能")
	if _, err := s.store.SaveAnnotationCase(*c, c.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PreparePairwiseSide(context.Background(), PairwiseSideRequest{TaskID: c.TaskID, Side: "C"}); err == nil {
		t.Fatal("invalid side accepted")
	}
}
