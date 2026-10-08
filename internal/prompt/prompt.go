package prompt

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/blueship581/pinru/internal/analysis"
	"github.com/blueship581/pinru/internal/util"
)

// ── 任务类型 ──────────────────────────────────────────────────────────────────

const (
	TaskTypeUncategorized = "未归类"
	TaskTypeBugFix        = "Bug修复"
	TaskTypeCodeGen       = "0-1代码生成"
	TaskTypeFeature       = "Feature迭代"
	TaskTypeUnderstand    = "代码理解"
	TaskTypeRefactor      = "代码重构"
	TaskTypeEngineering   = "工程化"
	TaskTypeTesting       = "代码测试"
)

var taskTypeAliases = map[string]string{
	"uncategorized": TaskTypeUncategorized,
	"unclassified":  TaskTypeUncategorized,
	"未分类":           TaskTypeUncategorized,
	"未归类":           TaskTypeUncategorized,
	"bugfix":        TaskTypeBugFix,
	"bug修复":         TaskTypeBugFix,
	"缺陷修复":          TaskTypeBugFix,
	"代码生成":          TaskTypeCodeGen,
	"0-1代码生成":       TaskTypeCodeGen,
	"0-1":           TaskTypeCodeGen,
	"0到1":           TaskTypeCodeGen,
	"从0到1":          TaskTypeCodeGen,
	"从零到一":          TaskTypeCodeGen,
	"feature":       TaskTypeFeature,
	"feature迭代":     TaskTypeFeature,
	"功能开发":          TaskTypeFeature,
	"代码理解":          TaskTypeUnderstand,
	"refactor":      TaskTypeRefactor,
	"代码重构":          TaskTypeRefactor,
	"perf":          "性能优化",
	"性能优化":          "性能优化",
	"工程化":           TaskTypeEngineering,
	"test":          TaskTypeTesting,
	"测试":            TaskTypeTesting,
	"测试补全":          TaskTypeTesting,
	"代码测试":          TaskTypeTesting,
}

// ── 约束标签类型 ───────────────────────────────────────────────────────────────

const (
	ConstraintStack    = "技术栈或依赖约束"
	ConstraintArch     = "架构或模式约束"
	ConstraintStyle    = "代码风格或规范约束"
	ConstraintNonCode  = "非代码回复约束"
	ConstraintBusiness = "业务逻辑约束"
	ConstraintNone     = "无约束"
)

// ── 修改范围 ──────────────────────────────────────────────────────────────────

const (
	ScopeSingleFile  = "单文件"
	ScopeModuleFiles = "模块内多文件"
	ScopeCrossModule = "跨模块多文件"
	ScopeCrossSystem = "跨系统多模块"
)

const (
	PreferredPromptBodyMinRunes = 150
	MaxPromptBodyRunes          = 300
)

// ── 任务类型到出题要点的精简指导 ──────────────────────────────────────────────

// taskGuidance 是从执行手册提炼的出题要点，供 LLM 理解每种任务类型的出题方向。
// 不直接暴露给最终生成的提示词，而是作为 LLM 的内部参考材料。
var taskGuidance = map[string]string{
	TaskTypeUncategorized: `出题方向：先不要预设任务类别，直接根据仓库里最真实、最典型、最容易被开发者提出来的问题或需求出题。
可以是 bug、功能补充、理解梳理、重构或测试补全中的任意一种，但题目表达仍然要自然，像用户真实提出的需求。
关键要求：
- 不要为了贴类别而硬套模板，优先选择仓库里最值得做的一件事
- 题目描述仍然必须具体，有清晰的业务现象或目标
- 如果仓库里存在明显问题，优先围绕真实痛点出题`,

	TaskTypeBugFix: `出题方向：找出代码中存在的逻辑错误、运行时异常、边界条件遗漏、类型错误或安全漏洞，
描述用户在使用系统时遇到的异常现象（报错信息、非预期输出、功能失效等），
让模型去定位并修复这个 bug。
关键要求：
- 描述"遇到了什么问题"，而非"要修改哪个文件"
- 可以提供报错信息的文字描述（不要粘贴堆栈，用业务语言描述现象）
- 必须基于仓库中真实存在的代码缺陷出题`,

	TaskTypeCodeGen: `出题方向：在现有系统、现有仓库和现有业务边界的基础上，新增一个此前还不存在的完整业务模块或核心能力。
描述用户为什么现在需要这项能力、它要覆盖哪些关键流程、不同角色分别怎么使用，
让模型从当前系统起步，把这个能力从入口、流程、状态到结果补齐完整。
关键要求：
- 明确这是"基于现有系统补齐完整新模块/新能力"，不是在空白项目里独立造一个 demo
- 要体现完整链路，而不只是加一个孤立按钮、字段或接口
- 需要说明和现有功能、现有数据、现有流程怎么衔接，避免和普通 Feature 迭代混淆
- 生成结果应能直接运行和验证`,

	TaskTypeFeature: `出题方向：在现有已实现的功能基础上，扩展或新增一个与业务相关的新特性。
描述"用户希望新增什么能力"，强调新功能与现有功能的协同关系，
让模型在不破坏原有逻辑的前提下平滑迭代。
关键要求：
- 明确说明是"在现有系统基础上"新增功能，不是从零构建
- 强调新旧功能的兼容性要求
- 功能扩展要自然合理，是现有功能的延伸
- 更适合已有模块的增强、补入口、补规则、补交互，不要写成完整新模块从零到一落地`,

	TaskTypeUnderstand: `出题方向：要求解释、梳理或可视化某段代码/功能模块的运作机制。
以"不了解这套系统的新人"视角提问，描述"我想理解 XXX 是怎么运作的"，
让模型输出易于理解的解释、图表或文档。
关键要求：
- 问题聚焦在"弄清楚系统是怎么工作的"，而非修改代码
- 可要求输出架构图、流程图、数据流说明等（用约束标签指定格式）
- 描述要自然，像真实开发者提出的理解需求`,

	TaskTypeRefactor: `出题方向：找出代码中存在的代码异味（上帝函数、重复代码、紧耦合、难以维护等），
描述"这部分代码维护起来很痛苦"的用户感受，
让模型在不改变外部行为的前提下对内部结构进行重构优化。
关键要求：
- 重构目标要明确（性能、可读性、可维护性、解耦等）
- 必须强调"不改变外部行为" / "保持向后兼容"
- 基于真实存在代码异味的模块出题`,

	TaskTypeEngineering: `出题方向：围绕构建流程、自动化、测试配置、依赖管理、CI/CD 等工程化需求出题。
描述"团队在开发流程中遇到了什么效率问题"，
让模型提供工程化解决方案。
关键要求：
- 聚焦在开发流程和工具链层面，而非业务逻辑
- 描述工程化痛点，如"每次发布都要手动操作"、"测试配置混乱"等
- 要求可执行的方案，不是纯理论描述`,

	TaskTypeTesting: `出题方向：围绕单元测试、集成测试、回归测试或测试基建补齐出题。
描述"当前代码缺少有效验证，改动后容易回归"这一类真实问题，
让模型补充测试用例或改进测试覆盖。
关键要求：
- 重点是验证已有或新增行为，而不是重复实现业务逻辑
- 明确说明要覆盖的场景、边界条件或异常分支
- 测试需求必须与仓库中现有代码和测试框架兼容`,
}

// ── 约束方向描述映射 ─────────────────────────────────────────────────────────

// constraintDirection 将约束类型映射为自然语言的方向描述（仅供 LLM 理解方向），
// 不得作为输出中的标签前缀。
var constraintDirection = map[string]string{
	ConstraintStack:    "关注技术栈或依赖选择方面的要求",
	ConstraintArch:     "关注架构或设计模式方面的要求",
	ConstraintStyle:    "关注代码风格或命名规范方面的要求",
	ConstraintNonCode:  "关注非代码产出（文档、图表等）方面的要求",
	ConstraintBusiness: "关注业务逻辑或业务规则方面的要求",
}

// ── 公共类型 ──────────────────────────────────────────────────────────────────

type TaskInfo struct {
	ID              string
	GitLabProjectID int64
	ProjectName     string
	Status          string
}

type PromptRequest struct {
	TaskType        string
	Scopes          []string
	Constraints     []string
	AdditionalNotes *string
}

// ── 系统提示词 ────────────────────────────────────────────────────────────────

// BuildSystemPrompt 返回严格要求业务语言输出的系统提示词。
func BuildSystemPrompt() string {
	return strings.Join([]string{
		"你是一名专业的代码模型评测出题员。你的任务是：基于给定的代码仓库分析，",
		"为代码模型生成一道「评测任务提示词」。",
		"",
		"输出必须严格遵守以下规则，每一条都不可违反：",
		"",
		"1. 只写业务需求，禁止写技术实现",
		"   - 不能出现：文件路径、类名、方法名、函数名、变量名、import 语句、package 名",
		"   - 不能出现：数据库表名、字段名、API 路径的具体字符串",
		"   - 可以出现：功能描述、业务场景、用户视角的操作描述",
		"",
		"2. 禁止 Markdown 格式",
		"   - 不能出现：井号标题、双星粗体、代码块、有序或无序列表符号",
		"   - 输出必须是纯文本段落",
		"",
		"3. 自然完整",
		fmt.Sprintf("   - 正文描述控制在 1 段 3-6 句话内，完整表达业务背景、用户现象、目标结果和必要边界"),
		fmt.Sprintf("   - 全文建议控制在 %d-%d 个字之间（空白字符不计入），最多不超过 %d 个字", PreferredPromptBodyMinRunes, MaxPromptBodyRunes, MaxPromptBodyRunes),
		"   - 去掉空话和套话，但不要为了压短而丢掉关键业务信息",
		"   - 中等题必须写出一个真正改变实现路径的边界：不是把正常路径再说一遍，而是补上超限、重复、冲突、空值或旧数据这类特例发生时该怎么处理，并写清用户看到的提示和数据的最终结果",
		"",
		"4. 约束要求必须融入正文",
		"   - 所有约束要求（技术栈、架构、代码风格、业务规则等）必须作为正文的自然组成部分写出，和需求描述合在同一段里",
		"   - 严禁将约束单独分段、分行或加任何前缀标签",
		"   - 严禁出现\"xx约束：\"\"xx约束:\"\"xx要求：\"等\"标签名称：内容\"形式的分类标头",
		"   - 最终输出应该是一整段连贯的文字，像真实开发者在聊天窗口里一口气说完的需求",
		"   - 若无约束，则不要为了凑字数而添加约束相关的句子",
		"",
		"5. 口语化、自然",
		"   - 读起来要像真实开发者或产品经理发出的任务描述",
		"   - 去除 AI 写作惯用的刻板措辞，不得出现模板化表达、AI式前言或机械总结",
		"   - 语句必须通顺完整，不堆砌同义短语，不用少量同义词替换制造语义换皮",
		"   - 不使用箭头、Emoji、反引号或装饰性符号串联需求",
		"   - 若输入明确要求保留必要的文件名、命令或其他关键事实，不得为了调整文风而删除或改名",
		"",
		"6. 输出前自检",
		fmt.Sprintf("   - 如果正文部分超过 %d 个字，先自行压缩语言，再输出最终版本", MaxPromptBodyRunes),
		"   - 仅供内部选题自检：中等题必须跨多文件并产生不少于 20 行有效代码改动，且题面必须同时具备调用/数据流理解、实现决策、边界条件与约束三项证据，缺一项都不行：调用/数据流要写出输入或动作经过的业务环节及最终影响，实现决策要写出处理策略的选择及其正常、失败或冲突结果，边界条件与约束要写出具体触发条件、边界输入或旧数据及预期行为；困难或地狱题还必须跨多文件并产生不少于 20 行有效代码改动，且题面至少体现多模块整合、关键设计取舍、复杂技术关注点中的一项，并留下模块汇合、冲突取舍或复杂技术触发的完整证据。只写抽象结论、兼容性口号或技术名词不算命中。改动规模大致可比，不能一侧是几行微修、另一侧才是完整重构；依赖锁文件、依赖目录、构建产物和纯文档不计入",
		"",
		"   " + BuildMediumDifficultyReferenceRule(),
		"",
		"   " + BuildBoundaryExpressionRule(),
		"",
		"   " + BuildMediumAcceptanceChecklistRule(),
		"",
		"直接输出提示词正文，不要加任何前言、解释或标注。",
	}, "\n")
}

// ── 用户提示词 ────────────────────────────────────────────────────────────────

// BuildUserPrompt 基于代码分析和任务参数构建发给 LLM 的用户提示词。
// 代码上下文仅作为出题参考，不能直接出现在最终提示词中。
func BuildUserPrompt(task TaskInfo, req PromptRequest, summary analysis.Summary, _ string) string {
	var sb strings.Builder

	// ── 1. 任务类型出题指导 ──
	normalizedTaskType := NormalizeTaskType(req.TaskType)
	guidance := taskGuidance[normalizedTaskType]
	if guidance == "" {
		guidance = "请根据代码仓库内容，生成一道符合业务场景的评测提示词。"
	}
	sb.WriteString("任务类型：")
	sb.WriteString(normalizedTaskType)
	sb.WriteString("\n\n")
	sb.WriteString("出题要点（仅供你理解方向，不要出现在输出中）：\n")
	sb.WriteString(guidance)
	sb.WriteString("\n\n")

	// ── 2. 修改范围要求 ──
	if len(req.Scopes) > 0 {
		sb.WriteString("修改范围：")
		sb.WriteString(strings.Join(req.Scopes, "、"))
		sb.WriteString("\n")
		sb.WriteString(buildScopeGuidance(req.Scopes))
		sb.WriteString("\n\n")
	}

	// ── 3. 约束要求 ──
	// 约束必须自然融入正文，不得单独分段或使用任何标签前缀。
	constraintDescs := buildConstraintDescriptions(req.Constraints)
	if len(constraintDescs) > 0 {
		sb.WriteString("本题需要在正文中自然体现以下方面的要求（仅供你理解方向，严禁作为输出中的标题或分段依据）：\n")
		for _, desc := range constraintDescs {
			sb.WriteString("- ")
			sb.WriteString(desc)
			sb.WriteString("\n")
		}
		sb.WriteString("重要：把上述所有要求和需求描述合在同一段文字里写出来，像开发者在聊天窗口一口气说完。\n")
		sb.WriteString("严禁单独分行、分段或加任何\"xx约束：\"\"xx要求：\"形式的前缀标签。\n")
		sb.WriteString("正面示例（仅示意风格，不可照抄）：\n")
		sb.WriteString("\"公益岗位有合同到期时间，现在系统只能查看所有在职记录，没法提前看到哪些合同快到期了。在公益岗位模块加一个到期预警视图，列出30天内要到期的岗位和已过期未退出的人员，标明剩余天数。使用现有桌面端 IPC 通信方式对接数据，不引入新的第三方包。查询逻辑放在主进程，渲染进程只负责展示，保持现有进程分工。对外函数需标注参数和返回类型，变量名采用小驼峰格式。\"\n")
		sb.WriteString("反面示例（严禁这样写）：\n")
		sb.WriteString("\"...标明剩余天数。\\n技术栈约束：使用现有桌面端 IPC 通信方式...\\n架构约束：查询逻辑放在主进程...\\n代码风格约束：对外函数需标注参数...\"\n")
		sb.WriteString("约束内容必须结合以下仓库信息生成，用业务/场景语言表达，不写技术标识符。\n\n")
	} else {
		sb.WriteString("本题没有额外约束，正文写完即止，不要追加任何约束相关的句子。\n\n")
	}

	// ── 4. 额外说明 ──
	if req.AdditionalNotes != nil && strings.TrimSpace(*req.AdditionalNotes) != "" {
		sb.WriteString("额外要求：")
		sb.WriteString(strings.TrimSpace(*req.AdditionalNotes))
		sb.WriteString("\n\n")
	}

	// ── 5. 仓库上下文（仅供出题参考，禁止直接引用到输出中）──
	sb.WriteString("=== 仓库参考信息（仅用于理解业务背景，禁止出现在输出中）===\n")
	sb.WriteString("项目名称：")
	sb.WriteString(task.ProjectName)
	sb.WriteString("\n")
	sb.WriteString("技术栈：")
	sb.WriteString(strings.Join(summary.DetectedStack, "、"))
	sb.WriteString("\n")
	sb.WriteString("文件数：")
	sb.WriteString(fmt.Sprintf("%d", summary.TotalFiles))
	sb.WriteString("\n")

	if len(summary.FileTree) > 0 {
		sb.WriteString("\n目录结构（节选）：\n")
		// 只取前 30 行文件树，避免 token 浪费
		lines := summary.FileTree
		if len(lines) > 30 {
			lines = lines[:30]
		}
		sb.WriteString(strings.Join(lines, "\n"))
		sb.WriteString("\n")
	}

	// 代码片段：只用于让 LLM 理解业务逻辑，但明确禁止泄露技术细节到输出
	if len(summary.KeyFiles) > 0 {
		sb.WriteString("\n核心文件内容（仅供理解业务逻辑，绝对不能将文件路径/类名/函数名写进提示词）：\n")
		// 最多展示 3 个文件片段，减少技术细节泄露风险
		count := len(summary.KeyFiles)
		if count > 3 {
			count = 3
		}
		for i := 0; i < count; i++ {
			f := summary.KeyFiles[i]
			sb.WriteString("[文件 ")
			sb.WriteString(fmt.Sprintf("%d", i+1))
			sb.WriteString("]\n")
			sb.WriteString(f.Snippet)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("=== 参考信息结束 ===\n\n")

	// ── 6. 最终输出要求 ──
	sb.WriteString(TaskTypeDifficultyRule)
	sb.WriteString("现在请生成一道符合上述要求的评测提示词。")
	if len(constraintDescs) > 0 {
		sb.WriteString("所有约束要求必须和需求描述融合在同一段文字中，严禁单独分段或加任何标签前缀。")
	}
	sb.WriteString(fmt.Sprintf("输出前请自检：全文建议控制在 %d-%d 个字之间，最多不超过 %d 个字，且必须是一整段连贯的文字。", PreferredPromptBodyMinRunes, MaxPromptBodyRunes, MaxPromptBodyRunes))
	sb.WriteString("中等题还要同时具备调用/数据流理解、实现决策、边界条件与约束三项证据，缺一项都不算达标：写清输入到结果的业务链路、处理策略的选择及正常/失败/冲突结果、真实触发条件及预期行为；困难题还要至少体现多模块整合、关键设计取舍、复杂技术关注点中的一项，并写出模块汇合、冲突取舍或复杂技术触发的完整证据。只写抽象结论或技术名词、三项缺任何一项时必须换题。")
	sb.WriteString(BuildBoundaryExpressionRule())
	sb.WriteString(BuildMediumAcceptanceChecklistRule())
	sb.WriteString(BuildMediumDifficultyReferenceRule())
	sb.WriteString("直接输出提示词内容，不加任何前言或说明。")

	return sb.String()
}

// ── 辅助函数 ──────────────────────────────────────────────────────────────────

// TaskTypeDifficultyRule separates generation labels from G16/G19 content gates.
const TaskTypeDifficultyRule = "题型与标签规则（优先于通用难度措辞）：0-1代码生成按 G16 选择真实困难题，标签为困难或地狱；Feature迭代和 Bug修复按 G19 至少达到中等的真实难度，调用/数据流理解、实现决策、边界条件与约束三项必须全部具备，出题标签固定为困难。下文中等题门槛用于 Feature/Bug 内容验收，困难题门槛用于 0-1 内容验收，不能依据展示标签混用。G16/G19 不合格题作废，必须重新选题，不得只润色措辞或调高难度字段。内部逐题记录源码依据、用户动作到结果的调用链、影响结果对错的策略选择、边界触发及预期行为；三项须分别核对，不能用一句空态提示充当全部证据，也不能凭“重复提交、不重复、沿用、展示”等关键词判定达标；0-1 另记录至少一项困难特征的完整证据。正文只保留自然业务场景，不输出审核术语或自检记录。"

// MediumDifficultyReferencePrompt 是已通过审核的中等题范例（c2c-001-2，Feature迭代）。
// 三项中等难度证据在同一段话里自然表达：调用/数据流是“点击建议后带参数预填并跳转、
// 入账后组合与建议立即刷新”；实现决策是“已经补过仓的类别对应的建议不再重复出现”；
// 边界条件是“已经补过仓”“避免照着旧提示重复入账”。它只作为写法参照，禁止照抄其中的
// 业务对象。
const MediumDifficultyReferencePrompt = "分析页给出的补仓建议目前只是一句文字，用户看完还得自己切到记录页手动选类别填小时，来回折腾几趟就容易忘。需要把建议做成能直接执行的入口，点击低配资产的建议后带着推荐的类别和小时数预填到入账表单并跳转到记录页，入账后建议和组合数据立即按新情况刷新，已经补过仓的类别对应的低配建议不再重复出现，避免用户照着旧提示重复入账"

// BoundaryExpressionPatterns 是中等题“边界条件与约束”这一项的写法模式，提炼自已通过
// 审核的题库：边界句必须改变实现路径，说明特例发生时用户看到什么、数据最终是什么。
// 只点名词（超额、重复）或只喊“保持一致”不算命中。
const BoundaryExpressionPatterns = "边界条件与约束的写法（只用模式，不要照抄业务对象）：一、数值或口径冲突：写出同一时刻出现的互相矛盾的数值或口径，说明以哪一处为准，并交代剩余额度、已配置、健康度这类数字最终按同一套口径对齐，例如超出上限入账时明确提示差额并阻止入账或让用户确认，不能让两处各算各的。二、重复或并发操作：写清重复提交、连续点击、撤销超时后再次操作时系统怎么处理，是保留首次、合并还是拒绝第二次，并保证重复操作不会重复累计。三、历史数据或状态被替换：写清旧记录、已失效配置或未保存内容被重置、回滚、覆盖时的兜底，给出默认值、迁移或回退方式，说明用户会看到什么提示。反例（这样写不算命中）：只写“超过上限就提示”或“重复提交要处理”，只列正常分支不说异常分支走哪条路；边界句只是把正常路径再说一遍；只喊“保持一致”却不说明以哪一处为准。"

// HardDifficultyEvidencePatterns 是困难题“必须被审核方看见”的三项特征写法。审核按
// G16 判定：题面里看不到多模块整合、关键设计取舍或复杂技术关注点，就按真实难度打回，
// 调高难度字段无效。因此困难题必须主动写出取舍或复杂链路，而不是罗列功能与边界。
const HardDifficultyEvidencePatterns = "困难题三项特征必须至少写实一项，而且要写到审核方一眼能看见（只罗列功能和边界会被判成中等）：一、多模块整合：写清至少两个业务环节如何汇合，谁产生数据、谁消费数据、以哪一处为准，最后哪些页面或统计同时变化；两个对象各有自己的状态时也要交代冲突怎么对齐。二、关键设计取舍：写清两个都说得通的方案，本题选哪个、为什么，以及另一个方案会带来什么后果；例如整体回滚还是保留已成功的部分、先写本地再同步还是等确认、同一份数据多处共享还是各自快照。三、复杂技术关注点：写清并发、重复提交、离线、异步回调、版本兼容、旧数据迁移、权限差异、性能退化或异常链路中的至少一个触发过程，并给出该场景下系统必须保证的结果。反例（这样写会被打回）：只写“超限就提示”“保存失败要重试”这类单点边界；只把新增入口、新页面、新统计列一遍；把几个互不相关的小改拼成一题。"

// HardDifficultyChecklist 是困难题交付前的逐条打钩清单，用来对齐审核口径。
const HardDifficultyChecklist = "困难题交付前逐条打钩（勾不上就换题重写，不是改措辞）：一、本题有没有一条贯穿两个以上业务环节的链路，能说清数据从哪产生、被谁消费、冲突时以哪一处为准、最后哪些页面同时变化。二、本题有没有一个真正的设计岔路口，两个方案都合理而本题只选一个，并写出了选择后的后果。三、本题有没有一个复杂技术场景（并发、重复提交、离线、异步、版本兼容、旧数据迁移、权限、性能、异常恢复），并写清该场景下必须保证的结果。三项至少要有一项写实；如果只能写出“新增入口 + 边界提示”，说明这题是中等题，必须换题。"

// BuildHardDifficultyRule 生成给 LLM 的困难题写法说明，供单题出题、技能文档与批量
// 出题共用，避免三处规则漂移。
func BuildHardDifficultyRule() string {
	return HardDifficultyEvidencePatterns
}

// BuildHardDifficultyChecklistRule 生成给 LLM 的困难题打钩清单。
func BuildHardDifficultyChecklistRule() string {
	return HardDifficultyChecklist
}

// BuildBoundaryExpressionRule 生成给 LLM 的边界写法说明，供单题出题和批量出题共用。
func BuildBoundaryExpressionRule() string {
	return BoundaryExpressionPatterns
}

// MediumAcceptanceChecklist 是中等题交付前的逐条打钩清单。它把“三项证据齐全”
// 拆成可执行的自检动作：调用/数据流要能追出链路，实现决策要能指出标准路径和特殊
// 情况下的取舍，边界条件要是一个真正改变实现路径的特例而不是复述需求。
const MediumAcceptanceChecklist = "中等题交付前逐条打钩（任何一条没打上钩都必须重写题目，不是修改措辞）：一、调用/数据流：能否追出用户的一次操作经过哪些处理、哪些校验、哪些计算，最后落在哪些页面的哪些数字上，并用同时或一起体现多处联动。二、实现决策：能否指出系统在哪一步做了取舍，标准情况下怎么处理、出现异常或冲突时走哪条路，有没有明确写出以哪一处为准。三、边界条件：是否补上了一个会改变实现的特例，例如超出上限、重复操作、两项口径冲突、空值或旧数据、操作被撤销或恢复，而不是把正常路径再说一遍；这个特例发生时用户看到什么，数据最终结果是什么，能否直接对照验收。"

// BuildMediumAcceptanceChecklistRule 生成给 LLM 的中等题验收清单说明，供单题出题和
// 批量出题共用，避免两处规则漂移。
func BuildMediumAcceptanceChecklistRule() string {
	return MediumAcceptanceChecklist
}

// BuildMediumDifficultyReferenceRule 生成给 LLM 的中等题范例说明，供单题出题和批量
// 出题共用，避免两处规则漂移。
func BuildMediumDifficultyReferenceRule() string {
	return "中等题写法范例（该题已通过审核，三项证据必须像它一样融在同一段话里，不得照抄其中的业务对象）：" + MediumDifficultyReferencePrompt + "。这条题里，点击建议后带参数预填并跳转到入账、入账后组合与建议立即刷新是调用/数据流；已经补过仓的类别不再重复出现是明确的实现决策；已经补过仓、避免照着旧提示重复入账是边界条件与约束。三项缺一不可。"
}

func NormalizeTaskType(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	key := strings.ToLower(strings.ReplaceAll(trimmed, " ", ""))
	if normalized, ok := taskTypeAliases[key]; ok {
		return normalized
	}

	return trimmed
}

// buildConstraintDescriptions 将约束类型列表转换为自然语言方向描述。
// "无约束" 类型直接过滤，其余类型返回方向描述（不含任何可作标签的短语）。
func buildConstraintDescriptions(constraints []string) []string {
	var descs []string
	for _, c := range constraints {
		if c == ConstraintNone || strings.Contains(c, "无约束") {
			continue
		}
		if desc, ok := constraintDirection[c]; ok {
			descs = append(descs, desc)
		} else {
			// 未知约束类型，直接使用原始值
			descs = append(descs, c)
		}
	}
	return descs
}

// buildScopeGuidance 根据选中的修改范围生成出题提示。
func buildScopeGuidance(scopes []string) string {
	var hints []string
	for _, s := range scopes {
		switch s {
		case ScopeSingleFile:
			hints = append(hints, "题目涉及的改动仅限于单一功能点或单一页面（单文件级别）")
		case ScopeModuleFiles:
			hints = append(hints, "题目的改动涉及同一功能模块内的多个协作部分（模块内多文件）")
		case ScopeCrossModule:
			hints = append(hints, "题目的改动需要跨越多个不同功能模块（跨模块多文件）")
		case ScopeCrossSystem:
			hints = append(hints, "题目的改动需要涉及前后端联动、多个子系统或数据存储与业务逻辑的联动（跨系统多模块）")
		}
	}
	if len(hints) == 0 {
		return ""
	}
	return "范围说明：" + strings.Join(hints, "；")
}

// SplitPromptSections 兼容旧格式提示词，将正文与约束标签行分离。
// 新规则下提示词不应有独立约束行，此函数保留用于向后兼容（压缩流程使用）。
func SplitPromptSections(promptText string) (string, []string) {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(promptText), "\r\n", "\n"), "\n")
	bodyLines := make([]string, 0, len(lines))
	constraintLines := make([]string, 0, len(lines))
	inConstraintSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if isLegacyConstraintLine(trimmed) {
			inConstraintSection = true
			constraintLines = append(constraintLines, trimmed)
			continue
		}

		if inConstraintSection {
			constraintLines = append(constraintLines, trimmed)
			continue
		}

		bodyLines = append(bodyLines, trimmed)
	}

	return strings.Join(bodyLines, "\n"), constraintLines
}

// PromptBodyRuneCount 统计提示词正文的非空白字符数。
// 兼容旧格式：若存在约束标签行则排除。
func PromptBodyRuneCount(promptText string) int {
	body, _ := SplitPromptSections(promptText)

	count := 0
	for _, r := range body {
		if unicode.IsSpace(r) {
			continue
		}
		count++
	}
	return count
}

func PromptBodyExceedsLimit(promptText string) bool {
	return PromptBodyRuneCount(promptText) > MaxPromptBodyRunes
}

func ValidatePromptWritingQuality(promptText string) error {
	trimmed := strings.TrimSpace(promptText)
	if trimmed == "" {
		return fmt.Errorf("提示词不能为空")
	}
	if strings.ContainsAny(trimmed, "\r\n") {
		return fmt.Errorf("提示词必须是连贯的单段文字")
	}
	compactLower := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(trimmed, " ", ""), "\t", ""))
	for _, prefix := range []string{"以下是", "作为一个ai", "作为ai", "基于以上分析", "我将从以下几个方面"} {
		if strings.HasPrefix(compactLower, prefix) {
			return fmt.Errorf("不要使用“%s”等模板化表达，直接陈述需求", prefix)
		}
	}
	for _, fragment := range []string{
		"五维评分", "审核分数", "评分门槛", "收录门槛", "压低分数",
		"为了扣分", "方便扣分", "制造扣分", "故意保留一个问题", "让模型容易出错",
	} {
		if strings.Contains(compactLower, fragment) {
			return fmt.Errorf("提示词不得包含评分、收录或诱导失败等审核规则")
		}
	}
	vagueOnly := compactLower
	for _, phrase := range []string{"优化体验", "完善逻辑", "增强稳定性"} {
		vagueOnly = strings.ReplaceAll(vagueOnly, phrase, "")
	}
	vagueOnly = strings.Trim(vagueOnly, "，。！？；：、,.!?;:（）()")
	if vagueOnly == "" {
		return fmt.Errorf("提示词内容过于空泛，请写明真实场景、目标行为和可核查结果")
	}
	if strings.Contains(trimmed, "`") || strings.Contains(trimmed, "→") || strings.Contains(trimmed, "⇒") ||
		strings.Contains(trimmed, "➜") || strings.Contains(trimmed, "➡") || strings.Contains(trimmed, "->") || strings.Contains(trimmed, "=>") {
		return fmt.Errorf("不要使用箭头、反引号等装饰符号")
	}
	for _, r := range trimmed {
		if r == 0x2022 || (r >= 0x2190 && r <= 0x21FF) || (r >= 0x2460 && r <= 0x27BF) ||
			(r >= 0x1F000 && r <= 0x1FAFF) || r == 0xFE0F {
			return fmt.Errorf("不要使用箭头、Emoji、勾选图标等装饰符号")
		}
	}
	for _, ending := range []string{"同时还需要", "并且", "以及", "而且", "但是", "例如", "比如", "包括", "从而"} {
		if strings.HasSuffix(trimmed, ending) {
			return fmt.Errorf("语句不完整，不能以“%s”等连接语结束", ending)
		}
	}
	return nil
}

func promptWritingSimilarity(left, right string) float64 {
	leftSet := promptWritingBigrams(left)
	rightSet := promptWritingBigrams(right)
	if len(leftSet) == 0 || len(rightSet) == 0 {
		return 0
	}
	intersection := 0
	for gram := range leftSet {
		if _, ok := rightSet[gram]; ok {
			intersection++
		}
	}
	return float64(2*intersection) / float64(len(leftSet)+len(rightSet))
}

func normalizePromptWriting(value string) string {
	var normalized strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}

func promptWritingBigrams(value string) map[string]struct{} {
	runes := []rune(value)
	grams := make(map[string]struct{}, len(runes))
	for index := 0; index+1 < len(runes); index++ {
		grams[string(runes[index:index+2])] = struct{}{}
	}
	return grams
}

func BuildShortenSystemPrompt(limit int) string {
	return strings.Join([]string{
		"你是一名中文产品需求文案编辑。",
		fmt.Sprintf("请把给定提示词压缩到 %d 个字以内，同时尽量保留自然完整的业务表达。", limit),
		"不要改变业务含义，不要引入技术实现。",
		fmt.Sprintf("压缩后的正文仍应尽量保持在 %d-%d 个字这一自然业务描述区间内。", PreferredPromptBodyMinRunes, limit),
		"所有约束要求必须和需求描述融合在同一段文字中，不要单独分行或加标签前缀。",
		"只输出精炼后的文字，不要输出解释、前言、标题或 Markdown。",
	}, "\n")
}

func BuildShortenUserPrompt(body string, limit int) string {
	var sb strings.Builder
	sb.WriteString("请在不改变原意的前提下，把下面这段提示词压缩得更短、更自然。\n")
	sb.WriteString(fmt.Sprintf("要求：最终尽量控制在 %d-%d 个字之间；如果原文过长，至少确保不超过 %d 个字，保留业务场景、问题现象、目标结果和必要边界，所有内容合在一段里。\n", PreferredPromptBodyMinRunes, limit, limit))
	sb.WriteString("原文如下：\n")
	sb.WriteString(strings.TrimSpace(body))
	return sb.String()
}

// isLegacyConstraintLine 检查是否为旧格式的约束标签行，用于向后兼容。
func isLegacyConstraintLine(line string) bool {
	for _, prefix := range []string{
		"技术栈约束：",
		"技术栈约束:",
		"架构约束：",
		"架构约束:",
		"代码规范约束：",
		"代码规范约束:",
		"非代码回复约束：",
		"非代码回复约束:",
		"业务逻辑约束：",
		"业务逻辑约束:",
	} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// DefaultManualDir 返回执行手册的平台对应存放目录，供 CLI Agent 的 additionalDirs 使用。
func DefaultManualDir() string { return util.PinruManualDir() }
