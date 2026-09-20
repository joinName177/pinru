package cli

import (
	"strings"
	"testing"
)

func TestDecodePairwiseReviewAcceptsConcreteComparison(t *testing.T) {
	raw := []byte(`{"status":"ready","conclusion":"A_better","reason":"A 在 internal/filter.go 中实现了服务端筛选并通过 go test ./...；B 只修改 frontend/src/List.tsx，接口仍返回未过滤数据，因此 A 的交付更完整。"}`)
	result, err := decodePairwiseReview(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Conclusion != "A_better" || result.Status != "ready" {
		t.Fatalf("result = %#v", result)
	}
}

func TestDecodePairwiseReviewRejectsGenericOrOneSidedReason(t *testing.T) {
	for _, reason := range []string{
		"A 更好，整体完成度更高。",
		"A 在 internal/filter.go 中完成了筛选并通过测试，因此表现很好。",
		strings.Repeat("A 很好，B 也很好。", 8),
	} {
		raw := []byte(`{"status":"ready","conclusion":"A_better","reason":` + quotePairwiseJSON(reason) + `}`)
		if result, err := decodePairwiseReview(raw); err == nil {
			t.Fatalf("accepted generic review %#v for %q", result, reason)
		}
	}
}

func TestDecodePairwiseReviewRequiresPortalLengthAndDetailedSameReason(t *testing.T) {
	short := []byte(`{"status":"ready","conclusion":"A_better","reason":"A 修改 main.go，B 未修改 main.go，所以 A 更好。"}`)
	if _, err := decodePairwiseReview(short); err == nil {
		t.Fatal("expected reason shorter than 60 Chinese characters to fail")
	}
	same := []byte(`{"status":"ready","conclusion":"same","reason":"A 在 main.go 完成实现并通过测试，B 在 main.go 也完成实现并通过测试；两边表现都很好，最终选择 Same。"}`)
	if _, err := decodePairwiseReview(same); err == nil {
		t.Fatal("expected Same without equivalence or offsetting trade-offs to fail")
	}
}

func TestDecodePairwiseReviewRejectsMetricInventoryWithoutARealTradeoff(t *testing.T) {
	reason := "A 执行 npm test 的退出码为 0，共有 48 条断言通过，产物哈希为 8f45d1a09bc73125，第 126 行也完成修改；B 使用 3.2.1 版本构建，记录了 1920×1080 像素和 16 个轮廓坐标。两边基本等价，优缺点相互抵消。"
	raw := []byte(`{"status":"ready","conclusion":"same","reason":` + quotePairwiseJSON(reason) + `}`)
	if result, err := decodePairwiseReview(raw); err == nil {
		t.Fatalf("accepted metric inventory without a decision basis: %#v", result)
	}
}

func TestDecodePairwiseReviewRejectsMarkdownOrChecklistFormatting(t *testing.T) {
	reason := "- A：在页面实现了筛选功能并运行测试。\n- B：也实现了筛选功能，但没有覆盖空数据。\n- 结论：这道题更看重异常场景下是否仍能正常使用，所以选择 A。"
	raw := []byte(`{"status":"ready","conclusion":"A_better","reason":` + quotePairwiseJSON(reason) + `}`)
	if result, err := decodePairwiseReview(raw); err == nil {
		t.Fatalf("accepted checklist-form review: %#v", result)
	}
}

func TestDecodePairwiseReviewStripsDecorativeQuoteBrackets(t *testing.T) {
	for _, marks := range []string{"『筛选功能』", "「筛选功能」", "【筛选功能】", "《筛选功能》", "〔筛选功能〕", "〈筛选功能〉"} {
		reason := "A 完成了" + marks + "并运行测试确认空数据也有反馈；B 只实现常规流程，异常输入仍会中断操作。这道题更看重用户遇到异常时能否继续使用，因此 A 更可靠。"
		raw := []byte(`{"status":"ready","conclusion":"A_better","reason":` + quotePairwiseJSON(reason) + `}`)
		result, err := decodePairwiseReview(raw)
		if err != nil {
			t.Fatalf("rejected %q after stripping brackets: %v", marks, err)
		}
		if strings.ContainsAny(result.Reason, "『』「」【】《》〔〕〖〗〘〙〚〛〈〉") {
			t.Fatalf("reason still contains decorative brackets after %q: %q", marks, result.Reason)
		}
		if !strings.Contains(result.Reason, "A 完成了筛选功能并运行测试") {
			t.Fatalf("stripping %q changed the sentence beyond removing brackets: %q", marks, result.Reason)
		}
	}
}

func TestDecodePairwiseReviewAcceptsNaturalComparisonWithDecisionBasis(t *testing.T) {
	reason := "A 很快找到了筛选失效的原因，修改后页面在空数据和重复提交时都能正常反馈，最后也实际走完了用户操作；B 的主体功能可以使用，但只验证了常规流程，空数据时仍会留下一块没有说明的空白区域。这道题更看重用户遇到异常输入时能不能继续操作，因此 A 更可靠。"
	result, err := decodePairwiseReview([]byte(`{"status":"ready","conclusion":"A_better","reason":` + quotePairwiseJSON(reason) + `}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != reason {
		t.Fatalf("reason = %q", result.Reason)
	}
}

func TestDecodePairwiseReviewRejectsInactionWithoutTriggerNode(t *testing.T) {
	reason := "A 真正把复制与分享拆成两条路径：复制按钮只写剪贴板，分享按钮才走系统分享，并按真实结果更新提示，还补了回归测试且构建通过。B 全程停在读代码和空想阶段，没有改动任何文件，交付仍是有缺陷的原实现，复制按钮照旧弹出分享面板。本题更看重按钮行为与提示是否一致，因此 A 明显更好。"
	raw := []byte(`{"status":"ready","conclusion":"A_better","reason":` + quotePairwiseJSON(reason) + `}`)
	if result, err := decodePairwiseReview(raw); err == nil {
		t.Fatalf("accepted inaction criticism without a trigger node: %#v", result)
	}
}

func TestDecodePairwiseReviewAcceptsInactionWithTriggerNode(t *testing.T) {
	reason := "A 在 ClipboardShareAdapter 和 CyberCardView 中拆开复制与分享：复制按钮只写剪贴板，分享按钮按真实结果反馈，取消不再误报成功；补充的回归测试以及构建均通过。B 读完上述文件并定位到复制按钮误用 shareSlip 后，在拆分两条调用路径这一步反复重读，没有执行修改；还从 /workspace 运行 npm run build，因找不到 package.json 失败，提交仍保留原缺陷。本题最看重按钮行为与提示是否一致，因此 A 明显更好。"
	result, err := decodePairwiseReview([]byte(`{"status":"ready","conclusion":"A_better","reason":` + quotePairwiseJSON(reason) + `}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Reason != reason {
		t.Fatalf("reason = %q", result.Reason)
	}
}

func TestBuildPairwiseReviewPromptRequiresTriggerNodeForInaction(t *testing.T) {
	prompt := buildPairwiseReviewPrompt("/tmp/evidence.json", "", "")
	if !strings.Contains(prompt, "触发节点") || !strings.Contains(prompt, "只读未改") {
		t.Fatalf("prompt does not require a trigger node for inaction: %s", prompt)
	}
	for _, symbol := range []string{"『』", "「」", "【】", "《》", "〔〕", "〈〉"} {
		if !strings.Contains(prompt, symbol) {
			t.Fatalf("prompt does not forbid decorative bracket family %q: %s", symbol, prompt)
		}
	}
}

func TestDecodePairwiseReviewKeepsTriggerNodeAfterStrippingBrackets(t *testing.T) {
	reason := "A 在 ClipboardShareAdapter 和 CyberCardView 中拆开『复制』与【分享】：复制按钮只写剪贴板，分享按钮按真实结果反馈，取消不再误报成功；补充的回归测试以及构建均通过。B 读完上述文件并定位到复制按钮误用 shareSlip 后，在拆分《两条调用路径》这一步反复重读，没有执行修改；还从 /workspace 运行 npm run build，因找不到 package.json 失败，提交仍保留原缺陷。本题最看重按钮行为与提示是否一致，因此 A 明显更好。"
	result, err := decodePairwiseReview([]byte(`{"status":"ready","conclusion":"A_better","reason":` + quotePairwiseJSON(reason) + `}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(result.Reason, "『』「」【】《》〔〕〖〗〈〉") {
		t.Fatalf("reason still contains decorative brackets: %q", result.Reason)
	}
	if !strings.Contains(result.Reason, "拆开复制与分享") || !strings.Contains(result.Reason, "拆分两条调用路径") {
		t.Fatalf("stripping changed the sentence beyond removing brackets: %q", result.Reason)
	}
}

func TestDecodePairwiseReviewRejectsReasonOverThreeHundredTwentyCharacters(t *testing.T) {
	prefix := "A 完成了核心功能并实际运行了页面，B 也完成实现但异常流程仍会中断用户操作，这道题更看重实际使用是否可靠，因此 A 更值得选择。"
	reason := prefix + strings.Repeat("补", 321-len([]rune(prefix)))
	if got := len([]rune(reason)); got != 321 {
		t.Fatalf("test reason length = %d", got)
	}
	raw := []byte(`{"status":"ready","conclusion":"A_better","reason":` + quotePairwiseJSON(reason) + `}`)
	if result, err := decodePairwiseReview(raw); err == nil {
		t.Fatalf("accepted 321-character review: %#v", result)
	}
}

func TestPairwiseReviewSchemaLimitsReasonToThreeHundredTwentyCharacters(t *testing.T) {
	properties := pairwiseReviewSchema()["properties"].(map[string]any)
	reason := properties["reason"].(map[string]any)
	if reason["maxLength"] != 320 {
		t.Fatalf("reason maxLength = %#v", reason["maxLength"])
	}
}

func quotePairwiseJSON(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
