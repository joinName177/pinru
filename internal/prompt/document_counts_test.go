package prompt

import (
	"fmt"
	"strings"
	"testing"
)

func TestDocumentCountsValidation(t *testing.T) {
	for _, counts := range []DocumentCounts{{}, {CodeGen: 3, Feature: 2, BugFix: 4, General: 6, Difficult: 6}, DefaultDocumentCounts()} {
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
	if counts.General != 4 || counts.Difficult != 4 {
		t.Fatalf("difficulty allocation = %+v, want 4 general and 4 difficult", counts)
	}
	defaults := DefaultDocumentCounts()
	if defaults.CodeGen != 10 || defaults.Feature != 10 || defaults.BugFix != 2 || defaults.General != 13 || defaults.Difficult != 12 || defaults.Total() != 25 {
		t.Fatalf("default difficulty allocation = %+v", defaults)
	}
	if defaults.ByType()["工程化"] != 0 {
		t.Fatalf("default engineering count = %d, want 0", defaults.ByType()["工程化"])
	}
}

func TestDocumentCountsCheckActualOutput(t *testing.T) {
	counts := DocumentCounts{Feature: 2, BugFix: 3, General: 4, Difficult: 4}
	promptTexts := []string{
		"在现有订单列表中补充配送方式筛选，并保持翻页后的筛选状态。",
		"会员取消预约后同步释放名额，让候补用户能及时收到可预约通知。",
		"修复商品下架后仍出现在搜索结果中的问题，并刷新相关缓存。",
		"修复重复提交退款申请时生成两条记录的问题，保留首次处理结果。",
		"修复课程改期后学员端仍显示原上课时间的问题，确保通知内容同步更新。",
		"梳理发布流程中的状态变化和失败分支，并把结论写入 README 文档。",
		"补充订单超时关闭和重复支付场景的回归测试，覆盖状态恢复结果。",
		"整理会员等级计算逻辑，在不改变现有规则的情况下减少重复分支。",
	}
	var doc strings.Builder
	itemIndex := 0
	for _, kind := range []string{"Feature迭代", "Bug修复", "代码理解", "代码测试", "代码重构"} {
		fmt.Fprintf(&doc, "**%s**\n", kind)
		for i := 1; i <= counts.ByType()[kind]; i++ {
			difficulty := "困难"
			if itemIndex < counts.General {
				difficulty = "一般"
			}
			fmt.Fprintf(&doc, "%d. 【%s】%s\n", i, difficulty, promptTexts[itemIndex])
			itemIndex++
		}
	}
	if err := counts.ValidateDocument(doc.String()); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{
		strings.Replace(doc.String(), "1. 【一般】在现有订单列表中补充配送方式筛选，并保持翻页后的筛选状态。\n", "", 1),
		doc.String() + "2. 【一般】多余的重构题\n",
		strings.Replace(doc.String(), "【困难】", "【一般】", 1),
		strings.Replace(doc.String(), "【困难】", "【简单】", 1),
	} {
		if err := counts.ValidateDocument(output); err == nil {
			t.Fatal("accepted output with wrong counts")
		}
	}
}

func TestDocumentCountsValidateDocumentRejectsRepeatedPromptBodies(t *testing.T) {
	counts := DocumentCounts{CodeGen: 1, Feature: 1, General: 3, Difficult: 2}
	document := strings.Join([]string{
		"**0-1代码生成**",
		"1. 【一般】会员提交预约后，管理员可以确认到店状态，并在列表中查看最新处理结果。",
		"**Feature迭代**",
		"1. 【困难】会员提交预约后，管理员可以确认离店状态，并在列表中查看最新处理结果。",
		"**代码理解**",
		"1. 【一般】梳理订单提交到完成的主要流程，并把结论写入 README 文档。",
		"**代码测试**",
		"1. 【困难】补充订单失败和重复提交场景的回归测试。",
		"**代码重构**",
		"1. 【一般】整理订单状态处理逻辑，并保持现有业务行为不变。",
	}, "\n")

	err := counts.ValidateDocument(document)
	if err == nil || !strings.Contains(err.Error(), "重复比例") {
		t.Fatalf("ValidateDocument() error = %v, want repeated prompt rejection", err)
	}
}
