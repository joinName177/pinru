package prompt

import (
	"fmt"
	"regexp"
	"strings"
)

// Only code generation, feature iteration, and bug fixing are generated.
type DocumentCounts struct {
	CodeGen   int `json:"codeGen"`
	Feature   int `json:"feature"`
	BugFix    int `json:"bugFix"`
	General   int `json:"general"`
	Difficult int `json:"difficult"`
}

func DefaultDocumentCounts() DocumentCounts {
	return DocumentCounts{CodeGen: 10, Feature: 10, BugFix: 2, General: 0, Difficult: 22}
}

func (c DocumentCounts) Total() int {
	return c.CodeGen + c.Feature + c.BugFix
}

// NormalizeDifficultyAllocation keeps requests from older clients usable. New
// clients always send both values; an omitted allocation is split as evenly as
// possible, with the extra item assigned to the general group.
func (c DocumentCounts) NormalizeDifficultyAllocation() DocumentCounts {
	if c.General == 0 && c.Difficult == 0 && c.CodeGen >= 0 && c.Feature >= 0 && c.BugFix >= 0 {
		total := c.Total()
		c.General = (total + 1) / 2
		c.Difficult = total / 2
	}
	return c
}

func (c DocumentCounts) Validate() error {
	if c.CodeGen < 0 || c.Feature < 0 || c.BugFix < 0 || c.General < 0 || c.Difficult < 0 {
		return fmt.Errorf("题型和难度数量必须为非负整数")
	}
	c = c.NormalizeDifficultyAllocation()
	// Keep arithmetic and browser integer representation consistent.
	const maxCount = 9007199254740987
	if uint64(c.CodeGen) > maxCount || uint64(c.Feature) > maxCount || uint64(c.BugFix) > maxCount || uint64(c.General) > maxCount || uint64(c.Difficult) > maxCount || uint64(c.CodeGen)+uint64(c.Feature)+uint64(c.BugFix) > maxCount {
		return fmt.Errorf("题型总数过大")
	}
	if uint64(c.General)+uint64(c.Difficult) != uint64(c.Total()) {
		return fmt.Errorf("难度数量合计 %d 题，与题型总数 %d 题不一致", c.General+c.Difficult, c.Total())
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
	heading := regexp.MustCompile(`^\s{0,3}(?:#{1,6}\s*)?(?:\*\*)?\s*(0-1代码生成|Feature迭代|代码理解|Bug修复|代码重构|工程化|代码测试)\s*(?:\*\*)?\s*$`)
	item := regexp.MustCompile(`^\s*(?:[-*]\s+|\d+[.、)]\s+)【(简单|一般|困难|地狱)】\s*(\S.*)$`)
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
			promptText := strings.TrimSpace(match[2])
			if err := ValidatePromptWritingQuality(promptText); err != nil {
				return fmt.Errorf("生成文档的第 %d 条提示词文案不符合要求：%w", len(prompts)+1, err)
			}
			prompts = append(prompts, promptText)
		}
	}
	for _, kind := range []string{"0-1代码生成", "Feature迭代", "Bug修复", "代码理解", "工程化", "代码测试", "代码重构"} {
		if counts[kind] != c.ByType()[kind] {
			return fmt.Errorf("生成文档的 %s 数量不符：要求 %d 条，实际 %d 条，请重新生成", kind, c.ByType()[kind], counts[kind])
		}
	}
	if difficultyCounts["简单"] > 0 || difficultyCounts["地狱"] > 0 {
		return fmt.Errorf("生成文档只允许一般和困难两种难度：发现简单 %d 条、地狱 %d 条，请重新生成", difficultyCounts["简单"], difficultyCounts["地狱"])
	}
	if difficultyCounts["一般"] != c.General || difficultyCounts["困难"] != c.Difficult {
		return fmt.Errorf("生成文档的难度数量不符：要求一般 %d 条、困难 %d 条，实际一般 %d 条、困难 %d 条，请重新生成", c.General, c.Difficult, difficultyCounts["一般"], difficultyCounts["困难"])
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
