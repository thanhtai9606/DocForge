package domain_test

import (
	"errors"
	"fmt"
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

func TestCanTransitionMatrix(t *testing.T) {
	cases := []struct {
		from domain.JobStatus
		to   domain.JobStatus
		ok   bool
	}{
		{domain.JobQueued, domain.JobProcessing, true},
		{domain.JobQueued, domain.JobCancelled, true},
		{domain.JobQueued, domain.JobFailed, true},
		{domain.JobQueued, domain.JobCompleted, false},
		{domain.JobProcessing, domain.JobCompleted, true},
		{domain.JobProcessing, domain.JobFailed, true},
		{domain.JobProcessing, domain.JobCancelled, true},
		{domain.JobProcessing, domain.JobQueued, false},
		{domain.JobCompleted, domain.JobFailed, false},
		{domain.JobFailed, domain.JobQueued, false},
		{domain.JobCancelled, domain.JobProcessing, false},
	}
	for _, tc := range cases {
		got := domain.CanTransition(tc.from, tc.to)
		if got != tc.ok {
			t.Fatalf("%s -> %s: got %v want %v", tc.from, tc.to, got, tc.ok)
		}
	}
}

func TestAsAppError(t *testing.T) {
	err := domain.NewAppError(domain.CodeInvalidPDF, "bad pdf", false)
	got, ok := domain.AsAppError(err)
	if !ok || got.Code != domain.CodeInvalidPDF || got.Retryable {
		t.Fatalf("unexpected AsAppError result: %#v ok=%v", got, ok)
	}
	wrapped := fmt.Errorf("wrap: %w", err)
	got, ok = domain.AsAppError(wrapped)
	if !ok || got.Code != domain.CodeInvalidPDF {
		t.Fatalf("wrapped AsAppError failed: %#v ok=%v", got, ok)
	}
	if _, ok := domain.AsAppError(errors.New("plain")); ok {
		t.Fatal("plain error should not convert")
	}
	if err.Error() != "bad pdf" {
		t.Fatalf("Error()=%q", err.Error())
	}
}
