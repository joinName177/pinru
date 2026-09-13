package prompt

import (
	"fmt"
	"regexp"
	"strings"
)

// The remaining four task types are fixed at one per generated document.
type DocumentCounts struct {
	CodeGen    int    `json:"codeGen"`
	Feature    int    `json:"feature"`
	BugFix     int    `json:"bugFix"`
	Difficulty string `json:"difficulty,omitempty"`
}

func DefaultDocumentCounts() DocumentCounts {
	return DocumentCounts{CodeGen: 8, Feature: 8, Difficulty: "auto"}
}

func (c DocumentCounts) Validate() error {
	if c.CodeGen < 0 || c.Feature < 0 || c.BugFix < 0 {
		return fmt.Errorf("题型数量必须为非负整数")
	}
	// Keep arithmetic and browser integer representation consistent.
	const maxCount = 9007199254740987
	if uint64(c.CodeGen) > maxCount || uint64(c.Feature) > maxCount || uint64(c.BugFix) > maxCount || uint64(c.CodeGen)+uint64(c.Feature)+uint64(c.BugFix) > maxCount {
		return fmt.Errorf("题型总数过大")
	}
	switch strings.TrimSpace(c.Difficulty) {
	case "", "auto", "general", "challenging":
	default:
		return fmt.Errorf("批次难度必须是 auto、general 或 challenging")
	}
	return nil
}

func (c DocumentCounts) ByType() map[string]int {
	return map[string]int{"0-1代码生成": c.CodeGen, "Feature迭代": c.Feature, "Bug修复": c.BugFix, "代码理解": 1, "工程化": 1, "代码测试": 1, "代码重构": 1}
}

func (c DocumentCounts) ValidateDocument(content string) error {
	counts := map[string]int{}
	heading := regexp.MustCompile(`^\s{0,3}(?:#{1,6}\s*)?(?:\*\*)?\s*(0-1代码生成|Feature迭代|代码理解|Bug修复|代码重构|工程化|代码测试)\s*(?:\*\*)?\s*$`)
	item := regexp.MustCompile(`^\s*(?:[-*]\s+|\d+[.、)]\s+)【(?:简单|一般|困难|地狱)】\s*\S`)
	current := ""
	for _, line := range strings.Split(content, "\n") {
		if match := heading.FindStringSubmatch(line); len(match) > 1 {
			current = match[1]
			continue
		}
		if current != "" && item.MatchString(line) {
			counts[current]++
		}
	}
	for _, kind := range []string{"0-1代码生成", "Feature迭代", "Bug修复", "代码理解", "工程化", "代码测试", "代码重构"} {
		if counts[kind] != c.ByType()[kind] {
			return fmt.Errorf("生成文档的 %s 数量不符：要求 %d 条，实际 %d 条，请重新生成", kind, c.ByType()[kind], counts[kind])
		}
	}
	return nil
}
