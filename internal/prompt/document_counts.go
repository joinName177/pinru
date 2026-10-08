package prompt

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Generated Feature/Bug labels are difficult; only CodeGen may use hell.
type DocumentCounts struct {
	CodeGen int `json:"codeGen"`
	Feature int `json:"feature"`
	BugFix  int `json:"bugFix"`
	Medium  int `json:"medium"`
	// EasierDifficult is kept for clients from the intermediate 较难 config.
	// New clients should send Medium instead.
	EasierDifficult int `json:"easierDifficult"`
	Difficult       int `json:"difficult"`
	Hell            int `json:"hell"`
}

// MinGeneratedPromptRunes 是单条生成提示词的非空白字符下限。
// 低于该长度的需求通常只能对应单点小修，直接拒绝以落实“不生成简单需求”。
const MinGeneratedPromptRunes = 60

func DefaultDocumentCounts() DocumentCounts {
	return DocumentCounts{CodeGen: 10, Feature: 10, BugFix: 2, Medium: 0, Difficult: 22, Hell: 0}
}

func (c DocumentCounts) Total() int {
	return c.CodeGen + c.Feature + c.BugFix
}

// NormalizeDifficultyAllocation defaults unspecified allocations to all difficult.
// Legacy medium allocations remain visible so Validate can report them.
func (c DocumentCounts) NormalizeDifficultyAllocation() DocumentCounts {
	if c.Medium == 0 && c.EasierDifficult != 0 {
		c.Medium = c.EasierDifficult
	}
	if c.Medium == 0 && c.Difficult == 0 && c.Hell == 0 && c.Total() > 0 {
		c.Difficult = c.Total()
	}
	c.EasierDifficult = 0
	return c
}

func (c DocumentCounts) Validate() error {
	if c.CodeGen < 0 || c.Feature < 0 || c.BugFix < 0 || c.Medium < 0 || c.EasierDifficult < 0 || c.Difficult < 0 || c.Hell < 0 {
		return fmt.Errorf("题型和难度数量必须为非负整数")
	}
	c = c.NormalizeDifficultyAllocation()
	// Keep arithmetic and browser integer representation consistent.
	const maxCount = 9007199254740987
	if uint64(c.CodeGen) > maxCount || uint64(c.Feature) > maxCount || uint64(c.BugFix) > maxCount || uint64(c.Medium) > maxCount || uint64(c.EasierDifficult) > maxCount || uint64(c.Difficult) > maxCount || uint64(c.Hell) > maxCount || uint64(c.CodeGen)+uint64(c.Feature)+uint64(c.BugFix) > maxCount {
		return fmt.Errorf("题型总数过大")
	}
	// Content acceptance and generated labels are separate contracts.
	if c.Medium > 0 {
		return fmt.Errorf("出题标签固定为困难（0-1可选地狱），中等数量必须为 0；Feature/Bug 的真实内容仍须达到 G19 三项门槛")
	}
	if c.Hell > c.CodeGen {
		return fmt.Errorf("地狱题数量不能超过 0-1代码生成数量：最多 %d 题，实际 %d 题", c.CodeGen, c.Hell)
	}
	if uint64(c.Medium)+uint64(c.Difficult)+uint64(c.Hell) != uint64(c.Total()) {
		return fmt.Errorf("难度数量合计 %d 题，与题型总数 %d 题不一致", c.Medium+c.Difficult+c.Hell, c.Total())
	}
	return nil
}

func (c DocumentCounts) ByType() map[string]int {
	return map[string]int{"0-1代码生成": c.CodeGen, "Feature迭代": c.Feature, "Bug修复": c.BugFix, "代码理解": 0, "工程化": 0, "代码测试": 0, "代码重构": 0}
}

func (c DocumentCounts) ValidateDocument(content string) error {
	c = c.NormalizeDifficultyAllocation()
	counts := map[string]int{}
	difficultyCounts := map[string]int{}
	typeDifficultyCounts := map[string]map[string]int{}
	heading := regexp.MustCompile(`^\s{0,3}(?:#{1,6}\s*)?(?:\*\*)?\s*(0-1代码生成|Feature迭代|代码理解|Bug修复|代码重构|工程化|代码测试)\s*(?:\*\*)?\s*$`)
	item := regexp.MustCompile(`^\s*(?:[-*]\s+|\d+[.、)]\s+)【(简单|一般|中等|较难|困难|地狱)】\s*(\S.*)$`)
	current := ""
	prompts := make([]string, 0, c.Total())
	for _, line := range strings.Split(content, "\n") {
		if match := heading.FindStringSubmatch(line); len(match) > 1 {
			current = match[1]
			continue
		}
		if match := item.FindStringSubmatch(line); current != "" && len(match) > 2 {
			counts[current]++
			difficultyCounts[match[1]]++
			if typeDifficultyCounts[current] == nil {
				typeDifficultyCounts[current] = make(map[string]int)
			}
			typeDifficultyCounts[current][match[1]]++
			promptText := strings.TrimSpace(match[2])
			if err := ValidatePromptWritingQuality(promptText); err != nil {
				return fmt.Errorf("生成文档的第 %d 条提示词文案不符合要求：%w", len(prompts)+1, err)
			}
			if runes := generatedPromptRuneCount(promptText); runes < MinGeneratedPromptRunes {
				return fmt.Errorf("生成文档的第 %d 条提示词只有 %d 个有效字符，低于 %d 字下限，属于简单需求，请改选有真实联动链路的题目", len(prompts)+1, runes, MinGeneratedPromptRunes)
			}
			if err := ValidatePromptDifficultyEvidence(promptText, current, match[1]); err != nil {
				return fmt.Errorf("生成文档的第 %d 条提示词难度证据不足：%w", len(prompts)+1, err)
			}
			prompts = append(prompts, promptText)
		}
	}
	for _, kind := range []string{"0-1代码生成", "Feature迭代", "Bug修复", "代码理解", "工程化", "代码测试", "代码重构"} {
		if counts[kind] != c.ByType()[kind] {
			return fmt.Errorf("生成文档的 %s 数量不符：要求 %d 条，实际 %d 条，请重新生成", kind, c.ByType()[kind], counts[kind])
		}
	}
	if difficultyCounts["简单"] > 0 || difficultyCounts["一般"] > 0 || difficultyCounts["较难"] > 0 {
		return fmt.Errorf("生成文档只允许中等、困难、地狱三种难度：发现简单 %d 条、一般 %d 条、较难 %d 条，请重新生成", difficultyCounts["简单"], difficultyCounts["一般"], difficultyCounts["较难"])
	}
	if difficultyCounts["中等"] != c.Medium || difficultyCounts["困难"] != c.Difficult || difficultyCounts["地狱"] != c.Hell {
		return fmt.Errorf("生成文档的难度数量不符：要求中等 %d 条、困难 %d 条、地狱 %d 条，实际中等 %d 条、困难 %d 条、地狱 %d 条，请重新生成", c.Medium, c.Difficult, c.Hell, difficultyCounts["中等"], difficultyCounts["困难"], difficultyCounts["地狱"])
	}
	// Feature/Bug 固定展示困难；0-1 可选困难或地狱。
	for _, kind := range []string{"Feature迭代", "Bug修复"} {
		if c.ByType()[kind] > 0 && (typeDifficultyCounts[kind]["地狱"] > 0 || typeDifficultyCounts[kind]["中等"] > 0) {
			return fmt.Errorf("生成文档的 %s 必须使用困难标签", kind)
		}
	}
	if c.CodeGen > 0 && typeDifficultyCounts["0-1代码生成"]["中等"] > 0 {
		return fmt.Errorf("生成文档的 0-1代码生成必须使用困难或地狱难度")
	}
	for left := 0; left < len(prompts); left++ {
		leftNormalized := normalizePromptWriting(prompts[left])
		for right := left + 1; right < len(prompts); right++ {
			rightNormalized := normalizePromptWriting(prompts[right])
			if leftNormalized == rightNormalized {
				return fmt.Errorf("生成文档的第 %d 条和第 %d 条提示词内容重复", left+1, right+1)
			}
			if len([]rune(leftNormalized)) >= 24 && len([]rune(rightNormalized)) >= 24 && promptWritingSimilarity(leftNormalized, rightNormalized) >= 0.86 {
				return fmt.Errorf("生成文档的第 %d 条和第 %d 条提示词重复比例过高，不能只替换少量词语", left+1, right+1)
			}
		}
	}
	return nil
}

// generatedPromptRuneCount 统计提示词正文的非空白字符数，用于拦截一句话式简单需求。
func generatedPromptRuneCount(promptText string) int {
	count := 0
	for _, r := range promptText {
		if unicode.IsSpace(r) {
			continue
		}
		count++
	}
	return count
}
