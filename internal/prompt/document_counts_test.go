package prompt

import (
	"fmt"
	"strings"
	"testing"
)

func TestDocumentCountsValidation(t *testing.T) {
	for _, counts := range []DocumentCounts{{}, {CodeGen: 3, Feature: 2, BugFix: 4, General: 7, Difficult: 6}, DefaultDocumentCounts()} {
		if err := counts.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, counts := range []DocumentCounts{{CodeGen: -1}, {Feature: -1}, {BugFix: -1}, {General: -1}, {Difficult: -1}, {CodeGen: 9007199254740991}, {CodeGen: 1, General: 1, Difficult: 1}} {
		if err := counts.Validate(); err == nil {
			t.Fatalf("accepted invalid counts: %+v", counts)
		}
	}
}

func TestDocumentCountsNormalizesMissingDifficultyAllocation(t *testing.T) {
	counts := (DocumentCounts{Feature: 2, BugFix: 3}).NormalizeDifficultyAllocation()
	if counts.General != 5 || counts.Difficult != 4 {
		t.Fatalf("difficulty allocation = %+v, want 5 general and 4 difficult", counts)
	}
	defaults := DefaultDocumentCounts()
	if defaults.General != 10 || defaults.Difficult != 10 {
		t.Fatalf("default difficulty allocation = %+v", defaults)
	}
}

func TestDocumentCountsCheckActualOutput(t *testing.T) {
	counts := DocumentCounts{Feature: 2, BugFix: 3, General: 5, Difficult: 4}
	var doc strings.Builder
	itemIndex := 0
	for _, kind := range []string{"Feature迭代", "Bug修复", "代码理解", "工程化", "代码测试", "代码重构"} {
		fmt.Fprintf(&doc, "**%s**\n", kind)
		for i := 1; i <= counts.ByType()[kind]; i++ {
			difficulty := "困难"
			if itemIndex < counts.General {
				difficulty = "一般"
			}
			fmt.Fprintf(&doc, "%d. 【%s】实际需求内容 README\n", i, difficulty)
			itemIndex++
		}
	}
	if err := counts.ValidateDocument(doc.String()); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{
		strings.Replace(doc.String(), "3. 【一般】实际需求内容 README\n", "", 1),
		doc.String() + "2. 【一般】多余的重构题\n",
		strings.Replace(doc.String(), "【困难】", "【一般】", 1),
		strings.Replace(doc.String(), "【困难】", "【简单】", 1),
	} {
		if err := counts.ValidateDocument(output); err == nil {
			t.Fatal("accepted output with wrong counts")
		}
	}
}
