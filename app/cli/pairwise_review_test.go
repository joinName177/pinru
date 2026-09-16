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

func quotePairwiseJSON(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
