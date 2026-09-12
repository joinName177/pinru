package annotation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	domain "github.com/blueship581/pinru/internal/annotation"
	"github.com/blueship581/pinru/internal/github"
	"github.com/blueship581/pinru/internal/gitops"
	"github.com/blueship581/pinru/internal/store"
)

// PublishSnapshot only publishes the frozen initial commit, never the live workspace.
func (s *AnnotationService) PublishSnapshot(ctx context.Context, req PrepareRequest) (*domain.Case, error) {
	unlock, err := s.lockTask(req.TaskID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	c, err := s.loadCase(req.TaskID)
	if err != nil {
		return nil, err
	}
	if c.SnapshotURL != "" {
		return c, nil
	}
	if len(c.InitialSHA) != 40 {
		return nil, errors.New("缺少真实初始快照，请先准备题目")
	}
	accounts, err := s.store.ListGitHubAccounts()
	if err != nil {
		return nil, err
	}
	var account *store.GitHubAccount
	for i := range accounts {
		if accounts[i].IsDefault {
			account = &accounts[i]
			break
		}
	}
	if account == nil && len(accounts) == 1 {
		account = &accounts[0]
	}
	if account == nil {
		return nil, errors.New("请在设置中配置默认 GitHub 账号后重试发布初始快照")
	}
	baseline := filepath.Join(s.caseDir(c.TaskID), "initial", c.InitialSHA)
	url, err := s.publishInitial(ctx, baseline, "pinru-initial-"+stableKey(c.TaskID), c.InitialSHA, *account)
	if err != nil {
		return nil, err
	}
	if sha, err := snapshotSHA(url); err != nil || sha != c.InitialSHA {
		return nil, errors.New("发布结果与初始 SHA 不一致")
	}
	c.SnapshotURL = url
	return s.store.SaveAnnotationCase(*c, c.Revision)
}

func publishInitial(ctx context.Context, baseline, repoName, sha string, account store.GitHubAccount) (string, error) {
	if strings.TrimSpace(account.Token) == "" || strings.TrimSpace(account.Username) == "" {
		return "", errors.New("GitHub 账号配置不完整")
	}
	parent, err := os.MkdirTemp("", "pinru-initial-publish-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(parent)
	work := filepath.Join(parent, "repo")
	if err := ctx.Err(); err != nil {
		return "", err
	}
	target := account.Username + "/" + repoName
	if err := prepareSnapshotPublication(ctx, baseline, work, sha, "https://github.com/"+target+".git"); err != nil {
		return "", err
	}
	repo, err := github.EnsureRepository(target, account.Token, nil)
	if err != nil {
		return "", fmt.Errorf("创建初始快照仓库失败：%w", err)
	}
	if err := gitops.PushBranchWithMode(work, "main", account.Username, account.Token, false); err != nil {
		return "", err
	}
	return strings.TrimRight(repo.HTMLURL, "/") + "/commit/" + sha, nil
}

// Configure only the disposable publishing clone; frozen evidence stays unchanged.
func prepareSnapshotPublication(ctx context.Context, baseline, work, sha, remote string) error {
	if err := cloneInitialRepository(ctx, baseline, work, sha); err != nil {
		return err
	}
	if err := gitops.EnsureBranch(work, "main"); err != nil {
		return err
	}
	_, err := runCommand(ctx, work, "git", "remote", "add", "origin", remote)
	return err
}
