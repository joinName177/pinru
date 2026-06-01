package prompt

import (
	"strings"
	"testing"

	"github.com/blueship581/pinru/internal/analysis"
)

func TestSplitPromptSectionsSeparatesConstraintLines(t *testing.T) {
	raw := "筛选条件切换后列表偶尔还停留在上一轮结果，需要保证页面只展示最后一次筛选结果。\n业务逻辑约束：已下架商品不能重新出现在可售列表。\n非代码回复约束：只描述用户看到的结果。"

	body, constraints := SplitPromptSections(raw)

	if body != "筛选条件切换后列表偶尔还停留在上一轮结果，需要保证页面只展示最后一次筛选结果。" {
		t.Fatalf("body = %q", body)
	}
	if len(constraints) != 2 {
		t.Fatalf("len(constraints) = %d, want 2", len(constraints))
	}
}

func TestPromptBodyRuneCountIgnoresConstraintLinesAndWhitespace(t *testing.T) {
	raw := "  登录后首页偶尔还是游客状态，需要保证刷新后立刻展示会员身份。 \n\n业务逻辑约束：游客缓存不能覆盖登录态。\n"

	if got := PromptBodyRuneCount(raw); got != 30 {
		t.Fatalf("PromptBodyRuneCount() = %d, want 30", got)
	}
}

func TestNormalizeTaskTypeSupportsZeroToOneAliases(t *testing.T) {
	tests := []string{"代码生成", "0-1代码生成", "0-1", "从零到一", "从0到1", "0到1"}
	for _, input := range tests {
		if got := NormalizeTaskType(input); got != TaskTypeCodeGen {
			t.Fatalf("NormalizeTaskType(%q) = %q, want %q", input, got, TaskTypeCodeGen)
		}
	}
}

func TestBuildSystemPromptUsesExpandedLengthGuidance(t *testing.T) {
	prompt := BuildSystemPrompt()
	if !strings.Contains(prompt, "150-300 个字") {
		t.Fatalf("BuildSystemPrompt() missing expanded length range: %q", prompt)
	}
	if strings.Contains(prompt, "80 个字") {
		t.Fatalf("BuildSystemPrompt() still contains legacy 80-char limit: %q", prompt)
	}
}

func TestBuildUserPromptUsesZeroToOneTaskTypeAndNewLimit(t *testing.T) {
	got := BuildUserPrompt(
		TaskInfo{ProjectName: "PINRU"},
		PromptRequest{TaskType: "从零到一"},
		analysis.Summary{DetectedStack: []string{"Go"}, TotalFiles: 12},
		"",
	)

	for _, want := range []string{
		"任务类型：0-1代码生成",
		"基于现有系统补齐完整新模块/新能力",
		"150-300 个字之间",
		"最多不超过 300 个字",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("BuildUserPrompt() missing %q in: %q", want, got)
		}
	}
}

func TestPromptBodyExceedsLimitUsesThreeHundredRuneCap(t *testing.T) {
	if PromptBodyExceedsLimit(strings.Repeat("好", MaxPromptBodyRunes)) {
		t.Fatalf("PromptBodyExceedsLimit() should allow exactly %d runes", MaxPromptBodyRunes)
	}
	if !PromptBodyExceedsLimit(strings.Repeat("好", MaxPromptBodyRunes+1)) {
		t.Fatalf("PromptBodyExceedsLimit() should reject more than %d runes", MaxPromptBodyRunes)
	}
}
