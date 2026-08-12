package httocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/thanhtai9606/DocForge/apps/api/internal/orchestrator"
)

// Provider calls an isolated OCR sidecar over HTTP. It never writes job/DB state.
type Provider struct {
	BaseURL string
	Client  *http.Client
	name    string
	version string
}

func New(baseURL string, timeout time.Duration) *Provider {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &Provider{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Client:  &http.Client{Timeout: timeout},
		name:    "http-ocr",
		version: "1.0.0",
	}
}

func (p *Provider) Name() string    { return p.name }
func (p *Provider) Version() string { return p.version }

type ocrResponse struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	Language   string  `json:"language"`
	Provider   string  `json:"provider"`
	Version    string  `json:"version"`
	Error      string  `json:"error"`
}

func (p *Provider) Process(ctx context.Context, req orchestrator.OCRRequest) (orchestrator.OCRResult, error) {
	if p.BaseURL == "" {
		return orchestrator.OCRResult{}, fmt.Errorf("ocr sidecar URL is empty")
	}
	if req.PageNumber < 1 {
		return orchestrator.OCRResult{}, fmt.Errorf("invalid page number")
	}
	if len(req.Payload) == 0 {
		return orchestrator.OCRResult{}, fmt.Errorf("pdf payload is empty")
	}
	body, err := json.Marshal(map[string]any{
		"document_id": req.DocumentID,
		"page_number": req.PageNumber,
		"language":    req.Language,
		"pdf_base64":  base64.StdEncoding.EncodeToString(req.Payload),
	})
	if err != nil {
		return orchestrator.OCRResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/v1/ocr", bytes.NewReader(body))
	if err != nil {
		return orchestrator.OCRResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return orchestrator.OCRResult{}, fmt.Errorf("ocr sidecar: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return orchestrator.OCRResult{}, err
	}
	var out ocrResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return orchestrator.OCRResult{}, fmt.Errorf("ocr sidecar invalid json (status %d)", resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		msg := out.Error
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return orchestrator.OCRResult{}, fmt.Errorf("ocr sidecar status %d: %s", resp.StatusCode, msg)
	}
	if out.Provider != "" {
		p.name = out.Provider
	}
	if out.Version != "" {
		p.version = out.Version
	}
	lang := out.Language
	if lang == "" && len(req.Language) > 0 {
		lang = req.Language[0]
	}
	conf := out.Confidence
	if conf < 0 {
		conf = 0
	}
	if conf > 1 {
		conf = 1
	}
	return orchestrator.OCRResult{Text: out.Text, Confidence: conf, Language: lang}, nil
}

var _ orchestrator.OCRProvider = (*Provider)(nil)
