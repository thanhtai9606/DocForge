package artifacts_test

import (
	"testing"

	"github.com/thanhtai9606/DocForge/apps/api/internal/artifacts"
	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
)

func TestDownloadName(t *testing.T) {
	doc := &domain.Document{Filename: "My Report (final).pdf"}
	art := &domain.Artifact{Kind: "export", Format: "markdown"}
	got := artifacts.DownloadName(doc, art)
	if got != "My_Report_final.md" {
		t.Fatalf("got=%q", got)
	}
	art.Format = "docx"
	if got := artifacts.DownloadName(doc, art); got != "My_Report_final.docx" {
		t.Fatalf("docx=%q", got)
	}
	cdom := &domain.Artifact{Kind: "cdom", Format: "json"}
	if got := artifacts.DownloadName(doc, cdom); got != "My_Report_final.cdom.json" {
		t.Fatalf("cdom=%q", got)
	}
}
