package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallBuiltinPromptSkillUsesVerifiableNaturalTaskRules(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	New().InstallBuiltinSkills()
	content, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "评审项目提示词生成", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, want := range []string{
		"150-300 个字",
		"可核查的交付结果",
		"至少一个真实边界",
		"不得出现五维评分",
		"不能故意制造失败",
		"困难题准入门槛",
		"中等题准入门槛",
		"题面必须同时具备调用/数据流理解、实现决策、边界条件与约束三项证据，缺一项都不行",
		"多模块整合、关键设计取舍、复杂技术关注点中的一项",
		"可独立验收的证据",
		"只写“统一展示”“兼容旧数据”“保持一致”“补充校验”或罗列技术名词不算命中",
		"不能拆成互不影响的局部小修",
		"三项证据必须像这条已通过审核的题一样融在同一段话里",
		"边界条件与约束的写法（只用模式，不要照抄业务对象）",
		"数值或口径冲突",
		"重复或并发操作",
		"历史数据或状态被替换",
		"中等题交付前逐条打钩",
		"擦边写法不算命中",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("installed prompt skill missing %q", want)
		}
	}
	if strings.Contains(text, "调用/数据流理解、实现决策、边界条件与约束中的一项") {
		t.Fatal("installed prompt skill still allows medium tasks with only one of the three criteria")
	}
	if strings.Contains(text, "不超过 80 字") {
		t.Fatal("installed prompt skill still contains the legacy 80-character limit")
	}
}
