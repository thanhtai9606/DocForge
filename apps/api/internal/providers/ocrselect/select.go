package ocrselect

import (
	"log/slog"
	"strings"

	"github.com/thanhtai9606/DocForge/apps/api/internal/config"
	"github.com/thanhtai9606/DocForge/apps/api/internal/orchestrator"
	"github.com/thanhtai9606/DocForge/apps/api/internal/providers/httocr"
	"github.com/thanhtai9606/DocForge/apps/api/internal/providers/stubocr"
	"github.com/thanhtai9606/DocForge/apps/api/internal/providers/tesseract"
)

// New picks an OCR provider. Selection stays outside providers themselves.
// auto: OCR_URL → HTTP sidecar, else local tesseract+pdftoppm, else stub.
func New(cfg config.Config, logger *slog.Logger) orchestrator.OCRProvider {
	if logger == nil {
		logger = slog.Default()
	}
	name := strings.ToLower(strings.TrimSpace(cfg.OCRProvider))
	if name == "" {
		name = "auto"
	}
	switch name {
	case "stub":
		logger.Warn("using stub OCR provider")
		return stubocr.New()
	case "http", "sidecar":
		if strings.TrimSpace(cfg.OCRURL) == "" {
			logger.Warn("OCR_URL required for http provider; falling back to stub")
			return stubocr.New()
		}
		logger.Info("using HTTP OCR sidecar", "url", cfg.OCRURL)
		return httocr.New(cfg.OCRURL, cfg.OCRTimeout)
	case "tesseract":
		p := tesseract.New()
		if !p.Available() {
			logger.Warn("tesseract/pdftoppm not on PATH; falling back to stub")
			return stubocr.New()
		}
		logger.Info("using local tesseract OCR")
		return p
	default:
		if strings.TrimSpace(cfg.OCRURL) != "" {
			logger.Info("auto OCR: HTTP sidecar", "url", cfg.OCRURL)
			return httocr.New(cfg.OCRURL, cfg.OCRTimeout)
		}
		p := tesseract.New()
		if p.Available() {
			logger.Info("auto OCR: local tesseract")
			return p
		}
		logger.Warn("auto OCR: stub (install tesseract+pdftoppm or set OCR_URL)")
		return stubocr.New()
	}
}
