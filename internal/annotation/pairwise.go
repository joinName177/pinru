package annotation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

type CaseMode string

const (
	CaseModeLegacy      CaseMode = "legacy"
	CaseModePairwiseGSB CaseMode = "pairwise_gsb"
)

type PairwiseSide string

const (
	PairwiseSideA PairwiseSide = "A"
	PairwiseSideB PairwiseSide = "B"
)

const (
	PairwiseVideoMissing        = "missing"
	PairwiseVideoRecording      = "recording"
	PairwiseVideoReady          = "ready"
	PairwiseVideoManualRequired = "manual_required"
	PairwiseVideoFailed         = "failed"

	PairwiseReviewReady         = "ready"
	PairwiseReviewNeedsEvidence = "needs_evidence"

	PairwiseConclusionA    = "A_better"
	PairwiseConclusionSame = "same"
	PairwiseConclusionB    = "B_better"
)

type PairwiseRun struct {
	Side           PairwiseSide `json:"side"`
	Branch         string       `json:"branch"`
	SessionID      string       `json:"sessionId"`
	TracePath      string       `json:"tracePath"`
	TurnCount      int          `json:"turnCount"`
	CaptureID      string       `json:"captureId"`
	CaptureHash    string       `json:"captureHash"`
	TraceHash      string       `json:"traceHash"`
	DeliverableSHA string       `json:"deliverableSha"`
	DeliverableURL string       `json:"deliverableUrl"`
	VideoStatus    string       `json:"videoStatus"`
	VideoPath      string       `json:"videoPath"`
	VideoURL       string       `json:"videoUrl"`
	RecordingError string       `json:"recordingError"`
	PreparedAt     int64        `json:"preparedAt"`
	CapturedAt     int64        `json:"capturedAt"`
	CommittedAt    int64        `json:"committedAt"`
}

type PairwiseReview struct {
	Current     *bool  `json:"current,omitempty"`
	ID          string `json:"id"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	Reason      string `json:"reason"`
	Model       string `json:"model"`
	SkillHash   string `json:"skillHash"`
	SourceHashA string `json:"sourceHashA"`
	SourceHashB string `json:"sourceHashB"`
	ReviewPath  string `json:"reviewPath"`
	ReviewHash  string `json:"reviewHash"`
	CreatedAt   int64  `json:"createdAt"`
}

type PairwiseData struct {
	Prompt            string           `json:"prompt"`
	Harness           string           `json:"harness"`
	HarnessVersion    string           `json:"harnessVersion"`
	OS                string           `json:"os"`
	Environment       string           `json:"environment"`
	RunA              PairwiseRun      `json:"runA"`
	RunB              PairwiseRun      `json:"runB"`
	Reviews           []PairwiseReview `json:"reviews"`
	Notes             string           `json:"notes"`
	AutoRecordEnabled bool             `json:"autoRecordEnabled"`
}

var pairwiseSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func NormalizeCase(c *Case) {
	if c == nil {
		return
	}
	if c.Mode == "" {
		c.Mode = CaseModeLegacy
	}
	if c.Rounds == nil {
		c.Rounds = []Round{}
	}
	if c.Captures == nil {
		c.Captures = []Capture{}
	}
	for i := range c.Rounds {
		if c.Rounds[i].Attachments == nil {
			c.Rounds[i].Attachments = []string{}
		}
		if c.Rounds[i].Evaluations == nil {
			c.Rounds[i].Evaluations = []Evaluation{}
		}
	}
	if c.Pairwise != nil {
		if c.Pairwise.RunA.Side == "" {
			c.Pairwise.RunA.Side = PairwiseSideA
		}
		if c.Pairwise.RunB.Side == "" {
			c.Pairwise.RunB.Side = PairwiseSideB
		}
		if c.Pairwise.RunA.Branch == "" {
			c.Pairwise.RunA.Branch = "A"
		}
		if c.Pairwise.RunB.Branch == "" {
			c.Pairwise.RunB.Branch = "B"
		}
		if c.Pairwise.RunA.VideoStatus == "" {
			c.Pairwise.RunA.VideoStatus = PairwiseVideoMissing
		}
		if c.Pairwise.RunB.VideoStatus == "" {
			c.Pairwise.RunB.VideoStatus = PairwiseVideoMissing
		}
		if c.Pairwise.Reviews == nil {
			c.Pairwise.Reviews = []PairwiseReview{}
		}
	}
}

func NewPairwiseData(prompt string) *PairwiseData {
	p := &PairwiseData{
		Prompt:  strings.TrimSpace(prompt),
		RunA:    PairwiseRun{Side: PairwiseSideA, Branch: "A", VideoStatus: PairwiseVideoMissing},
		RunB:    PairwiseRun{Side: PairwiseSideB, Branch: "B", VideoStatus: PairwiseVideoMissing},
		Reviews: []PairwiseReview{},
	}
	return p
}

func PairwiseRunSourceHash(run PairwiseRun) string {
	value := strings.Join([]string{
		run.CaptureID, run.CaptureHash, run.TraceHash, strings.ToLower(run.DeliverableSHA),
		run.DeliverableURL, run.VideoStatus, run.VideoPath, run.VideoURL,
	}, "\x00")
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:16])
}

func CurrentPairwiseReview(c Case) *PairwiseReview {
	if c.Pairwise == nil {
		return nil
	}
	a := PairwiseRunSourceHash(c.Pairwise.RunA)
	b := PairwiseRunSourceHash(c.Pairwise.RunB)
	for i := len(c.Pairwise.Reviews) - 1; i >= 0; i-- {
		r := &c.Pairwise.Reviews[i]
		if r.Status == PairwiseReviewReady && r.SourceHashA == a && r.SourceHashB == b {
			return r
		}
	}
	return nil
}

func ValidatePairwiseCase(c Case, formal bool) []string {
	issues := make([]string, 0)
	if c.Mode != CaseModePairwiseGSB || c.Pairwise == nil {
		return append(issues, "题目尚未启用 Pair-wise GSB 模式")
	}
	p := c.Pairwise
	if strings.TrimSpace(p.Prompt) == "" {
		issues = append(issues, "User Prompt 未填写")
	}
	if !pairwiseSHA.MatchString(c.InitialSHA) {
		issues = append(issues, "初始快照必须是完整 40 位 SHA")
	}
	if strings.TrimSpace(c.SnapshotURL) == "" {
		issues = append(issues, "初始快照地址未填写")
	}
	if strings.TrimSpace(p.Harness) == "" || strings.TrimSpace(p.HarnessVersion) == "" {
		issues = append(issues, "Harness 和版本必须填写")
	}
	if strings.TrimSpace(p.OS) == "" {
		issues = append(issues, "操作系统未填写")
	}
	validatePairwiseRun := func(label string, expected PairwiseSide, run PairwiseRun) {
		if run.Side != expected || run.Branch != string(expected) {
			issues = append(issues, fmt.Sprintf("%s 分支名称必须固定为 %s", label, expected))
		}
		if strings.TrimSpace(run.SessionID) == "" {
			issues = append(issues, label+" SessionID 未填写")
		}
		if run.TurnCount != 1 {
			issues = append(issues, label+" 必须且只能包含一轮有效交互")
		}
		if run.CaptureID == "" || run.CaptureHash == "" || run.TraceHash == "" {
			issues = append(issues, label+" 轨迹或代码证据不完整")
		}
		if !pairwiseSHA.MatchString(run.DeliverableSHA) || strings.TrimSpace(run.DeliverableURL) == "" {
			issues = append(issues, label+" 产物快照不完整")
		}
		if formal && (run.VideoStatus != PairwiseVideoReady || (strings.TrimSpace(run.VideoURL) == "" && strings.TrimSpace(run.VideoPath) == "")) {
			issues = append(issues, label+" 运行视频尚未就绪")
		}
	}
	validatePairwiseRun("A", PairwiseSideA, p.RunA)
	validatePairwiseRun("B", PairwiseSideB, p.RunB)
	if p.RunA.SessionID != "" && p.RunA.SessionID == p.RunB.SessionID {
		issues = append(issues, "A/B SessionID 必须不同")
	}
	if formal && CurrentPairwiseReview(c) == nil {
		issues = append(issues, "缺少与当前 A/B 证据一致的 GSB 评价")
	}
	return issues
}
