package annotation

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	domain "github.com/blueship581/pinru/internal/annotation"
	"github.com/google/uuid"
)

type EnablePairwiseRequest struct {
	TaskID         string `json:"taskId"`
	Harness        string `json:"harness"`
	HarnessVersion string `json:"harnessVersion"`
	OS             string `json:"os"`
	Environment    string `json:"environment"`
}

type PairwiseCaptureRequest struct {
	TaskID    string              `json:"taskId"`
	Side      domain.PairwiseSide `json:"side"`
	TracePath string              `json:"tracePath"`
}

type PairwiseMaterialsRequest struct {
	TaskID         string              `json:"taskId"`
	Side           domain.PairwiseSide `json:"side"`
	VideoURL       string              `json:"videoUrl"`
	RecordingError string              `json:"recordingError"`
}

func (s *AnnotationService) EnablePairwise(req EnablePairwiseRequest) (*domain.Case, error) {
	unlock, err := s.lockTask(req.TaskID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	c, err := s.loadCase(req.TaskID)
	if err != nil {
		return nil, err
	}
	if c.Mode == domain.CaseModeLegacy && (len(c.Rounds) > 0 || len(c.Captures) > 0 || c.SessionID != "") {
		return nil, errors.New("当前题目已有旧版标注证据，不能直接切换 Pair-wise 模式")
	}
	if c.TaskType == "代码理解" {
		return nil, errors.New("Pair-wise 模式暂不支持代码理解题")
	}
	task, err := s.store.GetTask(c.TaskID)
	if err != nil {
		return nil, err
	}
	prompt := ""
	if task != nil && task.PromptText != nil {
		prompt = *task.PromptText
	}
	if c.Pairwise == nil {
		c.Pairwise = domain.NewPairwiseData(prompt)
	}
	c.Mode = domain.CaseModePairwiseGSB
	c.Pairwise.Prompt = prompt
	c.Pairwise.Harness = strings.TrimSpace(req.Harness)
	c.Pairwise.HarnessVersion = strings.TrimSpace(req.HarnessVersion)
	c.Pairwise.OS = strings.TrimSpace(req.OS)
	c.Pairwise.Environment = strings.TrimSpace(req.Environment)
	return s.store.SaveAnnotationCase(*c, c.Revision)
}

func (s *AnnotationService) CapturePairwiseSide(ctx context.Context, req PairwiseCaptureRequest) (*domain.Case, error) {
	unlock, err := s.lockTask(req.TaskID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	c, err := s.loadCase(req.TaskID)
	if err != nil {
		return nil, err
	}
	if err := requirePairwiseCase(c); err != nil {
		return nil, err
	}
	run, err := pairwiseRun(c.Pairwise, req.Side)
	if err != nil {
		return nil, err
	}
	source, err := s.verifyBinding(ctx, c)
	if err != nil {
		return nil, err
	}
	before, err := domain.TreeHash(ctx, source)
	if err != nil {
		return nil, err
	}
	files, main, err := s.traceFiles(ctx, c, strings.TrimSpace(req.TracePath))
	if err != nil {
		return nil, err
	}
	rounds, err := domain.ParseTrace(files[main])
	if err != nil {
		return nil, err
	}
	if len(rounds) != 1 || rounds[0].Status == "excluded" {
		return nil, errors.New("Pair-wise 每个 Session 必须且只能包含一轮有效交互")
	}
	if rounds[0].Status != "complete" {
		return nil, errors.New("该 Session 首轮尚未完整结束")
	}
	if err := validateTraceSource(c, source, rounds, c.Pairwise.Prompt); err != nil {
		return nil, err
	}
	sessionID := strings.TrimSpace(rounds[0].SessionID)
	if sessionID == "" {
		return nil, errors.New("轨迹缺少真实 SessionID")
	}
	other, _ := pairwiseRun(c.Pairwise, oppositePairwiseSide(req.Side))
	if other.SessionID != "" && other.SessionID == sessionID {
		return nil, errors.New("A/B SessionID 必须不同")
	}
	id := uuid.NewString()
	dir := filepath.Join(s.caseDir(c.TaskID), "pairwise-captures", strings.ToLower(string(req.Side)), id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = os.RemoveAll(dir)
		}
	}()
	codePath := filepath.Join(dir, "code")
	captureHash, err := domain.CopyEvidenceTree(ctx, source, codePath)
	if err != nil {
		return nil, err
	}
	for name, data := range files {
		target := filepath.Join(dir, "traces", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			return nil, err
		}
	}
	after, err := domain.TreeHash(ctx, source)
	if err != nil {
		return nil, err
	}
	filesAfter, _, err := s.traceFiles(ctx, c, strings.TrimSpace(req.TracePath))
	if err != nil {
		return nil, err
	}
	if before != captureHash || captureHash != after || digestFiles(files) != digestFiles(filesAfter) {
		return nil, errors.New("代码或轨迹仍在变化，请等待模型完成后再采集")
	}
	traceRoot := filepath.Join(dir, "traces")
	traceHash, err := domain.TreeHash(ctx, traceRoot)
	if err != nil {
		return nil, err
	}
	manifest, err := json.MarshalIndent(rounds, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "rounds.json"), manifest, 0o600); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	run.SessionID = sessionID
	run.TracePath = strings.TrimSpace(req.TracePath)
	run.TurnCount = 1
	run.CaptureID = id
	run.CaptureHash = captureHash
	run.TraceHash = traceHash
	run.CapturedAt = now
	c.Captures = append(c.Captures, domain.Capture{
		ID: id, Dir: dir, TracePath: filepath.Join(traceRoot, filepath.FromSlash(main)),
		CodePath: codePath, Hash: captureHash, TraceHash: traceHash, CreatedAt: now,
	})
	saved, err := s.store.SaveAnnotationCase(*c, c.Revision)
	if err != nil {
		return nil, err
	}
	accepted = true
	return saved, nil
}

func (s *AnnotationService) SavePairwiseMaterials(req PairwiseMaterialsRequest) (*domain.Case, error) {
	unlock, err := s.lockTask(req.TaskID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	c, err := s.loadCase(req.TaskID)
	if err != nil {
		return nil, err
	}
	if err := requirePairwiseCase(c); err != nil {
		return nil, err
	}
	run, err := pairwiseRun(c.Pairwise, req.Side)
	if err != nil {
		return nil, err
	}
	videoURL := strings.TrimSpace(req.VideoURL)
	if videoURL != "" {
		u, err := url.Parse(videoURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return nil, errors.New("运行视频必须填写可访问的 HTTP(S) 链接")
		}
		run.VideoStatus = domain.PairwiseVideoReady
	} else if strings.TrimSpace(req.RecordingError) != "" {
		run.VideoStatus = domain.PairwiseVideoManualRequired
	} else {
		run.VideoStatus = domain.PairwiseVideoMissing
	}
	run.VideoURL = videoURL
	run.RecordingError = strings.TrimSpace(req.RecordingError)
	return s.store.SaveAnnotationCase(*c, c.Revision)
}
