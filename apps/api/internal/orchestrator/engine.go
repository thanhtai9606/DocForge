package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/jobs"
	"github.com/thanhtai9606/DocForge/apps/api/internal/storage"
	"github.com/thanhtai9606/DocForge/packages/cdom"
)

// Stage names from PROJECT_SPEC §3.
const (
	StageDetect     = "detect"
	StageExtract    = "extract"
	StageOCR        = "ocr"
	StageNormalize  = "normalize"
	StagePostprocess = "postprocess"
	StageExport     = "export"
)

// DocKind is the detect-stage classification.
type DocKind string

const (
	KindDigital DocKind = "digital"
	KindScanned DocKind = "scanned"
	KindMixed   DocKind = "mixed"
)

// OCRProvider is the replaceable OCR contract (providers must stay isolated).
type OCRProvider interface {
	Name() string
	Version() string
	Process(ctx context.Context, req OCRRequest) (OCRResult, error)
}

type OCRRequest struct {
	DocumentID string
	PageNumber int
	AssetURI   string
	Language   []string
	Payload    []byte
}

type OCRResult struct {
	Text       string
	Confidence float64
	Language   string
}

// Engine runs the processing pipeline for one job.
type Engine struct {
	Documents jobs.DocumentRepository
	Jobs      jobs.JobRepository
	Artifacts jobs.ArtifactRepository
	Objects   storage.ObjectStore
	Progress  jobs.ProgressStore
	OCR       OCRProvider
	Now       func() time.Time
}

type RunResult struct {
	Kind       DocKind
	CDOMKey    string
	ArtifactID string
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now().UTC()
}

// Run executes detect → extract/ocr → normalize for a queued/processing job.
func (e *Engine) Run(ctx context.Context, jobID string) (*RunResult, error) {
	job, err := e.Jobs.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.Status == domain.JobCancelled || job.Status == domain.JobCompleted {
		return &RunResult{}, nil
	}
	doc, err := e.Documents.Get(ctx, job.DocumentID)
	if err != nil {
		return nil, err
	}

	if job.Status == domain.JobQueued {
		if err := job.Transition(domain.JobProcessing); err != nil {
			return nil, err
		}
		doc.Status = domain.DocumentProcessing
		doc.UpdatedAt = e.now()
		if err := e.Documents.Update(ctx, doc); err != nil {
			return nil, err
		}
		if err := e.Jobs.Update(ctx, job); err != nil {
			return nil, err
		}
	}

	if err := e.setProgress(ctx, job, StageDetect, 10); err != nil {
		return nil, err
	}
	if cancelled, err := e.isCancelled(ctx, job.ID); err != nil {
		return nil, err
	} else if cancelled {
		return e.markCancelled(ctx, job, doc)
	}

	pdf, err := e.readObject(ctx, doc.StorageKey)
	if err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeStorageError, err.Error(), true)
	}

	kind, pageCount, pagesText, err := DetectAndExtract(pdf)
	if err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeInvalidPDF, err.Error(), false)
	}
	doc.PageCount = pageCount
	doc.UpdatedAt = e.now()
	_ = e.Documents.Update(ctx, doc)

	var blocks [][]extractedBlock
	switch kind {
	case KindDigital, KindMixed:
		if err := e.setProgress(ctx, job, StageExtract, 40); err != nil {
			return nil, err
		}
		blocks = pagesText
	case KindScanned:
		if err := e.setProgress(ctx, job, StageOCR, 40); err != nil {
			return nil, err
		}
		blocks, err = e.runOCR(ctx, doc, pdf, pageCount)
		if err != nil {
			return nil, e.fail(ctx, job, doc, domain.CodeOCRProviderError, err.Error(), true)
		}
	}

	if err := e.setProgress(ctx, job, StageNormalize, 75); err != nil {
		return nil, err
	}
	cdomDoc := NormalizeToCDOM(doc, kind, blocks, e.OCR)
	if err := cdomDoc.Validate(); err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeNormalizationFailed, err.Error(), false)
	}

	raw, err := json.MarshalIndent(cdomDoc, "", "  ")
	if err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeNormalizationFailed, err.Error(), false)
	}
	cdomKey := path.Join("documents", doc.ID, "cdom.json")
	if err := e.Objects.Put(ctx, cdomKey, bytes.NewReader(raw), int64(len(raw)), "application/json"); err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeStorageError, err.Error(), true)
	}

	artifactID := uuid.NewString()
	art := &domain.Artifact{
		ID:         artifactID,
		DocumentID: doc.ID,
		JobID:      job.ID,
		Kind:       "cdom",
		Format:     "json",
		StorageKey: cdomKey,
		SizeBytes:  int64(len(raw)),
		CreatedAt:  e.now(),
	}
	if err := e.Artifacts.Create(ctx, art); err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeStorageError, err.Error(), true)
	}

	// Phase 2 stops before full export workers; mark export staged as queued-complete for CDOM.
	if err := e.setProgress(ctx, job, StageExport, 95); err != nil {
		return nil, err
	}

	if err := job.Transition(domain.JobCompleted); err != nil {
		return nil, err
	}
	job.Stage = "completed"
	job.Progress = 100
	job.ErrorCode = ""
	job.ErrorMsg = ""
	job.UpdatedAt = e.now()
	if err := e.Jobs.Update(ctx, job); err != nil {
		return nil, err
	}
	doc.Status = domain.DocumentCompleted
	doc.UpdatedAt = e.now()
	if err := e.Documents.Update(ctx, doc); err != nil {
		return nil, err
	}
	_ = e.setProgress(ctx, job, "completed", 100)

	return &RunResult{Kind: kind, CDOMKey: cdomKey, ArtifactID: artifactID}, nil
}

func (e *Engine) runOCR(ctx context.Context, doc *domain.Document, pdf []byte, pageCount int) ([][]extractedBlock, error) {
	if e.OCR == nil {
		return nil, fmt.Errorf("ocr provider not configured")
	}
	if pageCount < 1 {
		pageCount = 1
	}
	out := make([][]extractedBlock, pageCount)
	for i := 0; i < pageCount; i++ {
		res, err := e.OCR.Process(ctx, OCRRequest{
			DocumentID: doc.ID,
			PageNumber: i + 1,
			AssetURI:   doc.StorageKey,
			Language:   []string{"vi", "en"},
			Payload:    pdf,
		})
		if err != nil {
			return nil, err
		}
		out[i] = []extractedBlock{{
			Text:       res.Text,
			Confidence: res.Confidence,
			Language:   res.Language,
			Type:       cdom.BlockParagraph,
		}}
	}
	return out, nil
}

func (e *Engine) readObject(ctx context.Context, key string) ([]byte, error) {
	rc, err := e.Objects.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (e *Engine) setProgress(ctx context.Context, job *domain.Job, stage string, progress int) error {
	job.Stage = stage
	job.Progress = progress
	job.UpdatedAt = e.now()
	if err := e.Jobs.Update(ctx, job); err != nil {
		return err
	}
	if e.Progress != nil {
		_ = e.Progress.SetProgress(ctx, job.ID, stage, progress)
	}
	return nil
}

func (e *Engine) isCancelled(ctx context.Context, jobID string) (bool, error) {
	job, err := e.Jobs.Get(ctx, jobID)
	if err != nil {
		return false, err
	}
	return job.Status == domain.JobCancelled, nil
}

func (e *Engine) markCancelled(ctx context.Context, job *domain.Job, doc *domain.Document) (*RunResult, error) {
	job.Stage = "cancelled"
	job.UpdatedAt = e.now()
	_ = e.Jobs.Update(ctx, job)
	doc.Status = domain.DocumentCancelled
	doc.UpdatedAt = e.now()
	_ = e.Documents.Update(ctx, doc)
	return &RunResult{}, nil
}

func (e *Engine) fail(ctx context.Context, job *domain.Job, doc *domain.Document, code, msg string, retryable bool) error {
	_ = retryable
	if job.Status == domain.JobProcessing || job.Status == domain.JobQueued {
		_ = job.Transition(domain.JobFailed)
	} else {
		job.Status = domain.JobFailed
	}
	job.ErrorCode = code
	job.ErrorMsg = msg
	job.Stage = "failed"
	job.UpdatedAt = e.now()
	_ = e.Jobs.Update(ctx, job)
	doc.Status = domain.DocumentFailed
	doc.UpdatedAt = e.now()
	_ = e.Documents.Update(ctx, doc)
	if e.Progress != nil {
		_ = e.Progress.SetProgress(ctx, job.ID, "failed", job.Progress)
	}
	return domain.NewAppError(code, msg, retryable)
}

type extractedBlock struct {
	Text       string
	Confidence float64
	Language   string
	Type       cdom.BlockType
}

// DetectAndExtract classifies the PDF and returns per-page text blocks when available.
func DetectAndExtract(pdf []byte) (DocKind, int, [][]extractedBlock, error) {
	if len(pdf) < 4 || string(pdf[:4]) != "%PDF" {
		return "", 0, nil, fmt.Errorf("not a PDF")
	}
	pages := splitPDFTextHeuristic(pdf)
	if len(pages) == 0 {
		pages = []string{""}
	}
	pageCount := len(pages)
	digitalPages := 0
	blocks := make([][]extractedBlock, pageCount)
	for i, text := range pages {
		text = strings.TrimSpace(text)
		runes := utf8.RuneCountInString(text)
		if runes >= 40 {
			digitalPages++
			blocks[i] = splitParagraphs(text)
		} else {
			blocks[i] = nil
		}
	}
	kind := KindScanned
	switch {
	case digitalPages == pageCount:
		kind = KindDigital
	case digitalPages == 0:
		kind = KindScanned
	default:
		kind = KindMixed
	}
	return kind, pageCount, blocks, nil
}

func splitParagraphs(text string) []extractedBlock {
	parts := strings.Split(text, "\n\n")
	out := make([]extractedBlock, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, extractedBlock{
			Text:       p,
			Confidence: 0.95,
			Language:   detectLangHint(p),
			Type:       cdom.BlockParagraph,
		})
	}
	if len(out) == 0 && strings.TrimSpace(text) != "" {
		out = append(out, extractedBlock{Text: strings.TrimSpace(text), Confidence: 0.9, Language: "und", Type: cdom.BlockParagraph})
	}
	return out
}

func detectLangHint(text string) string {
	for _, r := range text {
		if r >= 0x0100 {
			return "vi"
		}
	}
	return "en"
}

// splitPDFTextHeuristic extracts rough text streams without a full PDF parser.
// Good enough for digital vs scanned detection in Phase 2; native extractors can replace later.
func splitPDFTextHeuristic(pdf []byte) []string {
	content := string(pdf)
	// Count pages via /Type /Page roughly.
	pageCount := strings.Count(content, "/Type /Page")
	if pageCount == 0 {
		pageCount = strings.Count(content, "/Type/Page")
	}
	if pageCount < 1 {
		pageCount = 1
	}

	var texts []string
	// Pull text from PDF literal strings (...) which often hold content in simple digital PDFs.
	var b strings.Builder
	in := false
	esc := false
	for i := 0; i < len(content); i++ {
		ch := content[i]
		if !in {
			if ch == '(' {
				in = true
				esc = false
			}
			continue
		}
		if esc {
			b.WriteByte(ch)
			esc = false
			continue
		}
		if ch == '\\' {
			esc = true
			continue
		}
		if ch == ')' {
			in = false
			b.WriteByte('\n')
			continue
		}
		if ch == '\r' || ch == '\n' {
			continue
		}
		b.WriteByte(ch)
	}
	full := strings.TrimSpace(b.String())
	if full == "" {
		out := make([]string, pageCount)
		return out
	}
	// Distribute text across pages naively.
	chunks := strings.Split(full, "\n")
	per := len(chunks) / pageCount
	if per < 1 {
		per = 1
	}
	texts = make([]string, pageCount)
	for i := 0; i < pageCount; i++ {
		start := i * per
		end := start + per
		if i == pageCount-1 || end > len(chunks) {
			end = len(chunks)
		}
		if start >= len(chunks) {
			texts[i] = ""
			continue
		}
		texts[i] = strings.TrimSpace(strings.Join(chunks[start:end], "\n"))
	}
	return texts
}

// NormalizeToCDOM builds a CDOM document from extracted/OCR blocks.
func NormalizeToCDOM(doc *domain.Document, kind DocKind, pages [][]extractedBlock, ocr OCRProvider) *cdom.Document {
	out := &cdom.Document{
		SchemaVersion: cdom.SchemaVersion,
		DocumentID:    doc.ID,
		Source: cdom.Source{
			Filename:  doc.Filename,
			MimeType:  doc.ContentType,
			PageCount: len(pages),
		},
		Metadata: map[string]any{
			"document_kind": string(kind),
		},
		Processing: map[string]any{
			"provider_name":      "docforge-orchestrator",
			"provider_version":   "0.2.0",
			"model_name":         "heuristic-extract",
			"model_version":      "1",
			"cdm_schema_version": cdom.SchemaVersion,
		},
		Pages: make([]cdom.Page, 0, len(pages)),
	}
	if kind == KindScanned && ocr != nil {
		out.Processing["provider_name"] = ocr.Name()
		out.Processing["provider_version"] = ocr.Version()
		out.Processing["model_name"] = ocr.Name()
	}
	order := 0
	for i, pageBlocks := range pages {
		page := cdom.Page{
			PageNumber: i + 1,
			Width:      612,
			Height:     792,
			Language:   "und",
			Blocks:     make([]cdom.Block, 0, len(pageBlocks)),
		}
		for j, blk := range pageBlocks {
			order++
			lang := blk.Language
			if lang == "" {
				lang = "und"
			}
			if j == 0 {
				page.Language = lang
			}
			page.Blocks = append(page.Blocks, cdom.Block{
				ID:           fmt.Sprintf("p%d-b%d", i+1, j+1),
				Type:         blk.Type,
				BBox:         cdom.BBox{72, float64(720 - j*40), 540, float64(750 - j*40)},
				ReadingOrder: order,
				Confidence:   blk.Confidence,
				Language:     lang,
				Text:         blk.Text,
			})
		}
		out.Pages = append(out.Pages, page)
	}
	if out.Source.PageCount == 0 {
		out.Source.PageCount = len(out.Pages)
	}
	return out
}
