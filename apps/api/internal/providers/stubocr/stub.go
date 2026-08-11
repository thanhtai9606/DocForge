package stubocr

import (
	"context"
	"fmt"

	"github.com/thanhtai9606/DocForge/apps/api/internal/orchestrator"
)

// Provider is a CPU-only placeholder OCR implementation for Phase 2 wiring.
type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string    { return "stub-ocr" }
func (p *Provider) Version() string { return "0.1.0" }

func (p *Provider) Process(_ context.Context, req orchestrator.OCRRequest) (orchestrator.OCRResult, error) {
	if req.PageNumber < 1 {
		return orchestrator.OCRResult{}, fmt.Errorf("invalid page number")
	}
	text := fmt.Sprintf("Stub OCR text for document %s page %d", req.DocumentID, req.PageNumber)
	if len(req.Language) > 0 && req.Language[0] == "vi" {
		text = fmt.Sprintf("Văn bản OCR giả lập cho tài liệu %s trang %d", req.DocumentID, req.PageNumber)
	}
	lang := "und"
	if len(req.Language) > 0 && req.Language[0] != "" {
		lang = req.Language[0]
	}
	return orchestrator.OCRResult{Text: text, Confidence: 0.8, Language: lang}, nil
}

var _ orchestrator.OCRProvider = (*Provider)(nil)
