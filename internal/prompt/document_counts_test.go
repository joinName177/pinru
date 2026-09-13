package prompt

import (
	"fmt"
	"strings"
	"testing"
)

func TestDocumentCountsValidation(t *testing.T) {
	for _, counts := range []DocumentCounts{{}, {CodeGen: 3, Feature: 2, BugFix: 4, Difficulty: "auto"}, {Difficulty: "general"}, {Difficulty: "challenging"}, DefaultDocumentCounts()} {
		if err := counts.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, counts := range []DocumentCounts{{CodeGen: -1}, {Feature: -1}, {BugFix: -1}, {CodeGen: 9007199254740991}, {Difficulty: "hard"}} {
		if err := counts.Validate(); err == nil {
			t.Fatalf("accepted invalid counts: %+v", counts)
		}
	}
}

func TestDocumentCountsCheckActualOutput(t *testing.T) {
	counts := DocumentCounts{Feature: 2, BugFix: 3}
	var doc strings.Builder
	for _, kind := range []string{"Feature迭代", "Bug修复", "代码理解", "工程化", "代码测试", "代码重构"} {
		fmt.Fprintf(&doc, "**%s**\n", kind)
		for i := 1; i <= counts.ByType()[kind]; i++ {
			fmt.Fprintf(&doc, "%d. 【一般】实际需求内容 README\n", i)
		}
	}
	if err := counts.ValidateDocument(doc.String()); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{strings.Replace(doc.String(), "3. 【一般】实际需求内容 README\n", "", 1), doc.String() + "2. 【一般】多余的重构题\n"} {
		if err := counts.ValidateDocument(output); err == nil {
			t.Fatal("accepted output with wrong counts")
		}
	}
}
