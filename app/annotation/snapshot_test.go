package annotation

import (
	"context"
	"github.com/blueship581/pinru/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishInitialUsesFrozenBaselineAndIsIdempotent(t *testing.T) {
	s, _, source := annotationFixture(t)
	if err := s.store.CreateGitHubAccount(store.GitHubAccount{ID: "account", Username: "owner", Token: "fixture", IsDefault: true}); err != nil {
		t.Fatal(err)
	}
	c, _ := s.loadCase("题目-1")
	os.WriteFile(filepath.Join(source, "later.txt"), []byte("uncommitted later output"), 0600)
	calls := 0
	s.publishInitial = func(ctx context.Context, baseline, repo, sha string, account store.GitHubAccount) (string, error) {
		calls++
		if baseline == source || sha != c.InitialSHA {
			t.Fatal("not using frozen initial state")
		}
		if _, err := os.Stat(filepath.Join(baseline, "later.txt")); !os.IsNotExist(err) {
			t.Fatal("later code leaked into initial snapshot")
		}
		return "https://github.com/owner/" + repo + "/commit/" + sha, nil
	}
	for i := 0; i < 2; i++ {
		updated, err := s.PublishSnapshot(context.Background(), PrepareRequest{TaskID: c.TaskID})
		if err != nil || !strings.HasSuffix(updated.SnapshotURL, c.InitialSHA) {
			t.Fatalf("publish: %v %v", updated, err)
		}
	}
	if calls != 1 {
		t.Fatalf("published %d times", calls)
	}
}

func TestSnapshotPublicationPushesInitialCommitWithoutExistingOrigin(t *testing.T) {
	s, _, source := annotationFixture(t)
	c, err := s.loadCase("题目-1")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	baseline := filepath.Join(s.caseDir(c.TaskID), "initial", c.InitialSHA)
	remote := filepath.Join(t.TempDir(), "remote.git")
	if _, err := runCommand(ctx, "", "git", "init", "--bare", remote); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "later.txt"), []byte("later output"), 0600); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(t.TempDir(), "publish")
	if err := prepareSnapshotPublication(ctx, baseline, work, c.InitialSHA, remote); err != nil {
		t.Fatal(err)
	}
	if _, err := runCommand(ctx, work, "git", "push", "origin", "main:main"); err != nil {
		t.Fatal(err)
	}
	head, err := runCommand(ctx, remote, "git", "rev-parse", "refs/heads/main")
	if err != nil || strings.TrimSpace(string(head)) != c.InitialSHA {
		t.Fatalf("pushed %s, want %s: %v", head, c.InitialSHA, err)
	}
	if _, err := runCommand(ctx, remote, "git", "cat-file", "-e", "main:later.txt"); err == nil {
		t.Fatal("published later output")
	}
}
