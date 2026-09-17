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

func quotePairwiseJSON(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
