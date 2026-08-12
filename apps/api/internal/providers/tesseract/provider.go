package tesseract

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/thanhtai9606/DocForge/apps/api/internal/orchestrator"
)

// Runner executes a host binary. Tests inject a fake.
type Runner func(ctx context.Context, name string, args []string, stdin []byte) (stdout []byte, err error)

// Provider OCRs a PDF page via pdftoppm + tesseract on the host (CPU, no CGO).
type Provider struct {
	LookPath func(file string) (string, error)
	Run      Runner
	DPI      int
}

func New() *Provider {
	return &Provider{
		LookPath: exec.LookPath,
		Run:      defaultRun,
		DPI:      200,
	}
}

func (p *Provider) Name() string    { return "tesseract" }
func (p *Provider) Version() string { return "cli" }

func (p *Provider) Available() bool {
	if p.LookPath == nil {
		return false
	}
	if _, err := p.LookPath("tesseract"); err != nil {
		return false
	}
	if _, err := p.LookPath("pdftoppm"); err != nil {
		return false
	}
	return true
}

func (p *Provider) Process(ctx context.Context, req orchestrator.OCRRequest) (orchestrator.OCRResult, error) {
	if req.PageNumber < 1 {
		return orchestrator.OCRResult{}, fmt.Errorf("invalid page number")
	}
	if len(req.Payload) == 0 {
		return orchestrator.OCRResult{}, fmt.Errorf("pdf payload is empty")
	}
	if p.Run == nil {
		p.Run = defaultRun
	}
	dpi := p.DPI
	if dpi < 72 {
		dpi = 200
	}
	dir, err := os.MkdirTemp("", "docforge-ocr-*")
	if err != nil {
		return orchestrator.OCRResult{}, err
	}
	defer os.RemoveAll(dir)

	pdfPath := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(pdfPath, req.Payload, 0o600); err != nil {
		return orchestrator.OCRResult{}, err
	}
	prefix := filepath.Join(dir, "page")
	if _, err := p.Run(ctx, "pdftoppm", []string{
		"-f", strconv.Itoa(req.PageNumber),
		"-l", strconv.Itoa(req.PageNumber),
		"-png", "-r", strconv.Itoa(dpi),
		pdfPath, prefix,
	}, nil); err != nil {
		return orchestrator.OCRResult{}, fmt.Errorf("pdftoppm: %w", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "page*.png"))
	if len(matches) == 0 {
		return orchestrator.OCRResult{}, fmt.Errorf("pdftoppm produced no page image")
	}

	tsvBase := filepath.Join(dir, "out")
	if _, err := p.Run(ctx, "tesseract", []string{
		matches[0], tsvBase, "-l", tessLangs(req.Language), "--psm", "3", "tsv",
	}, nil); err != nil {
		return orchestrator.OCRResult{}, fmt.Errorf("tesseract: %w", err)
	}
	tsv, err := os.ReadFile(tsvBase + ".tsv")
	if err != nil {
		return orchestrator.OCRResult{}, err
	}
	text, conf := parseTSV(tsv)
	lang := "und"
	if len(req.Language) > 0 && req.Language[0] != "" {
		lang = tessLangs([]string{req.Language[0]})
	}
	return orchestrator.OCRResult{Text: text, Confidence: conf, Language: lang}, nil
}

func tessLangs(langs []string) string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(langs))
	for _, l := range langs {
		code := mapLang(l)
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	if len(out) == 0 {
		return "eng"
	}
	return strings.Join(out, "+")
}

func mapLang(l string) string {
	switch strings.ToLower(strings.TrimSpace(l)) {
	case "vi", "vie", "vn":
		return "vie"
	case "en", "eng":
		return "eng"
	case "":
		return "eng"
	default:
		return strings.ToLower(strings.TrimSpace(l))
	}
}

func parseTSV(tsv []byte) (string, float64) {
	lines := bytes.Split(tsv, []byte("\n"))
	if len(lines) < 2 {
		return strings.TrimSpace(string(tsv)), 0
	}
	header := strings.Split(string(lines[0]), "\t")
	confI, textI := -1, -1
	for i, h := range header {
		switch h {
		case "conf":
			confI = i
		case "text":
			textI = i
		}
	}
	if confI < 0 || textI < 0 {
		return strings.TrimSpace(string(tsv)), 0
	}
	words := make([]string, 0)
	var sum float64
	var n int
	for _, line := range lines[1:] {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		cols := strings.Split(string(line), "\t")
		if len(cols) <= confI || len(cols) <= textI {
			continue
		}
		text := strings.TrimSpace(cols[textI])
		conf, err := strconv.ParseFloat(cols[confI], 64)
		if err != nil || conf < 0 || text == "" {
			continue
		}
		words = append(words, text)
		sum += conf / 100.0
		n++
	}
	avg := 0.0
	if n > 0 {
		avg = sum / float64(n)
	}
	return strings.Join(words, " "), avg
}

func defaultRun(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s: %w (%s)", name, err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

var _ orchestrator.OCRProvider = (*Provider)(nil)
