package annotation

import (
	domain "github.com/blueship581/pinru/internal/annotation"
	"testing"
)

func TestNextPromptUsesScoresWithoutDissatisfactionOrBugRequirement(t *testing.T) {
	five, four := 5, 4
	for _, tc := range []struct {
		name   string
		scores [5]*int
		count  int
		want   string
	}{
		{"all five", [5]*int{&five, &five, &five, &five, &five}, 1, ""},
		{"process deduction", [5]*int{&five, &five, &four, &five, &five}, 1, "补充边界验证并说明结果"},
		{"missing evidence", [5]*int{&five, &five, nil, &five, &five}, 1, ""},
		{"round limit", [5]*int{&five, &five, &four, &five, &five}, 10, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &domain.Evaluation{Scores: tc.scores, Status: "ready", NextPrompt: "补充边界验证并说明结果"}
			normalizeNextPrompt(e, tc.count)
			if e.NextPrompt != tc.want {
				t.Fatalf("got %q want %q", e.NextPrompt, tc.want)
			}
		})
	}
}
