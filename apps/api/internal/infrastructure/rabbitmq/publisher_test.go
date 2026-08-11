package rabbitmq_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/rabbitmq"
)

func TestProcessJobMessageJSON(t *testing.T) {
	msg := rabbitmq.ProcessJobMessage{
		JobID:      "job-1",
		DocumentID: "doc-1",
		EnqueuedAt: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var got rabbitmq.ProcessJobMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.JobID != "job-1" || got.DocumentID != "doc-1" || got.EnqueuedAt == "" {
		t.Fatalf("got=%+v", got)
	}
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"job_id", "document_id", "enqueued_at"} {
		if _, ok := asMap[key]; !ok {
			t.Fatalf("missing key %s in %s", key, string(raw))
		}
	}
}
