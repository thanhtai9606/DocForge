package domain_test

import (
	"testing"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
)

func TestJobTransition(t *testing.T) {
	job := &domain.Job{Status: domain.JobQueued}
	if err := job.Transition(domain.JobProcessing); err != nil {
		t.Fatalf("queued->processing: %v", err)
	}
	if err := job.Transition(domain.JobCompleted); err != nil {
		t.Fatalf("processing->completed: %v", err)
	}
	if err := job.Transition(domain.JobQueued); err == nil {
		t.Fatal("expected invalid transition error")
	}
}
