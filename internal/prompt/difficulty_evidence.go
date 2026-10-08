package prompt

import (
	"fmt"
	"strings"
)

// ValidatePromptDifficultyEvidence checks the part of the difficulty contract
// that can be verified from the generated text itself. It is intentionally
// conservative: a label such as "兼容旧数据" is not evidence unless the text
// also says when it happens and what the user or system should observe.
//
// Feature/Bug 按 G19 检查三项证据，展示标签不改变门槛；0-1 按 G16 检查困难证据。
// 关键词校验仅做初筛，不能证明真实架构复杂度；仍需结合源码审题。
func ValidatePromptDifficultyEvidence(promptText, taskType, difficulty string) error {
	kind := NormalizeTaskType(taskType)
	level := strings.TrimSpace(difficulty)
	if kind != TaskTypeFeature && kind != TaskTypeBugFix && kind != TaskTypeCodeGen {
		return nil
	}
	if level != "中等" && level != "困难" && level != "地狱" {
		return nil
	}

	text := strings.TrimSpace(promptText)
	if kind == TaskTypeFeature || kind == TaskTypeBugFix {
		return validateMediumDifficultyEvidence(text, kind)
	}
	return validateHardDifficultyEvidence(text, kind)
}

// validateHardDifficultyEvidence 按审核口径（GSB G16）判定困难/地狱题：题面里必须能
// 看见多模块整合、关键设计取舍或复杂技术关注点中的至少一项，否则真实难度会被判成中等。
func validateHardDifficultyEvidence(text, kind string) error {
	if hasTradeoffEvidence(text) || hasComplexTechnicalEvidence(text) || hasStrictIntegrationEvidence(text) {
		return nil
	}
	return fmt.Errorf("%s题的题面达不到困难标准：看不出多模块整合、关键设计取舍或复杂技术关注点。该候选作废，请重新选题并写清两个以上业务环节如何汇合以及冲突时以哪一处为准、两个可行方案之间的取舍及后果，或并发/离线/版本兼容/旧数据迁移/异常恢复等复杂触发过程与必须保证的结果；只写“新增入口 + 边界提示”会被判成中等，调高难度字段无效", kind)
}

// validateMediumDifficultyEvidence 逐项检查中等题的三项硬性证据。三项必须同时
// 成立：缺调用/数据流说明链路不完整，缺实现决策说明只是罗列改动，缺边界条件说明
// 没有真实触发场景。只写统一展示、兼容旧数据这类抽象结论依然不算命中。
func validateMediumDifficultyEvidence(text, kind string) error {
	var missing []string
	if !hasFlowEvidence(text) {
		missing = append(missing, "调用/数据流理解（输入或动作经过的业务环节及最终影响）")
	}
	if !hasDecisionEvidence(text) {
		missing = append(missing, "实现决策（可选的处理策略及其正常、失败或冲突结果）")
	}
	if !hasBoundaryEvidence(text) {
		missing = append(missing, "边界条件与约束（具体触发条件、边界输入或旧数据及预期行为）")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%s题的三项中等难度证据必须全部具备，当前缺少：%s；只写统一展示、兼容旧数据等抽象结论不算命中，三项缺一项都不行，候选作废并重新选题，不能仅改措辞或调高标签", kind, strings.Join(missing, "；"))
}

// ValidateFeatureOrBugFixDifficultyEvidence checks all three G19 evidence shapes,
// independently of the displayed difficulty label.
func ValidateFeatureOrBugFixDifficultyEvidence(promptText, taskType string) error {
	return validateMediumDifficultyEvidence(strings.TrimSpace(promptText), NormalizeTaskType(taskType))
}

// hasFlowEvidence 识别调用/数据流理解的最低形状：输入或动作 → 经过的业务环节 →
// 最终影响。用词偏业务而不是实现，既能让生成的提示词保持自然，也能拦住只有一句
// 目标的空壳题目。
func hasFlowEvidence(text string) bool {
	return containsAny(text,
		"勾选", "提交", "修改", "保存", "点击", "切换", "上传", "导入", "导出", "下单", "取消", "删除", "退款", "支付", "筛选", "筛选到", "刷新", "登录", "进入", "选择", "发送", "调整", "改期", "录入", "确认", "核销", "查看", "点开", "补充", "新增", "修复", "增加", "扩展", "创建", "复制", "编辑", "跳转", "入账", "补录", "补标", "标记", "重试", "撤回", "驳回", "撤销", "打包", "套用", "清除", "提醒", "预警", "校验", "核对", "回滚", "恢复", "预览", "进入", "执行", "参与", "识别", "比对", "判断", "引入", "修正", "统一", "迁移", "重排", "排序", "置顶", "复活", "留档", "兼容",
	) && containsAny(text,
		"读取", "查询", "保存", "写入", "更新", "刷新", "同步", "校验", "计算", "生成", "扣减", "释放", "通知", "回退", "恢复", "合并", "分配", "汇总", "导出", "下载", "缓存", "记录", "处理", "转换", "过滤", "统计", "排序", "重算", "补齐", "回填", "截断", "修复", "聚合", "归集", "归类", "计入", "换算", "打包", "计划", "额度", "预算", "指标", "区间", "对应", "覆盖", "追溯",
	) && containsAny(text,
		"显示", "展示", "看到", "列表", "详情", "页面", "结果", "状态", "数据", "记录", "提示", "生效", "可用", "不可用", "保留", "回滚", "返回", "数量", "视图", "总笔数", "剩余额度", "空状态", "空提示", "高亮", "预警", "说明", "偏低", "偏高", "占比", "卡片", "区间", "建议", "明细", "标签", "汇总", "名次", "进度", "差额", "提示语", "档案", "门槛", "小字", "备注",
	) && containsAny(text, "先", "再", "之后", "然后", "随后", "经过", "时", "后", "按", "提交后", "刷新后", "切换后", "保存后", "创建后", "上传后", "修改后", "选择后", "进入后", "删除后", "改期后", "筛选后", "点开后", "保存成功后", "才能", "才", "导致", "同时", "一并", "一起", "自动")
}

// hasDecisionEvidence 要求出现明确的处理选择及其后果。写法有两种自然形态：
// 一是显式的取舍表述（而不是、不再、改为、优先、保持不变、各用各的等），
// 二是条件触发下的处理动作（重复时保留首次、失败时回退到数据库、缺字段时补齐默认值）。
// 只写“需要选择策略”这类空话仍不达标。
func hasDecisionEvidence(text string) bool {
	tradeoff := containsAny(text,
		"而不是", "否则", "与其", "宁可", "不再", "不回", "改为", "优先", "只保留", "只提示", "只隔离", "只允许", "只生成", "保持不变", "各用各的", "不回退", "不回滚", "取默认值", "沿用", "按首次", "按最后一次", "合并后再", "不再共用", "不再重复", "为准",
	)
	condition := containsAny(text,
		"只能", "如果", "若", "当", "一旦", "没有", "无法", "缺", "不存在", "为空", "冲突", "重复", "失败", "异常", "损坏", "超时", "部分成功", "应", "需", "但", "却", "全部", "并列", "相同", "都", "不能", "不会", "不得",
	)
	action := containsAny(text,
		"保留", "提示", "回退", "回滚", "跳过", "补零", "清零", "重置", "降级", "兜底", "拒绝", "隔离", "补齐", "沿用", "不改变", "不影响", "不重复", "不累计", "不再", "修复", "恢复", "排序", "重算", "统一", "继续", "允许", "改为", "退回", "转为", "标记", "写入", "改成", "归入", "释放", "识别", "给出", "生成", "展示", "提示语",
	)
	return tradeoff || (condition && action)
}

// hasBoundaryEvidence 要求给出具体触发条件与预期行为。兼容性口号本身故意不算数，
// 但“连续记录超过五笔”“旧的本地数据被改坏”“已经补过仓”这类真实触发点都算。
func hasBoundaryEvidence(text string) bool {
	trigger := containsAny(text,
		"如果", "若", "当", "一旦", "缺少", "没有符合", "没有记录", "没有数据", "为空", "重复提交", "重复请求", "失败时", "失败后", "刷新失败", "缓存失败", "任一环节失败", "并发", "并发提交", "并发时", "超时后", "离线时", "无权限时", "不存在时", "超过", "不足", "取消后", "删除后", "刷新后", "返回后", "处理之后", "操作后", "筛选时", "重试", "批量", "重复", "失败", "异常", "空结果", "旧数据", "旧版", "已经", "连续", "快速点击", "同时", "缺失", "损坏", "没有", "还没", "有的", "一部分", "类型不对", "缺字段", "无法识别", "对不上", "不一致", "不同", "重复打开", "再次", "上一次", "上一轮", "只能", "但", "却", "都", "并列", "全部",
	)
	expected := containsAny(text,
		"则", "必须", "不能", "不得", "保留", "提示", "回退", "回滚", "继续", "拒绝", "只允许", "只改", "只做", "应当", "应该", "避免", "生效", "显示", "展示", "恢复", "隔离", "默认", "不回", "不再", "不要", "不影响", "不改变", "保持不变", "同步", "一致", "补齐", "继续保留", "立即刷新", "立即重算", "不重复", "不累计", "补零", "跳过", "对得上", "正常", "如实", "原值", "空状态", "空提示", "成功", "不丢", "不重复", "按所选", "按实际", "完整", "同时更新", "一起更新", "逐笔", "正常", "中性", "也能", "能", "可以",
	)
	return trigger && expected && !isCompatibilitySloganOnly(text)
}

// hasStrictIntegrationEvidence requires an observable coordination contract.
// Multiple views of one record, by themselves, do not establish G16 complexity.
func hasStrictIntegrationEvidence(text string) bool {
	surfaces := 0
	for _, group := range [][]string{
		{"订单", "预约", "课程", "名单", "课表"},
		{"库存", "床位", "额度", "预算", "容量"},
		{"支付", "退款", "账户", "流水", "账期"},
		{"通知", "提醒", "消息", "配送"},
		{"审核", "权限", "角色"},
		{"目标", "计划"},
	} {
		if containsAny(text, group...) {
			surfaces++
		}
	}
	coordination := containsAny(text, "为准", "回滚", "补偿", "锁定", "释放", "预占", "核对", "对齐", "快照", "版本", "冲突")
	return surfaces >= 2 && coordination && hasFlowEvidence(text) && hasDecisionEvidence(text) && hasBoundaryEvidence(text)
}

func hasTradeoffEvidence(text string) bool {
	alternatives := containsAny(text, "而不是", "两种方案", "整体回滚", "局部恢复", "部分成功", "已成功部分", "各自快照", "最终一致", "优先保证")
	strategy := containsAny(text, "选择", "采用", "优先", "保留", "回滚", "补偿", "拒绝")
	consequence := containsAny(text, "避免", "否则", "导致", "不能", "不得", "必须", "后续", "代价")
	return alternatives && strategy && consequence
}

func hasComplexTechnicalEvidence(text string) bool {
	trigger := containsAny(text, "并发", "重复提交", "异步", "回调", "旧版本", "旧数据迁移", "迁入", "离线", "部分成功", "部分失败", "网络中断", "权限", "性能", "乱序")
	process := containsAny(text, "保存", "查询", "先", "随后", "之后", "过程中", "提交后", "刷新后", "读取", "写入", "处理", "恢复", "回退", "重放", "校验")
	guarantee := containsAny(text, "不能丢", "只生效一次", "不重复", "不能覆盖", "不得覆盖", "不能重复", "不得重复", "不能生成两条", "回滚", "补偿", "版本", "快照", "隔离", "撤销", "最终", "一致")
	return trigger && process && guarantee && hasBoundaryEvidence(text) && !isCompatibilitySloganOnly(text)
}

func isCompatibilitySloganOnly(text string) bool {
	compact := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(text), " ", ""), "\t", "")
	for _, phrase := range []string{"统一展示", "兼容旧数据", "兼容历史数据", "保持一致", "补充校验", "输出转义"} {
		compact = strings.ReplaceAll(compact, phrase, "")
	}
	compact = strings.Trim(compact, "，。；：、,.!?！？()（）")
	return compact == ""
}

func containsAny(text string, fragments ...string) bool {
	for _, fragment := range fragments {
		if strings.Contains(text, fragment) {
			return true
		}
	}
	return false
}
