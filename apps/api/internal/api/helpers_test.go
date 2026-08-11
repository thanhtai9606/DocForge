package api

import (
	"testing"

	"github.com/thanhtai9606/DocForge/apps/api/internal/artifacts"
)

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" markdown, json , ,docx ")
	want := []string{"markdown", "json", "docx"}
	if len(got) != len(want) {
		t.Fatalf("got=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got=%v want=%v", got, want)
		}
	}
}

func TestContentTypeForFormat(t *testing.T) {
	cases := map[string]string{
		"markdown": "text/markdown; charset=utf-8",
		"md":       "text/markdown; charset=utf-8",
		"docx":     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"json":     "application/json",
		"bin":      "application/octet-stream",
	}
	for in, want := range cases {
		if got := artifacts.ContentTypeForFormat(in); got != want {
			t.Fatalf("%s: got %q want %q", in, got, want)
		}
	}
}
