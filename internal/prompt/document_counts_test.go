package prompt

import (
	"fmt"
	"strings"
	"testing"
)

func TestDocumentCountsValidation(t *testing.T) {
	for _, counts := range []DocumentCounts{
		{},
		{CodeGen: 3, Feature: 2, BugFix: 4, Difficult: 8, Hell: 1},
		// 整批困难：中等 0、地狱 0、困难补足全部题型数量。
		{CodeGen: 3, Feature: 2, BugFix: 4, Difficult: 9},
		DefaultDocumentCounts(),
	} {
		if err := counts.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, counts := range []DocumentCounts{
		{CodeGen: -1},
		{Feature: -1},
		{BugFix: -1},
		{Medium: -1},
		{Difficult: -1},
		{Hell: -1},
		{CodeGen: 9007199254740991},
		{CodeGen: 1, Difficult: 1, Hell: 1},
		// 中等名额只能来自 Feature迭代和 Bug修复。
		{CodeGen: 1, Feature: 1, Medium: 2},
		// 地狱名额只能来自 0-1代码生成。
		{CodeGen: 1, Feature: 1, Hell: 2},
	} {
		if err := counts.Validate(); err == nil {
			t.Fatalf("accepted invalid counts: %+v", counts)
		}
	}
}

func TestDocumentCountsNormalizesMissingDifficultyAllocation(t *testing.T) {
	counts := (DocumentCounts{Feature: 2, BugFix: 3}).NormalizeDifficultyAllocation()
	if counts.Medium != 0 || counts.Difficult != 5 || counts.Hell != 0 {
		t.Fatalf("difficulty allocation = %+v, want 0 medium and 5 hard", counts)
	}
	defaults := DefaultDocumentCounts()
	if defaults.CodeGen != 10 || defaults.Feature != 10 || defaults.BugFix != 2 || defaults.Medium != 0 || defaults.Difficult != 22 || defaults.Hell != 0 || defaults.Total() != 22 {
		t.Fatalf("default difficulty allocation = %+v, want 0 medium and 22 difficult", defaults)
	}
	for _, taskType := range []string{"代码理解", "代码测试", "代码重构", "工程化"} {
		if defaults.ByType()[taskType] != 0 {
			t.Fatalf("default %s count = %d, want 0", taskType, defaults.ByType()[taskType])
		}
	}
}

func TestDocumentCountsCheckActualOutput(t *testing.T) {
	counts := DocumentCounts{Feature: 2, BugFix: 3, Difficult: 5}
	promptTexts := []string{
		"运营切换订单状态筛选后，系统先按新条件查询再刷新列表、汇总数量和导出结果；筛选条件变化时如果列表和统计对不上就以列表查询结果为准，没有符合条件的订单时保留筛选条件和分页位置并显示空态，刷新后不能回到上一轮数据。",
		"会员取消预约后，系统先释放名额并刷新候补名单，再向候补用户发送可预约通知；同一用户重复提交取消时只按第一次结果处理，通知重试只能生效一次，不能重复通知同一位用户。",
		"修复商品下架后仍出现在搜索结果中的问题，系统先刷新缓存，再展示最新的可售状态；缓存刷新失败时列表回退到数据库结果并提示重试，不能出现前台可售、后台已下架的冲突。",
		"修复重复提交退款申请时生成两条记录的问题，先校验请求再保留首次处理结果；退款状态、订单金额和财务明细在并发提交与失败重试下必须保持一致，重复请求只能生效一次。",
		"修复课程改期后学员端仍显示原上课时间的问题，确保通知内容、课表统计和教师端名单同步更新，历史已签到记录不能被改期覆盖；批量改期时如果部分课程失败，已签到记录必须保留并给出失败明细，不能出现部分成功部分回滚的情况。",
	}
	var doc strings.Builder
	itemIndex := 0
	for _, kind := range []string{"Feature迭代", "Bug修复"} {
		fmt.Fprintf(&doc, "**%s**\n", kind)
		for i := 1; i <= counts.ByType()[kind]; i++ {
			fmt.Fprintf(&doc, "%d. 【困难】%s\n", i, promptTexts[itemIndex])
			itemIndex++
		}
	}
	if err := counts.ValidateDocument(doc.String()); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{
		strings.Replace(doc.String(), "1. 【困难】运营切换订单状态筛选后，系统先按新条件查询再刷新列表、汇总数量和导出结果；筛选条件变化时如果列表和统计对不上就以列表查询结果为准，没有符合条件的订单时保留筛选条件和分页位置并显示空态，刷新后不能回到上一轮数据。\n", "", 1),
		doc.String() + "6. 【困难】补充一条与统计口径无关的额外记录，用于核对数量不匹配时的拒绝行为是否正确。\n",
		strings.Replace(doc.String(), "【困难】", "【一般】", 1),
		strings.Replace(doc.String(), "【困难】", "【简单】", 1),
		strings.Replace(doc.String(), promptTexts[0], "在现有订单列表中补充配送方式筛选。", 1),
	} {
		if err := counts.ValidateDocument(output); err == nil {
			t.Fatal("accepted output with wrong counts")
		}
	}
}

func TestDocumentCountsValidateAllDifficultDocument(t *testing.T) {
	// Feature迭代 和 Bug修复 的题面按困难标准撰写，标签可以是困难；0-1代码生成 照旧。
	counts := DocumentCounts{CodeGen: 1, Feature: 1, BugFix: 1, Difficult: 3}
	if err := counts.Validate(); err != nil {
		t.Fatalf("all-difficult allocation rejected: %v", err)
	}
	hardFeature := "管理员在审核台勾选多条待办后，系统先汇总每条的处理状态，再一次性更新审核台列表与发布详情；部分条目处理失败时保留失败明细并让成功项继续生效，避免整批结果丢失。"
	if err := ValidateFeatureOrBugFixDifficultyEvidence(hardFeature, TaskTypeFeature); err != nil {
		t.Fatalf("hard-standard Feature prompt rejected: %v", err)
	}

	// 内容单薄、没有真实联动也没有三项证据的条目仍然必须被拒绝。
	thinFeature := "管理员导入素材后，系统读取每行内容并校验数据格式，把可用记录写入列表。"
	if err := ValidateFeatureOrBugFixDifficultyEvidence(thinFeature, TaskTypeFeature); err == nil {
		t.Fatal("thin Feature prompt was accepted")
	}
}

func TestDocumentCountsValidateDocumentRejectsRepeatedPromptBodies(t *testing.T) {
	counts := DocumentCounts{CodeGen: 1, Feature: 1, Difficult: 2}
	document := strings.Join([]string{
		"**0-1代码生成**",
		"1. 【困难】会员提交预约之后，系统先预占名额再写入到店记录；并发确认时不能重复扣减名额，记录失败则回滚预占，列表和统计以已确认的预约结果为准。",
		"**Feature迭代**",
		"1. 【困难】会员提交预约之后，系统先预占名额再写入离店记录；并发确认时不能重复扣减名额，记录失败则回滚预占，列表和统计以已确认的预约结果为准。",
	}, "\n")

	err := counts.ValidateDocument(document)
	if err == nil || !strings.Contains(err.Error(), "重复比例") {
		t.Fatalf("ValidateDocument() error = %v, want repeated prompt rejection", err)
	}
}

func TestDocumentCountsValidateDocumentRejectsAbstractDifficultyEvidence(t *testing.T) {
	counts := DocumentCounts{Feature: 1, Difficult: 1}
	document := "**Feature迭代**\n1. 【困难】统一多个展示位置的输出转义，并兼容历史数据，保持现有页面风格和原有业务流程不变，不涉及新增入口、状态变化、保存行为或失败处理，只需要完成这项展示层调整即可。"

	err := counts.ValidateDocument(document)
	if err == nil || !strings.Contains(err.Error(), "难度证据不足") {
		t.Fatalf("ValidateDocument() error = %v, want abstract difficulty evidence rejection", err)
	}
}

func TestValidatePromptDifficultyEvidenceAcceptsConcreteMediumAndHardEvidence(t *testing.T) {
	medium := "用户修改记录后，数据先经过保存校验，再刷新列表和详情页；历史记录缺少新字段时按旧格式读取，保存失败则保留原内容并提示重试。"
	if err := ValidatePromptDifficultyEvidence(medium, TaskTypeFeature, "中等"); err != nil {
		t.Fatalf("medium evidence rejected: %v", err)
	}

	hard := "新增线索离线跟进能力，先保存本地沟通记录，联网后同步跟进计划；并发编辑同一线索时按版本合并无冲突记录，冲突项保留双方内容等待确认，不能覆盖服务端新版本，列表和详情最终一致。"
	if err := ValidatePromptDifficultyEvidence(hard, TaskTypeCodeGen, "困难"); err != nil {
		t.Fatalf("hard evidence rejected: %v", err)
	}
}

func TestValidatePromptDifficultyEvidenceRequiresAllThreeMediumCriteria(t *testing.T) {
	// 已有调用/数据流与实现决策，但缺少边界条件与约束，按新规则必须拒绝。
	missingBoundary := "用户点击低配资产的建议后，系统带着推荐的类别和小时数预填到入账表单并跳转到记录页，入账后建议和组合数据立即刷新；补仓完成后系统删掉对应的低配建议，而不是让用户照着旧提示再入账一次。"
	if err := ValidatePromptDifficultyEvidence(missingBoundary, TaskTypeFeature, "中等"); err == nil {
		t.Fatal("medium prompt without boundary evidence was accepted")
	} else if !strings.Contains(err.Error(), "边界条件与约束") {
		t.Fatalf("rejection did not name the missing criterion: %v", err)
	}

	// 只有链路和边界，没有实现决策，同样必须拒绝。
	missingDecision := "运营切换订单筛选之后，系统按新条件查询并刷新列表、汇总数量和导出结果；导入的历史订单字段不全时页面会跳过这批数据，列表下方展示一行说明。"
	if err := ValidatePromptDifficultyEvidence(missingDecision, TaskTypeBugFix, "中等"); err == nil {
		t.Fatal("medium prompt without decision evidence was accepted")
	} else if !strings.Contains(err.Error(), "实现决策") {
		t.Fatalf("rejection did not name the missing criterion: %v", err)
	}

	// 通过审核的中等题范例必须同时满足三项证据。
	if err := ValidatePromptDifficultyEvidence(MediumDifficultyReferencePrompt, TaskTypeFeature, "中等"); err != nil {
		t.Fatalf("approved medium reference prompt rejected: %v", err)
	}
}

// TestValidatePromptDifficultyEvidenceAcceptsNaturalXiaoyuanWording 锁定 c2c-002 文档
// 建卡时被误拦的五条写法：它们都具备三项证据，只是用了更自然的说法（保留置顶状态、
// 不能抹掉历史、按当前阶段重算、统一成本地日期口径、多档案各页取数一致）。
func TestValidatePromptDifficultyEvidenceAcceptsNaturalXiaoyuanWording(t *testing.T) {
	cases := []struct {
		name string
		kind string
		text string
	}{
		{
			name: "困难题用各页一致体现联动",
			kind: TaskTypeCodeGen,
			text: "工作里的焦虑和生活里的平静挤在同一个花园里，用户想把不同阶段的情绪分开打理。希望支持创建多个花园档案，每个档案有独立的记录、植物、连续天数和年报，首页顶部切换档案后，花园、时间轴、标本馆和年报这几个页面都从当前档案重新取数，新增记录只落到当前档案。首次打开时把已有的单花园数据自动迁入一个默认档案，不能丢任何一条旧记录；删除当前正在使用的档案时先自动切到另一个档案再删，剩下最后一个档案不允许删除，保证任何时刻都有可用的当前档案，切档案、删档案之后各页统计和顶部计数必须和当前档案一致。",
		},
		{
			name: "中等题用保留状态做决策",
			kind: TaskTypeFeature,
			text: "花园里的植物一直按固定顺序排，用户最在意的那株总被挤到后面。希望允许给植物排序和置顶，置顶的植物固定在花园最前面，其余按自定义顺序排，这个顺序要单独保存，切到标本馆、年报再切回来也不丢，重新计算植物后顺序同样保留；被置顶的植物枯萎后从花园消失，它的置顶状态要保留，等它复活后回到原来的置顶位置。排序只影响花园的展示，不能改变连续天数、阶段或年报的统计结果。",
		},
		{
			name: "中等题用保留归档做决策",
			kind: TaskTypeFeature,
			text: "标本馆里的枯萎植物只能摆着看，用户想重新捡起却只能去种植页再种一株新的，旧的那株就永远留在标本馆。希望支持对枯萎植物重新培育，从标本馆点进去直接跳到种植页并选好那类情绪，重新记录后这一株回到花园继续生长，连续天数从新的一轮开始累计，但它在标本馆留下的归档信息和曾经的生长天数要原样保留，不能因为复活就把历史抹掉。同一类情绪多次枯萎又复活时，每次生长经历都要各自留档，而不是只保留最近一次。",
		},
		{
			name: "修复题按阶段重算剩余天数",
			kind: TaskTypeBugFix,
			text: "花园卡片上那行下一阶段还需多少天的小字，一直按种子到发芽的三天来算，植物发芽之后就一直显示还需要零天，明明离开花和结果还远。需要修正为按当前所处阶段计算到下一阶段的真实剩余天数，种子、发芽、开花三个阶段分别对应三天、七天、十四天的门槛，已经结果的不再显示剩余天数；这行字还要和卡片上的阶段名称、进度条对得上，不能出现阶段写着开花、剩余天数却是零的矛盾。",
		},
		{
			name: "修复题统一本地日期口径",
			kind: TaskTypeBugFix,
			text: "在非零时区的环境里，用户当天早上的记录会被盖上前一天的日期，连续记录天数跟着少一天，发芽开花的时间往后拖，因为记录生成和连续计算用了世界统一日期，跟用户本地的一天对不上。需要把记录生成、连续计算和页面日期展示统一成同一套本地日期口径，旧数据里已经按旧口径存下的日期要能兼容读取，迁移后连续天数按正确日期重新算，页面上的日期也要和连续天数一致。",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidatePromptDifficultyEvidence(test.text, test.kind, "中等"); err != nil {
				t.Fatalf("natural wording rejected: %v", err)
			}
		})
	}
}
