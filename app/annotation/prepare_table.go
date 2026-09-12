package annotation

import (
	"context"
	"errors"
	"fmt"

	domain "github.com/blueship581/pinru/internal/annotation"
)

// captureAndPrepareTable persists each result separately so a cancelled or failed
// review can resume from the saved capture and evaluations. It never runs a prompt.
func (s *AnnotationService) captureAndPrepareTable(ctx context.Context, req CaptureRequest) (*domain.Case, error) {
	unlock, err := s.lockTask(req.TaskID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	domain.ReportProgress(ctx, 5, "正在采集轨迹与代码快照")
	c, err := s.captureLocked(ctx, req)
	if err != nil {
		return nil, err
	}
	var rounds []domain.Round
	for _, r := range c.Rounds {
		if r.Status == "complete" {
			rounds = append(rounds, r)
		}
	}
	if len(rounds) == 0 {
		return nil, errors.New("采集已保存，但没有已完成的有效轮次可供准备制表数据")
	}
	for i, r := range rounds {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		domain.ReportProgress(ctx, 15+80*i/len(rounds), fmt.Sprintf("轨迹已采集，正在使用 skill 准备制表数据：第 %d 轮（%d/%d）", r.Order, i+1, len(rounds)))
		c, err = s.reviewLocked(ctx, ReviewRequest{TaskID: req.TaskID, PromptID: r.PromptID})
		if err != nil {
			return nil, fmt.Errorf("采集已保存，第 %d 轮制表数据准备失败（已完成的评分保留）：%w", r.Order, err)
		}
	}
	return c, nil
}
