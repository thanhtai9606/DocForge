package ocrselect_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/thanhtai9606/DocForge/apps/api/internal/config"
	"github.com/thanhtai9606/DocForge/apps/api/internal/providers/ocrselect"
)

func TestSelectStub(t *testing.T) {
	p := ocrselect.New(config.Config{OCRProvider: "stub"}, slog.Default())
	if p.Name() != "stub-ocr" {
		t.Fatalf("name=%s", p.Name())
	}
}

func TestSelectHTTP(t *testing.T) {
	p := ocrselect.New(config.Config{OCRProvider: "http", OCRURL: "http://ocr:8090"}, slog.Default())
	if p.Name() != "http-ocr" {
		t.Fatalf("name=%s", p.Name())
	}
}

func TestSelectHTTPRequiresURL(t *testing.T) {
	p := ocrselect.New(config.Config{OCRProvider: "http"}, slog.Default())
	if p.Name() != "stub-ocr" {
		t.Fatalf("name=%s", p.Name())
	}
}

func TestSelectAutoPrefersURL(t *testing.T) {
	p := ocrselect.New(config.Config{OCRProvider: "auto", OCRURL: "http://127.0.0.1:8090"}, slog.Default())
	if p.Name() != "http-ocr" {
		t.Fatalf("name=%s", p.Name())
	}
}

func TestSelectAutoStubWithoutBinaries(t *testing.T) {
	t.Setenv("PATH", os.TempDir())
	p := ocrselect.New(config.Config{OCRProvider: "auto"}, slog.Default())
	if p.Name() != "stub-ocr" {
		t.Fatalf("name=%s", p.Name())
	}
}
