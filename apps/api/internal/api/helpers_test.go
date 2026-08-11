package api

import "testing"

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
		"markdown": "text/markdown",
		"md":       "text/markdown",
		"docx":     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"json":     "application/json",
		"bin":      "application/octet-stream",
	}
	for in, want := range cases {
		if got := contentTypeForFormat(in); got != want {
			t.Fatalf("%s: got %q want %q", in, got, want)
		}
	}
}
