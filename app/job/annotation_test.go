package job

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/blueship581/pinru/app/testutil"
)

func TestAnnotationJobUsesRegisteredHandlerAndPersistsOutput(t *testing.T) {
	st := testutil.OpenTestStore(t)
	svc := New(st, nil, nil, nil, nil, nil, func(ctx context.Context, kind, payload string) (any, error) {
		return map[string]string{"result": "frozen", "kind": kind}, nil
	})
	j, err := svc.SubmitJob(SubmitJobRequest{JobType: "annotation_export", InputPayload: `{"projectId":"p"}`, MaxRetries: 1, TimeoutSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, err := st.GetBackgroundJob(j.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == "done" {
			if got.OutputPayload == nil || !strings.Contains(*got.OutputPayload, "frozen") {
				t.Fatalf("missing output: %+v", got)
			}
			return
		}
		if got.Status == "error" {
			t.Fatalf("job failed %+v", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not finish")
}
