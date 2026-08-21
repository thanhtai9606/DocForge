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

type batchPageResponse struct {
	PageNumber int     `json:"page_number"`
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	Language   string  `json:"language"`
	Engine     string  `json:"engine"`
}

type batchResponse struct {
	Pages    []batchPageResponse `json:"pages"`
	Provider string              `json:"provider"`
	Version  string              `json:"version"`
	Error    string              `json:"error"`
}

// ProcessBatch OCRs multiple PDF pages in one sidecar call (shared rasterize).
func (p *Provider) ProcessBatch(ctx context.Context, req orchestrator.OCRBatchRequest) (map[int]orchestrator.OCRResult, error) {
	if p.BaseURL == "" {
		return nil, fmt.Errorf("ocr sidecar URL is empty")
	}
	if len(req.PageNumbers) == 0 {
		return nil, fmt.Errorf("page_numbers is empty")
	}
	if len(req.Payload) == 0 {
		return nil, fmt.Errorf("pdf payload is empty")
	}
	body, err := json.Marshal(map[string]any{
		"document_id":   req.DocumentID,
		"page_numbers":  req.PageNumbers,
		"language":      req.Language,
		"pdf_base64":    base64.StdEncoding.EncodeToString(req.Payload),
	})
	if err != nil {
		return nil, err
	}
	timeout := p.batchTimeout(len(req.PageNumbers))
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	} else if client.Timeout < timeout {
		client = &http.Client{
			Timeout:       timeout,
			Transport:     client.Transport,
			CheckRedirect: client.CheckRedirect,
			Jar:           client.Jar,
		}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/v1/ocr/batch", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ocr sidecar batch: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var out batchResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ocr sidecar batch invalid json (status %d)", resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		msg := out.Error
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return nil, fmt.Errorf("ocr sidecar batch status %d: %s", resp.StatusCode, msg)
	}
	if out.Provider != "" {
		p.name = out.Provider
	}
	if out.Version != "" {
		p.version = out.Version
	}
	results := make(map[int]orchestrator.OCRResult, len(out.Pages))
	for _, page := range out.Pages {
		if page.PageNumber < 1 {
			continue
		}
		conf := page.Confidence
		if conf < 0 {
			conf = 0
		}
		if conf > 1 {
			conf = 1
		}
		lang := page.Language
		if lang == "" && len(req.Language) > 0 {
			lang = req.Language[0]
		}
		results[page.PageNumber] = orchestrator.OCRResult{
			Text:       page.Text,
			Confidence: conf,
			Language:   lang,
		}
	}
	if len(results) != len(req.PageNumbers) {
		return nil, fmt.Errorf("ocr sidecar batch returned %d/%d pages", len(results), len(req.PageNumbers))
	}
	return results, nil
}

func (p *Provider) batchTimeout(pageCount int) time.Duration {
	base := p.Client.Timeout
	if base <= 0 {
		base = 120 * time.Second
	}
	// Scale with page count; cap at 20 minutes for large scans.
	scaled := time.Duration(pageCount) * (base / 2)
	if scaled < base {
		scaled = base
	}
	if scaled > 20*time.Minute {
		scaled = 20 * time.Minute
	}
	return scaled
}

var _ orchestrator.OCRBatchProcessor = (*Provider)(nil)
