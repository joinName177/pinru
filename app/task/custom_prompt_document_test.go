package task

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appgit "github.com/blueship581/pinru/app/git"
	"github.com/blueship581/pinru/app/testutil"
	"github.com/blueship581/pinru/internal/store"
)

func TestParseCustomPromptDocumentEntries(t *testing.T) {
	content := strings.Join([]string{
		"**0-1代码生成**",
		"",
		"1. 【简单】做一个学生入住登记入口，支持老师录入学生和床位关系。",
		"   需要保留异常提示。",
		"",
		"**Feature迭代**",
		"",
		"1. 在现有列表里增加入住状态筛选。",
		"",
		"**代码理解**",
		"",
		"- 【一般】梳理住宿状态从分配到退宿的流转。",
	}, "\n")

	entries := parseCustomPromptDocumentEntries(content)
	if len(entries) != 3 {
		t.Fatalf("entries len = %d, want 3: %+v", len(entries), entries)
	}
	if entries[0].TaskType != "0-1代码生成" || entries[0].PromptDifficulty != "简单" {
		t.Fatalf("entry[0] = %+v", entries[0])
	}
	if !strings.Contains(entries[0].PromptText, "需要保留异常提示") {
		t.Fatalf("entry[0].PromptText missing continuation: %q", entries[0].PromptText)
	}
	if entries[1].TaskType != "Feature迭代" || entries[1].PromptDifficulty != "中等" {
		t.Fatalf("entry[1] = %+v", entries[1])
	}
	if entries[2].TaskType != "代码理解" || entries[2].PromptDifficulty != "一般" {
		t.Fatalf("entry[2] = %+v", entries[2])
	}
}

func TestValidateCustomPromptDocumentBatchRules(t *testing.T) {
	entries := []customPromptEntry{
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "新增分批审核发布流程，管理员提交内容后先生成审核批次，确认后再发布；部分成功时选择保留成功项而不是整体回滚，失败项继续等待复核，重试只处理失败项，避免重复发布并保留每项的审核历史。"},
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "新增版本化发布模板流程，运营创建模板后先保存版本快照，草稿基于选定版本生成默认内容；模板与草稿并发编辑时不能覆盖草稿自有内容，已发布详情保留原版本，删除模板也不能破坏已有草稿的读取。"},
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "用户修改消息订阅偏好后，系统保存偏好并在发布节点查询对应规则，再把通知结果记录到消息历史；通知发送失败时保留待重试状态，重复回调不能生成两条相同记录。"},
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "新增异步素材审核流程，上传后先保存待审版本，再生成预览并提交审核；旧版本的审核回调晚到时不能覆盖新版本，撤销素材后回调不得重新发布，预览页与正式素材始终对应同一份已批准版本。"},
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "新增发布排期调度，管理员选定日期后先预占空档再保存发布安排；多人并发预占同一空档时只允许一个安排生效，保存失败必须释放预占，取消安排同步撤销待发通知，日历与发布列表以已确认安排为准。"},
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "新增申请审核与自动发布流程，提交后先保存申请版本并通知审核员，批准后才允许发布；审核和撤回并发发生时以最终确认版本为准，撤回后的异步回调不能再次发布，重试通知不能重复创建申请。"},
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "新增可恢复素材导入流程，上传后先保存批次进度再逐项校验入库；网络中断后从已确认位置恢复，同批次重复提交不能重复写入，选择保留已成功部分而不是整体回滚，失败明细可修正后单独重试。"},
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "新增发布复盘快照流程，选定时间范围后先冻结明细版本，再计算看板并生成异步导出；统计期间记录被删除时仍采用快照而不是混用实时数据，晚到的旧导出不能覆盖新筛选结果，失败重试必须复用同一版本。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "运营编辑发布内容后，系统先保存草稿再刷新编辑页；用户返回页面时读取最近一次草稿，保存失败则保留当前内容并提示重试，不能把旧草稿覆盖新输入。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "管理员选择处理人后，系统先查询审核列表再更新结果回显；批量处理失败时保留失败记录并显示原因，没有符合条件的记录时保留筛选条件并显示空态提示。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "用户从详情页返回列表时，系统带回之前的筛选条件并按条件重新查询；筛选结果为空时仍保留分页位置和高亮状态，刷新后不能显示上一轮列表。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "管理员打开发布记录后，系统读取失败原因并展示重试入口；记录刷新期间如果重试失败则保留原失败原因，重试成功后列表状态和详情结果同时更新。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "运营选择模板时，系统先读取最近使用记录再展示排序结果；默认模板已经失效时不能继续提交旧模板，页面应提示重新选择并保留其他已填写内容。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "审核员填写处理备注并提交结果后，系统保存备注再更新审核状态，列表摘要和详情页都读取新的记录；驳回或通过失败时保留上一步状态并允许重试。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "审核员撤回申请后，系统先更新申请状态再刷新列表、详情和历史记录；重新提交时改用新的状态，任一环节失败时保留原状态并提示具体原因。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "运营切换发布列表筛选后，系统先按新条件查询，再同步刷新统计、详情入口和导出结果；没有符合条件的记录时保留筛选条件并显示空态，刷新后不能回到上一轮统计口径。"},
		{TaskType: "代码理解", PromptDifficulty: "困难", PromptText: "梳理发布流程从填写到提交完成的关键状态流，并生成 README 文档。"},
	}
	if err := validateCustomPromptDocumentBatch(entries); err != nil {
		t.Fatalf("validateCustomPromptDocumentBatch() error = %v", err)
	}

	withoutReadme := append([]customPromptEntry(nil), entries...)
	withoutReadme[len(withoutReadme)-1].PromptText = "梳理发布流程从填写到提交完成的关键状态流。"
	if err := validateCustomPromptDocumentBatch(withoutReadme); err == nil || !strings.Contains(err.Error(), "README") {
		t.Fatalf("validate without README error = %v, want README error", err)
	}

	validDifficulty := append([]customPromptEntry(nil), entries...)
	validDifficulty[0].PromptDifficulty = "地狱"
	if err := validateCustomPromptDocumentBatch(validDifficulty); err != nil {
		t.Fatalf("validate real difficulty error = %v", err)
	}

	mediumCodeGen := append([]customPromptEntry(nil), entries...)
	mediumCodeGen[0].PromptDifficulty = "中等"
	if err := validateCustomPromptDocumentBatch(mediumCodeGen); err == nil || !strings.Contains(err.Error(), "0-1代码生成题必须使用困难或地狱") {
		t.Fatalf("validate medium codegen error = %v, want codegen difficulty error", err)
	}

	tooEasy := append([]customPromptEntry(nil), entries...)
	tooEasy[0].PromptDifficulty = "一般"
	if err := validateCustomPromptDocumentBatch(tooEasy); err == nil || !strings.Contains(err.Error(), "低于中等下限") {
		t.Fatalf("validate easy difficulty error = %v, want difficulty floor error", err)
	}

	unknownDifficulty := append([]customPromptEntry(nil), entries...)
	unknownDifficulty[0].PromptDifficulty = "超难"
	if err := validateCustomPromptDocumentBatch(unknownDifficulty); err == nil || !strings.Contains(err.Error(), "难度不支持") {
		t.Fatalf("validate unknown difficulty error = %v, want unsupported difficulty error", err)
	}

	tooFew := entries[:10]
	if err := validateCustomPromptDocumentBatch(tooFew); err != nil {
		t.Fatalf("custom counts rejected: %v", err)
	}
}

func TestValidateCustomPromptDocumentBatchAcceptsHardStandardFeatureAndBugFix(t *testing.T) {
	// Feature迭代和 Bug修复 的题面按困难标准撰写，标签可以是困难；地狱仍然不接收。
	hardEntries := []customPromptEntry{
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "管理员在审核台勾选多条待办后，系统先汇总每条的处理状态，再一次性更新审核台列表与发布详情；部分条目处理失败时保留失败明细并让成功项继续生效，避免整批结果丢失。"},
		{TaskType: "Bug修复", PromptDifficulty: "困难", PromptText: "一笔超过周预算的小时数入账后，页面出现互相矛盾的数字，剩余被强行显示成零，已配置却超过可配置，健康度分数也能超过十分。这是记账时没有按剩余预算做上限拦截导致的，需要在入账那一刻就校验这笔小时数是否超过剩余，超了要明确提示差额并阻止入账或让用户确认。同时要让剩余、已配置和健康度在任何情况下都保持一致，不能再出现已配置大于可配置的账。"},
	}
	if err := validateCustomPromptDocumentBatch(hardEntries); err != nil {
		t.Fatalf("hard-standard Feature/BugFix rejected: %v", err)
	}

	hellEntries := []customPromptEntry{
		{TaskType: "Feature迭代", PromptDifficulty: "地狱", PromptText: hardEntries[0].PromptText},
	}
	if err := validateCustomPromptDocumentBatch(hellEntries); err == nil {
		t.Fatal("Feature迭代 with 地狱 label was accepted")
	}
}

func TestValidateCustomPromptDocumentBatchReportsEveryInvalidItem(t *testing.T) {
	entries := []customPromptEntry{
		{
			TaskType:         "Feature迭代",
			PromptDifficulty: "困难",
			PromptText:       "新增筛选入口。",
		},
		{
			TaskType:         "0-1代码生成",
			PromptDifficulty: "中等",
			PromptText:       "新增一个导出能力，让用户把数据下载到本地。",
		},
		{
			TaskType:         "代码理解",
			PromptDifficulty: "困难",
			PromptText:       "梳理住宿状态从分配到退宿的流转过程。",
		},
	}

	err := validateCustomPromptDocumentBatch(entries)
	if err == nil {
		t.Fatal("validateCustomPromptDocumentBatch() accepted an invalid document")
	}
	message := err.Error()
	for _, want := range []string{"第 1 条", "第 2 条", "第 3 条", "有效字符", "必须使用困难或地狱难度", "README"} {
		if !strings.Contains(message, want) {
			t.Fatalf("aggregated validation error missing %q: %v", want, err)
		}
	}
}

func TestCustomPromptDocumentBatchPreservesDifficulties(t *testing.T) {
	entries := []customPromptEntry{
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "新增分批审核发布流程，管理员提交内容后先生成审核批次，确认后再发布；部分成功时选择保留成功项而不是整体回滚，失败项继续等待复核，重试只处理失败项，避免重复发布并保留每项的审核历史。"},
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "新增版本化发布模板流程，运营创建模板后先保存版本快照，草稿基于选定版本生成默认内容；模板与草稿并发编辑时不能覆盖草稿自有内容，已发布详情保留原版本，删除模板也不能破坏已有草稿的读取。"},
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "用户修改消息订阅偏好后，系统保存偏好并在发布节点查询对应规则，再把通知结果记录到消息历史；通知发送失败时保留待重试状态，重复回调不能生成两条相同记录。"},
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "新增异步素材审核流程，上传后先保存待审版本，再生成预览并提交审核；旧版本的审核回调晚到时不能覆盖新版本，撤销素材后回调不得重新发布，预览页与正式素材始终对应同一份已批准版本。"},
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "新增发布排期调度，管理员选定日期后先预占空档再保存发布安排；多人并发预占同一空档时只允许一个安排生效，保存失败必须释放预占，取消安排同步撤销待发通知，日历与发布列表以已确认安排为准。"},
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "新增申请审核与自动发布流程，提交后先保存申请版本并通知审核员，批准后才允许发布；审核和撤回并发发生时以最终确认版本为准，撤回后的异步回调不能再次发布，重试通知不能重复创建申请。"},
		{TaskType: "0-1代码生成", PromptDifficulty: "困难", PromptText: "新增可恢复素材导入流程，上传后先保存批次进度再逐项校验入库；网络中断后从已确认位置恢复，同批次重复提交不能重复写入，选择保留已成功部分而不是整体回滚，失败明细可修正后单独重试。"},
		{TaskType: "0-1代码生成", PromptDifficulty: "地狱", PromptText: "新增发布复盘快照流程，选定时间范围后先冻结明细版本，再计算看板并生成异步导出；统计期间记录被删除时仍采用快照而不是混用实时数据，晚到的旧导出不能覆盖新筛选结果，失败重试必须复用同一版本。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "运营编辑发布内容后，系统先保存草稿再刷新编辑页；用户返回页面时读取最近一次草稿，保存失败则保留当前内容并提示重试，不能把旧草稿覆盖新输入。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "管理员选择处理人后，系统先查询审核列表再更新结果回显；批量处理失败时保留失败记录并显示原因，没有符合条件的记录时保留筛选条件并显示空态提示。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "用户从详情页返回列表时，系统带回之前的筛选条件并按条件重新查询；筛选结果为空时仍保留分页位置和高亮状态，刷新后不能显示上一轮列表。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "管理员打开发布记录后，系统读取失败原因并展示重试入口；记录刷新期间如果重试失败则保留原失败原因，重试成功后列表状态和详情结果同时更新。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "运营选择模板时，系统先读取最近使用记录再展示排序结果；默认模板已经失效时不能继续提交旧模板，页面应提示重新选择并保留其他已填写内容。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "审核员填写处理备注并提交结果后，系统保存备注再更新审核状态，列表摘要和详情页都读取新的记录；驳回或通过失败时保留上一步状态并允许重试。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "审核员撤回申请后，系统先更新申请状态再刷新列表、详情和历史记录；重新提交时改用新的状态，任一环节失败时保留原状态并提示具体原因。"},
		{TaskType: "Feature迭代", PromptDifficulty: "困难", PromptText: "运营切换发布列表筛选后，系统先按新条件查询，再同步刷新统计、详情入口和导出结果；没有符合条件的记录时保留筛选条件并显示空态，刷新后不能回到上一轮统计口径。"},
		{TaskType: "代码理解", PromptDifficulty: "困难", PromptText: "梳理发布流程从填写到提交完成的关键状态流，并生成 README 文档。"},
	}

	if err := validateCustomPromptDocumentBatch(entries); err != nil {
		t.Fatalf("validateCustomPromptDocumentBatch() error = %v", err)
	}
	counts := map[string]int{}
	for _, entry := range entries {
		if entry.TaskType != "代码理解" {
			counts[entry.PromptDifficulty]++
		}
	}
	if counts["地狱"] != 1 || counts["困难"] != 15 || counts["中等"] != 0 {
		t.Fatalf("difficulty counts = %+v, want preserved labels", counts)
	}
}

func TestParseCustomPromptDocumentKeepsUnknownDifficultyForValidation(t *testing.T) {
	entries := parseCustomPromptDocumentEntries("**Feature迭代**\n1. 【超难】扩展已有流程")
	if len(entries) != 1 || entries[0].PromptDifficulty != "超难" {
		t.Fatalf("entries = %+v, want unknown label preserved", entries)
	}
	if err := validateCustomPromptDocumentBatch(entries); err == nil || !strings.Contains(err.Error(), "难度不支持") {
		t.Fatalf("validate error = %v, want unsupported difficulty", err)
	}
}

func TestParseCustomPromptDocumentEntriesAcceptsSpacedHeading(t *testing.T) {
	entries := parseCustomPromptDocumentEntries("## Bug 修复\n1. 【困难】统一预览和最终创建的相似度结论。")
	if len(entries) != 1 {
		t.Fatalf("entries = %+v, want one entry", entries)
	}
	if entries[0].TaskType != "Bug修复" {
		t.Fatalf("task type = %q, want Bug修复", entries[0].TaskType)
	}
}

func TestParseCustomPromptDocumentEntriesNormalizesDifficultyAliases(t *testing.T) {
	entries := parseCustomPromptDocumentEntries("## Bug 修复\n1. 【地狱级】第一条提示词\n2. 【困难】【困难】重复标签应从正文移除")
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want two entries", entries)
	}
	if entries[0].PromptDifficulty != "地狱" {
		t.Fatalf("difficulty = %q, want 地狱", entries[0].PromptDifficulty)
	}
	if entries[1].PromptText != "重复标签应从正文移除" {
		t.Fatalf("prompt text = %q, want duplicate marker removed", entries[1].PromptText)
	}
}

func TestInferCustomPromptDocumentProjectNameAcceptsManualPromptFilename(t *testing.T) {
	got := inferCustomPromptDocumentProjectName("/tmp/zqq-06_困难bug修复提示词.md")
	if got != "zqq-06" {
		t.Fatalf("project name = %q, want zqq-06", got)
	}
}

func TestDiscoverCustomPromptSourceItemUsesConfiguredRoot(t *testing.T) {
	testStore := testutil.OpenTestStore(t)
	defer testStore.Close()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "zqq-06"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := testStore.SetConfig("custom_project_root_path", root); err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}
	project := store.Project{ID: "project-custom-source", Name: "zqq-06"}
	if err := testStore.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	svc := New(testStore, appgit.New(testStore))
	item, ok := svc.discoverCustomPromptSourceItem(project, "zqq-06")
	if !ok || item.DisplayName != "zqq-06" || item.SourcePath != filepath.Join(root, "zqq-06") {
		t.Fatalf("fallback item = %+v, ok=%v", item, ok)
	}
}

func TestCustomPromptDocumentLimitsOnlyFixedTypes(t *testing.T) {
	for _, kind := range []string{"代码理解", "工程化", "代码测试", "代码重构", "Bug修复", "Feature迭代", "0-1代码生成"} {
		t.Run(kind, func(t *testing.T) {
			difficulty := "困难"
			if kind == "Bug修复" || kind == "Feature迭代" {
				difficulty = "困难"
			}
			promptText := "真实需求并生成 README"
			if kind == "Bug修复" || kind == "Feature迭代" {
				promptText = "用户提交记录后，系统先保存并刷新列表和详情；保存失败时保留原内容并提示重试，避免页面继续显示旧状态，刷新后仍应保留本次操作结果。"
			} else if kind == "0-1代码生成" {
				promptText = "用户提交内容后，系统先保存记录，再在列表和详情展示处理结果；失败时保留草稿并允许重试，重复提交不能重复写入记录，处理完成后还要更新通知状态。"
			}
			entry := customPromptEntry{TaskType: kind, PromptDifficulty: difficulty, PromptText: promptText}
			err := validateCustomPromptDocumentBatch([]customPromptEntry{entry, entry})
			fixed := kind == "代码理解" || kind == "工程化" || kind == "代码测试" || kind == "代码重构"
			if (err != nil) != fixed {
				t.Fatalf("duplicate type %s: %v", kind, err)
			}
		})
	}
}

func TestCreateTasksFromCustomPromptDocumentsCreatesTasksAndPromptArtifacts(t *testing.T) {
	testStore := testutil.OpenTestStore(t)
	defer testStore.Close()

	cloneBase := t.TempDir()
	project := store.Project{
		ID:                "project-custom-doc-task",
		Name:              "Custom Doc",
		CloneBasePath:     cloneBase,
		Models:            "ORIGIN,model-a",
		SourceModelFolder: "ORIGIN",
	}
	if err := testStore.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	sourcePath := filepath.Join(t.TempDir(), "zw-001-source")
	if err := os.MkdirAll(filepath.Join(sourcePath, "src"), 0o755); err != nil {
		t.Fatalf("MkdirAll(sourcePath) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "src", "main.ts"), []byte("export const value = 1;\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	if err := testStore.UpsertQuestionBankItem(store.QuestionBankItem{
		ProjectConfigID: project.ID,
		QuestionID:      1001,
		DisplayName:     "zw-001",
		SourceKind:      "local_directory",
		SourcePath:      sourcePath,
		OriginRef:       "custom:zw-001",
		Status:          "ready",
	}); err != nil {
		t.Fatalf("UpsertQuestionBankItem() error = %v", err)
	}

	docPath := filepath.Join(t.TempDir(), "zw-001_提示词_0602.md")
	docContent := strings.Join([]string{
		"**0-1代码生成**",
		"",
		"1. 【困难】新增分批审核发布流程，管理员提交内容后先生成审核批次，确认后再发布；部分成功时选择保留成功项而不是整体回滚，失败项继续等待复核，重试只处理失败项，避免重复发布并保留每项的审核历史。",
		"2. 【困难】新增版本化发布模板流程，运营创建模板后先保存版本快照，草稿基于选定版本生成默认内容；模板与草稿并发编辑时不能覆盖草稿自有内容，已发布详情保留原版本，删除模板也不能破坏已有草稿的读取。",
		"3. 【困难】用户修改消息订阅偏好后，系统保存偏好并在发布节点查询对应规则，再把通知结果记录到消息历史；通知发送失败时保留待重试状态，重复回调不能生成两条相同记录。",
		"4. 【地狱】新增异步素材审核流程，上传后先保存待审版本，再生成预览并提交审核；旧版本的审核回调晚到时不能覆盖新版本，撤销素材后回调不得重新发布，预览页与正式素材始终对应同一份已批准版本。",
		"5. 【困难】新增发布排期调度，管理员选定日期后先预占空档再保存发布安排；多人并发预占同一空档时只允许一个安排生效，保存失败必须释放预占，取消安排同步撤销待发通知，日历与发布列表以已确认安排为准。",
		"6. 【困难】新增申请审核与自动发布流程，提交后先保存申请版本并通知审核员，批准后才允许发布；审核和撤回并发发生时以最终确认版本为准，撤回后的异步回调不能再次发布，重试通知不能重复创建申请。",
		"7. 【困难】新增可恢复素材导入流程，上传后先保存批次进度再逐项校验入库；网络中断后从已确认位置恢复，同批次重复提交不能重复写入，选择保留已成功部分而不是整体回滚，失败明细可修正后单独重试。",
		"8. 【困难】新增发布复盘快照流程，选定时间范围后先冻结明细版本，再计算看板并生成异步导出；统计期间记录被删除时仍采用快照而不是混用实时数据，晚到的旧导出不能覆盖新筛选结果，失败重试必须复用同一版本。",
		"",
		"**Feature迭代**",
		"",
		"1. 【困难】运营编辑发布内容后，系统先保存草稿再刷新编辑页；用户返回页面时读取最近一次编辑结果，保存失败时保留当前内容并提示重试，不能把旧草稿覆盖新输入。",
		"2. 【困难】管理员选择处理人后，系统先查询审核列表再更新结果回显；批量处理失败时保留失败记录并显示原因，没有符合条件的记录时保留筛选条件并显示空态提示。",
		"3. 【困难】用户从详情页返回列表时，系统先带回之前的筛选条件再按条件重新查询并刷新列表；筛选结果为空时保留分页位置和高亮状态，刷新后不能显示上一轮列表。",
		"4. 【困难】管理员打开发布记录后，系统读取失败原因并展示重试入口；刷新期间如果重试失败就保留原失败原因，重试成功后列表状态和详情结果同时更新。",
		"5. 【困难】运营选择模板时，系统先读取最近使用记录再展示排序结果；默认模板已经失效时不能继续提交旧模板，页面应提示重新选择并保留其他已填写内容。",
		"6. 【困难】审核员填写处理备注并提交结果后，系统保存备注再更新审核状态，列表摘要和详情页都读取新的记录；驳回或通过失败时保留上一步状态并允许重试。",
		"7. 【困难】审核员撤回申请后，系统先更新申请状态再刷新列表、详情和历史记录；重新提交时改用新的状态，任一环节失败时保留原状态并提示具体原因。",
		"8. 【困难】运营切换发布列表筛选后，系统先按新条件查询，再同步刷新统计、详情入口和导出结果；没有符合条件的记录时保留筛选条件并显示空态，刷新后不能回到上一轮统计口径。",
		"",
		"**代码理解**",
		"",
		"1. 【困难】梳理发布流程从填写到提交完成的关键状态流，并生成 README 文档。",
	}, "\n")
	if err := os.WriteFile(docPath, []byte(docContent), 0o644); err != nil {
		t.Fatalf("WriteFile(doc) error = %v", err)
	}

	svc := New(testStore, appgit.New(testStore))
	result, err := svc.CreateTasksFromCustomPromptDocuments(CreateTasksFromCustomPromptDocumentsRequest{
		ProjectID:     project.ID,
		DocumentPaths: []string{docPath},
	})
	if err != nil {
		t.Fatalf("CreateTasksFromCustomPromptDocuments() error = %v", err)
	}
	if result.CreatedCount != 17 || result.ErrorCount != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.Details) != 1 || result.Details[0].ParsedCount != 17 {
		t.Fatalf("unexpected detail: %+v", result.Details)
	}

	tasks, err := testStore.ListTasks(&project.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 17 {
		t.Fatalf("tasks len = %d, want 17", len(tasks))
	}

	seenTypes := map[string]int{}
	seenDifficulties := map[string]int{}
	for _, task := range tasks {
		seenTypes[task.TaskType]++
		if task.TaskType == "代码理解" {
			if task.PromptDifficulty != "困难" {
				t.Fatalf("code understanding difficulty = %q, want 困难", task.PromptDifficulty)
			}
		} else {
			seenDifficulties[task.PromptDifficulty]++
		}
		if task.Status != "PromptReady" {
			t.Fatalf("task %s status = %q, want PromptReady", task.ID, task.Status)
		}
		if task.PromptText == nil || strings.TrimSpace(*task.PromptText) == "" {
			t.Fatalf("task %s prompt missing", task.ID)
		}
		if task.LocalPath == nil {
			t.Fatalf("task %s local path nil", task.ID)
		}
		artifactPath := filepath.Join(*task.LocalPath, "任务提示词.md")
		artifact, err := os.ReadFile(artifactPath)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", artifactPath, err)
		}
		if strings.TrimSpace(string(artifact)) != strings.TrimSpace(*task.PromptText) {
			t.Fatalf("artifact mismatch for %s", task.ID)
		}
		sourceFolderName := filepath.Base(*task.LocalPath)
		annotationCase, err := testStore.GetAnnotationCase(task.ID)
		if err != nil || annotationCase == nil || len(annotationCase.InitialSHA) != 40 {
			t.Fatalf("task %s must have a registered pre-turn snapshot: %+v %v", task.ID, annotationCase, err)
		}
		if _, err := os.Stat(filepath.Join(*task.LocalPath, sourceFolderName, "src", "main.ts")); err != nil {
			t.Fatalf("source copy missing for %s: %v", task.ID, err)
		}
		if _, err := os.Stat(filepath.Join(*task.LocalPath, "model-a", "src", "main.ts")); err != nil {
			if !os.IsNotExist(err) {
				t.Fatalf("unexpected model copy stat error for %s: %v", task.ID, err)
			}
		} else {
			t.Fatalf("model copy should not exist for custom prompt task %s", task.ID)
		}
	}
	if seenTypes["0-1代码生成"] != 8 || seenTypes["Feature迭代"] != 8 || seenTypes["代码理解"] != 1 {
		t.Fatalf("seenTypes = %+v", seenTypes)
	}
	if seenDifficulties["地狱"] != 1 || seenDifficulties["困难"] != 15 || seenDifficulties["中等"] != 0 {
		t.Fatalf("seenDifficulties = %+v", seenDifficulties)
	}
}

func TestCreateTasksFromCustomPromptDocumentsPreparesPortableSource(t *testing.T) {
	testStore := testutil.OpenTestStore(t)
	defer testStore.Close()

	cloneBase := t.TempDir()
	project := store.Project{
		ID:                "project-custom-doc-node-modules",
		Name:              "Custom Doc Node",
		CloneBasePath:     cloneBase,
		Models:            "ORIGIN",
		SourceModelFolder: "ORIGIN",
	}
	if err := testStore.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	sourcePath := filepath.Join(t.TempDir(), "zw-node-source")
	if err := os.MkdirAll(filepath.Join(sourcePath, "node_modules", "left-pad"), 0o755); err != nil {
		t.Fatalf("MkdirAll(node_modules) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(package.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "node_modules", "left-pad", "index.js"), []byte("module.exports = function(){};\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(node module) error = %v", err)
	}
	if err := testStore.UpsertQuestionBankItem(store.QuestionBankItem{
		ProjectConfigID: project.ID,
		QuestionID:      1002,
		DisplayName:     "zw-node",
		SourceKind:      "local_directory",
		SourcePath:      sourcePath,
		OriginRef:       "custom:zw-node",
		Status:          "ready",
	}); err != nil {
		t.Fatalf("UpsertQuestionBankItem() error = %v", err)
	}

	svc := New(testStore, appgit.New(testStore))
	taskPath := filepath.Join(cloneBase, "zw-node-0-1代码生成-1")
	taskDetail := svc.CreateCustomPromptTaskFromPayload(context.Background(), CustomPromptTaskJobPayload{
		ProjectID:        project.ID,
		QuestionID:       1002,
		ProjectName:      "zw-node",
		SourcePath:       sourcePath,
		TargetSourcePath: filepath.Join(taskPath, filepath.Base(taskPath)),
		TaskType:         "0-1代码生成",
		PromptDifficulty: "困难",
		PromptText:       "新增一个前端调试入口，方便快速查看当前页面运行状态。",
		ClaimSequence:    1,
		LocalPath:        taskPath,
		SourceModelName:  "ORIGIN",
	})
	if taskDetail.Status != "created" {
		t.Fatalf("CreateCustomPromptTaskFromPayload() = %+v", taskDetail)
	}

	tasks, err := testStore.ListTasks(&project.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].LocalPath == nil {
		t.Fatalf("tasks = %+v", tasks)
	}
	sourceFolderName := filepath.Base(*tasks[0].LocalPath)
	if _, err := os.Stat(filepath.Join(*tasks[0].LocalPath, sourceFolderName, "node_modules", "left-pad", "index.js")); !os.IsNotExist(err) {
		t.Fatalf("host node_modules must not enter container source: %v", err)
	}
}

// The import path must not reject domain-specific requirements just because
// their vocabulary is absent from the generation heuristic.
func TestCreateTasksFromCustomPromptDocumentsPackingRegression(t *testing.T) {
	db := testutil.OpenTestStore(t)
	root := t.TempDir()
	source := filepath.Join(root, "sources", "c2c-004")
	if err := os.MkdirAll(filepath.Join(source, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	sourceBody := []byte("export const packing = true;\n")
	if err := os.WriteFile(filepath.Join(source, "src", "main.ts"), sourceBody, 0o644); err != nil {
		t.Fatal(err)
	}
	project := store.Project{ID: "packing-import", Name: "c2c-004", CloneBasePath: filepath.Join(root, "tasks"), Models: "ORIGIN", SourceModelFolder: "ORIGIN"}
	if err := db.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	if err := db.SetConfig("custom_project_root_path", filepath.Dir(source)); err != nil {
		t.Fatal(err)
	}
	svc := New(db, appgit.New(db))
	result, err := svc.CreateTasksFromCustomPromptDocuments(CreateTasksFromCustomPromptDocumentsRequest{ProjectID: project.ID, DocumentPaths: []string{"testdata/c2c-004_提示词_1005.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.CreatedCount != 22 || result.ErrorCount != 0 {
		t.Fatalf("want 22 created, 0 errors: %+v", result)
	}
	tasks, err := db.ListTasks(&project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 22 {
		t.Fatalf("tasks=%d", len(tasks))
	}
	counts := map[string]int{}
	seenPrompts := map[string]bool{}
	for _, task := range tasks {
		counts[task.TaskType]++
		if task.Status != "PromptReady" || task.PromptDifficulty != "困难" || task.PromptText == nil || task.LocalPath == nil {
			t.Fatalf("incomplete task: %+v", task)
		}
		if seenPrompts[*task.PromptText] {
			t.Fatal("duplicate prompt")
		}
		seenPrompts[*task.PromptText] = true
		artifact, err := os.ReadFile(filepath.Join(*task.LocalPath, "任务提示词.md"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(artifact)) != strings.TrimSpace(*task.PromptText) {
			t.Fatal("prompt artifact mismatch")
		}
		copied, err := os.ReadFile(filepath.Join(*task.LocalPath, filepath.Base(*task.LocalPath), "src", "main.ts"))
		if err != nil {
			t.Fatal(err)
		}
		if string(copied) != string(sourceBody) {
			t.Fatal("source copy mismatch")
		}
		snapshot, err := db.GetAnnotationCase(task.ID)
		if err != nil || snapshot == nil || len(snapshot.InitialSHA) != 40 {
			t.Fatalf("initial snapshot missing: %v", err)
		}
	}
	for kind, want := range map[string]int{"0-1代码生成": 8, "Feature迭代": 12, "Bug修复": 2} {
		if counts[kind] != want {
			t.Fatalf("%s=%d, want %d", kind, counts[kind], want)
		}
	}
}
