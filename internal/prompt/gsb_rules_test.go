package prompt

import "testing"

func TestGSBHardLabelDoesNotReplaceFeatureEvidence(t *testing.T) {
	complete := "用户点击建议后先带入金额再跳转入账，入账完成后重新计算建议；已经补过仓的类别不再重复出现，避免用户按旧提示重复入账。"
	for _, kind := range []string{TaskTypeFeature, TaskTypeBugFix} {
		if err := ValidatePromptDifficultyEvidence(complete, kind, "困难"); err != nil {
			t.Errorf("%s: complete G19 evidence rejected: %v", kind, err)
		}
		incomplete := "用户修改订单后保存记录，同时更新列表、详情和统计。"
		if err := ValidatePromptDifficultyEvidence(incomplete, kind, "困难"); err == nil {
			t.Errorf("%s: module names bypassed missing G19 decision and boundary", kind)
		}
	}
}

func TestGSBRejectsOrdinaryReminderAsCodeGeneration(t *testing.T) {
	text := "新增饮水提醒功能，用户保存目标后读取摄入记录，计算剩余量并更新列表和统计；保存失败时保留原值并提示重试，没有记录时显示空态。"
	if err := ValidatePromptDifficultyEvidence(text, TaskTypeCodeGen, "困难"); err == nil {
		t.Fatal("ordinary reminder with save/retry boundaries must not satisfy G16")
	}
}

func TestGSBDefaultLabelsAreDifficult(t *testing.T) {
	for _, counts := range []DocumentCounts{DefaultDocumentCounts(), (DocumentCounts{Feature: 2, BugFix: 1}).NormalizeDifficultyAllocation()} {
		if counts.Medium != 0 || counts.Difficult != counts.Total() {
			t.Errorf("generated labels must default to difficult: %+v", counts)
		}
	}
}

func TestGSBListAndStatisticsAreNotSeparateBusinessModules(t *testing.T) {
	text := "运营切换订单筛选后，系统先按条件查询再刷新列表和统计；两处结果对不上时以列表查询结果为准，没有符合条件的订单时保留筛选条件并显示空态。"
	if err := ValidatePromptDifficultyEvidence(text, TaskTypeCodeGen, "困难"); err == nil {
		t.Fatal("ordinary filtering must not pass G16 as multi-module integration")
	}
}
