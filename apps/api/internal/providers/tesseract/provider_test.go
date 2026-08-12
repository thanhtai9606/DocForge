package tesseract_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thanhtai9606/DocForge/apps/api/internal/orchestrator"
	"github.com/thanhtai9606/DocForge/apps/api/internal/providers/tesseract"
)

func TestAvailable(t *testing.T) {
	p := tesseract.New()
	p.LookPath = func(file string) (string, error) {
		if file == "tesseract" || file == "pdftoppm" {
			return "/usr/bin/" + file, nil
		}
		return "", os.ErrNotExist
	}
	if !p.Available() {
		t.Fatal("expected available")
	}
	p.LookPath = func(string) (string, error) { return "", os.ErrNotExist }
	if p.Available() {
		t.Fatal("expected unavailable")
	}
}

func TestProcessMockedBinaries(t *testing.T) {
	p := tesseract.New()
	p.LookPath = func(file string) (string, error) { return "/bin/" + file, nil }
	p.Run = func(_ context.Context, name string, args []string, _ []byte) ([]byte, error) {
		if name == "pdftoppm" {
			prefix := args[len(args)-1]
			if err := os.WriteFile(prefix+"-1.png", []byte("fake-png"), 0o600); err != nil {
				return nil, err
			}
			return nil, nil
		}
		if name == "tesseract" {
			outBase := args[1]
			tsv := "level\tconf\ttext\n5\t88\tHello\n5\t72\tWorld\n"
			if err := os.WriteFile(outBase+".tsv", []byte(tsv), 0o600); err != nil {
				return nil, err
			}
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
	res, err := p.Process(context.Background(), orchestrator.OCRRequest{
		PageNumber: 1, Language: []string{"en"}, Payload: []byte("%PDF-1.4"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "Hello World" {
		t.Fatalf("text=%q", res.Text)
	}
	if res.Confidence < 0.7 || res.Confidence > 0.9 {
		t.Fatalf("conf=%v", res.Confidence)
	}
	if res.Language != "eng" {
		t.Fatalf("lang=%s", res.Language)
	}
}

func TestProcessWritesTempPDF(t *testing.T) {
	var sawPDF bool
	p := tesseract.New()
	p.Run = func(_ context.Context, name string, args []string, _ []byte) ([]byte, error) {
		if name == "pdftoppm" {
			pdfPath := args[len(args)-2]
			b, err := os.ReadFile(pdfPath)
			if err != nil {
				return nil, err
			}
			sawPDF = string(b) == "%PDF-fake"
			prefix := args[len(args)-1]
			return nil, os.WriteFile(prefix+"-1.png", []byte("x"), 0o600)
		}
		outBase := args[1]
		_ = os.MkdirAll(filepath.Dir(outBase), 0o700)
		return nil, os.WriteFile(outBase+".tsv", []byte("level\tconf\ttext\n5\t50\tA\n"), 0o600)
	}
	_, err := p.Process(context.Background(), orchestrator.OCRRequest{PageNumber: 1, Payload: []byte("%PDF-fake")})
	if err != nil {
		t.Fatal(err)
	}
	if !sawPDF {
		t.Fatal("pdftoppm did not receive temp PDF bytes")
	}
}

func TestInvalidPage(t *testing.T) {
	p := tesseract.New()
	_, err := p.Process(context.Background(), orchestrator.OCRRequest{PageNumber: 0, Payload: []byte("%PDF")})
	if err == nil || !strings.Contains(err.Error(), "invalid page") {
		t.Fatalf("err=%v", err)
	}
}
