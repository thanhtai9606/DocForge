package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/export"
	"github.com/thanhtai9606/DocForge/apps/api/internal/jobs"
	"github.com/thanhtai9606/DocForge/apps/api/internal/layout"
	"github.com/thanhtai9606/DocForge/apps/api/internal/pdfx"
	"github.com/thanhtai9606/DocForge/apps/api/internal/storage"
	"github.com/thanhtai9606/DocForge/packages/cdom"
)

const (
	StageDetect      = "detect"
	StageExtract     = "extract"
	StageOCR         = "ocr"
	StageLayout      = "layout"
	StageNormalize   = "normalize"
	StagePostprocess = "postprocess"
	StageExport      = "export"
)

type DocKind string

const (
	KindDigital DocKind = "digital"
	KindScanned DocKind = "scanned"
	KindMixed   DocKind = "mixed"
)

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

// Engine runs a production processing pipeline for one job.
type Engine struct {
	Documents jobs.DocumentRepository
	Jobs      jobs.JobRepository
	Artifacts jobs.ArtifactRepository
	Objects   storage.ObjectStore
	Progress  jobs.ProgressStore
	OCR       OCRProvider
	Layout    layout.Provider
	Exporters []export.Exporter
	// MaxParallelPages bounds page-level fan-out for extract/OCR.
	MaxParallelPages int
	Now              func() time.Time
}

type RunResult struct {
	Kind        DocKind
	CDOMKey     string
	ArtifactIDs []string
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now().UTC()
}

func (e *Engine) parallelPages() int {
	n := e.MaxParallelPages
	if n < 1 {
		n = runtime.GOMAXPROCS(0)
		if n < 2 {
			n = 2
		}
		if n > 8 {
			n = 8
		}
	}
	return n
}

// Run executes detect → extract/ocr → layout → normalize → export with idempotent artifact reuse.
func (e *Engine) Run(ctx context.Context, jobID string) (*RunResult, error) {
	start := e.now()
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
		job.Attempts++
		if err := e.Jobs.Update(ctx, job); err != nil {
			return nil, err
		}
	}

	cdomKey := path.Join("documents", doc.ID, "cdom.json")
	if existing := e.findArtifact(ctx, doc.ID, "cdom", "json"); existing != nil {
		// Idempotent resume: CDOM already produced for this document.
		if err := e.setProgress(ctx, job, StageExport, 90); err != nil {
			return nil, err
		}
		ids, err := e.ensureExports(ctx, job, doc, cdomKey)
		if err != nil {
			return nil, err
		}
		return e.complete(ctx, job, doc, KindDigital, cdomKey, ids, start)
	}

	if err := e.checkpoint(ctx, job, StageDetect, 5); err != nil {
		return e.handleStepErr(ctx, job, doc, err)
	}

	pdfBytes, err := e.readObject(ctx, doc.StorageKey)
	if err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeStorageError, err.Error(), true)
	}

	extracted, err := pdfx.Extract(pdfBytes, pdfx.ExtractOptions{MaxParallelPages: e.parallelPages()})
	if err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeInvalidPDF, err.Error(), false)
	}
	kindStr, _, _ := pdfx.Classify(extracted, 40)
	kind := DocKind(kindStr)
	doc.PageCount = extracted.PageCount
	doc.UpdatedAt = e.now()
	_ = e.Documents.Update(ctx, doc)

	cdomDoc := &cdom.Document{
		SchemaVersion: cdom.SchemaVersion,
		DocumentID:    doc.ID,
		Source: cdom.Source{
			Filename:  doc.Filename,
			MimeType:  doc.ContentType,
			PageCount: extracted.PageCount,
		},
		Metadata: map[string]any{
			"document_kind": string(kind),
		},
		Processing: map[string]any{
			"provider_name":      "docforge-orchestrator",
			"provider_version":   "0.3.0",
			"model_name":         "rsc-pdf-extract",
			"model_version":      "1",
			"cdm_schema_version": cdom.SchemaVersion,
		},
		Pages: make([]cdom.Page, 0, extracted.PageCount),
	}

	switch kind {
	case KindDigital:
		if err := e.checkpoint(ctx, job, StageExtract, 35); err != nil {
			return e.handleStepErr(ctx, job, doc, err)
		}
		cdomDoc.Pages = pagesFromExtract(extracted)
	case KindMixed:
		if err := e.checkpoint(ctx, job, StageExtract, 30); err != nil {
			return e.handleStepErr(ctx, job, doc, err)
		}
		pages, err := e.fillMixedPages(ctx, doc, pdfBytes, extracted)
		if err != nil {
			return nil, e.fail(ctx, job, doc, domain.CodeOCRProviderError, err.Error(), true)
		}
		cdomDoc.Pages = pages
		cdomDoc.Processing["model_name"] = "hybrid-extract-ocr"
	case KindScanned:
		if err := e.checkpoint(ctx, job, StageOCR, 35); err != nil {
			return e.handleStepErr(ctx, job, doc, err)
		}
		pages, err := e.ocrAllPages(ctx, doc, pdfBytes, extracted.PageCount, extracted)
		if err != nil {
			return nil, e.fail(ctx, job, doc, domain.CodeOCRProviderError, err.Error(), true)
		}
		cdomDoc.Pages = pages
		if e.OCR != nil {
			cdomDoc.Processing["provider_name"] = e.OCR.Name()
			cdomDoc.Processing["provider_version"] = e.OCR.Version()
			cdomDoc.Processing["model_name"] = e.OCR.Name()
		}
	}

	if err := e.checkpoint(ctx, job, StageLayout, 60); err != nil {
		return e.handleStepErr(ctx, job, doc, err)
	}
	layoutProvider := e.Layout
	if layoutProvider == nil {
		layoutProvider = layout.NewGeometric()
	}
	if err := layoutProvider.Analyze(ctx, cdomDoc); err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeLayoutFailed, err.Error(), false)
	}

	if err := e.checkpoint(ctx, job, StageNormalize, 75); err != nil {
		return e.handleStepErr(ctx, job, doc, err)
	}
	if err := cdomDoc.Validate(); err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeNormalizationFailed, err.Error(), false)
	}

	raw, err := json.MarshalIndent(cdomDoc, "", "  ")
	if err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeNormalizationFailed, err.Error(), false)
	}
	if err := e.Objects.Put(ctx, cdomKey, bytes.NewReader(raw), int64(len(raw)), "application/json"); err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeStorageError, err.Error(), true)
	}
	cdomArt := &domain.Artifact{
		ID: uuid.NewString(), DocumentID: doc.ID, JobID: job.ID,
		Kind: "cdom", Format: "json", StorageKey: cdomKey, SizeBytes: int64(len(raw)), CreatedAt: e.now(),
	}
	if err := e.Artifacts.Create(ctx, cdomArt); err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeStorageError, err.Error(), true)
	}

	if err := e.checkpoint(ctx, job, StageExport, 90); err != nil {
		return e.handleStepErr(ctx, job, doc, err)
	}
	ids, err := e.ensureExports(ctx, job, doc, cdomKey)
	if err != nil {
		return nil, err
	}
	ids = append([]string{cdomArt.ID}, ids...)
	return e.complete(ctx, job, doc, kind, cdomKey, ids, start)
}

func (e *Engine) handleStepErr(ctx context.Context, job *domain.Job, doc *domain.Document, err error) (*RunResult, error) {
	if appErr, ok := domain.AsAppError(err); ok && appErr.Code == domain.CodeJobCancelled {
		return e.markCancelled(ctx, job, doc)
	}
	return nil, err
}

func pagesFromExtract(extracted *pdfx.DocumentText) []cdom.Page {
	pages := make([]cdom.Page, 0, len(extracted.Pages))
	order := 0
	for _, p := range extracted.Pages {
		blocks, next := pdfx.ToCDOMBlocks(p, order)
		order = next
		pages = append(pages, cdom.Page{
			PageNumber: p.PageNumber,
			Width:      p.Width,
			Height:     p.Height,
			Language:   p.Language,
			Blocks:     blocks,
		})
	}
	return pages
}

func (e *Engine) fillMixedPages(ctx context.Context, doc *domain.Document, pdf []byte, extracted *pdfx.DocumentText) ([]cdom.Page, error) {
	pages := make([]cdom.Page, len(extracted.Pages))
	var eg errgroup.Group
	sem := make(chan struct{}, e.parallelPages())
	var mu sync.Mutex
	var ocrErr error
	orderBase := make([]int, len(extracted.Pages))
	running := 0
	for i, p := range extracted.Pages {
		orderBase[i] = running
		running += len(p.Blocks)
		if p.CharCount < 40 {
			running++ // placeholder for OCR block
		}
	}
	for i, p := range extracted.Pages {
		i, p := i, p
		if p.CharCount >= 40 {
			blocks, _ := pdfx.ToCDOMBlocks(p, orderBase[i])
			pages[i] = cdom.Page{PageNumber: p.PageNumber, Width: p.Width, Height: p.Height, Language: p.Language, Blocks: blocks}
			continue
		}
		eg.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			blk, err := e.ocrPage(ctx, doc, pdf, p.PageNumber)
			if err != nil {
				mu.Lock()
				ocrErr = err
				mu.Unlock()
				return err
			}
			pages[i] = cdom.Page{
				PageNumber: p.PageNumber, Width: p.Width, Height: p.Height, Language: blk.Language,
				Blocks: []cdom.Block{{
					ID: fmt.Sprintf("p%d-ocr1", p.PageNumber), Type: cdom.BlockParagraph,
					BBox: cdom.BBox{72, 72, p.Width - 72, p.Height - 72}, ReadingOrder: orderBase[i] + 1,
					Confidence: blk.Confidence, Language: blk.Language, Text: blk.Text,
				}},
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return pages, ocrErr
}

func (e *Engine) ocrAllPages(ctx context.Context, doc *domain.Document, pdf []byte, pageCount int, extracted *pdfx.DocumentText) ([]cdom.Page, error) {
	if e.OCR == nil {
		return nil, fmt.Errorf("ocr provider not configured")
	}
	pages := make([]cdom.Page, pageCount)
	var eg errgroup.Group
	sem := make(chan struct{}, e.parallelPages())
	for i := 0; i < pageCount; i++ {
		i := i
		width, height := 612.0, 792.0
		if extracted != nil && i < len(extracted.Pages) {
			width, height = extracted.Pages[i].Width, extracted.Pages[i].Height
		}
		eg.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := e.ocrPage(ctx, doc, pdf, i+1)
			if err != nil {
				return err
			}
			pages[i] = cdom.Page{
				PageNumber: i + 1, Width: width, Height: height, Language: res.Language,
				Blocks: []cdom.Block{{
					ID: fmt.Sprintf("p%d-ocr1", i+1), Type: cdom.BlockParagraph,
					BBox: cdom.BBox{72, 72, width - 72, height - 72}, ReadingOrder: i + 1,
					Confidence: res.Confidence, Language: res.Language, Text: res.Text,
				}},
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return pages, nil
}

func (e *Engine) ocrPage(ctx context.Context, doc *domain.Document, pdf []byte, pageNumber int) (OCRResult, error) {
	if e.OCR == nil {
		return OCRResult{}, fmt.Errorf("ocr provider not configured")
	}
	return e.OCR.Process(ctx, OCRRequest{
		DocumentID: doc.ID,
		PageNumber: pageNumber,
		AssetURI:   doc.StorageKey,
		Language:   []string{"vi", "en"},
		Payload:    pdf,
	})
}

func (e *Engine) ensureExports(ctx context.Context, job *domain.Job, doc *domain.Document, cdomKey string) ([]string, error) {
	exporters := e.Exporters
	if len(exporters) == 0 {
		exporters = []export.Exporter{export.NewJSON(), export.NewMarkdown()}
	}
	rc, err := e.Objects.Get(ctx, cdomKey)
	if err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeStorageError, err.Error(), true)
	}
	raw, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeStorageError, err.Error(), true)
	}
	cdomDoc, err := cdom.UnmarshalDocument(raw)
	if err != nil {
		return nil, e.fail(ctx, job, doc, domain.CodeNormalizationFailed, err.Error(), false)
	}

	ids := make([]string, 0, len(exporters))
	for _, ex := range exporters {
		if existing := e.findArtifact(ctx, doc.ID, "export", ex.Format()); existing != nil {
			ids = append(ids, existing.ID)
			continue
		}
		artPayload, err := ex.Export(ctx, cdomDoc)
		if err != nil {
			return nil, e.fail(ctx, job, doc, domain.CodeExportFailed, err.Error(), false)
		}
		key := path.Join("documents", doc.ID, "exports", "output."+extFor(artPayload.Format))
		if err := e.Objects.Put(ctx, key, bytes.NewReader(artPayload.Body), int64(len(artPayload.Body)), artPayload.ContentType); err != nil {
			return nil, e.fail(ctx, job, doc, domain.CodeStorageError, err.Error(), true)
		}
		a := &domain.Artifact{
			ID: uuid.NewString(), DocumentID: doc.ID, JobID: job.ID,
			Kind: "export", Format: artPayload.Format, StorageKey: key,
			SizeBytes: int64(len(artPayload.Body)), CreatedAt: e.now(),
		}
		if err := e.Artifacts.Create(ctx, a); err != nil {
			return nil, e.fail(ctx, job, doc, domain.CodeStorageError, err.Error(), true)
		}
		ids = append(ids, a.ID)
	}
	return ids, nil
}

func extFor(format string) string {
	switch format {
	case "markdown":
		return "md"
	default:
		return format
	}
}

func (e *Engine) findArtifact(ctx context.Context, documentID, kind, format string) *domain.Artifact {
	list, err := e.Artifacts.ListByDocument(ctx, documentID)
	if err != nil {
		return nil
	}
	for i := range list {
		if list[i].Kind == kind && list[i].Format == format {
			return &list[i]
		}
	}
	return nil
}

func (e *Engine) complete(ctx context.Context, job *domain.Job, doc *domain.Document, kind DocKind, cdomKey string, ids []string, start time.Time) (*RunResult, error) {
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
	_ = start
	return &RunResult{Kind: kind, CDOMKey: cdomKey, ArtifactIDs: ids}, nil
}

func (e *Engine) checkpoint(ctx context.Context, job *domain.Job, stage string, progress int) error {
	if cancelled, err := e.isCancelled(ctx, job.ID); err != nil {
		return err
	} else if cancelled {
		return domain.NewAppError(domain.CodeJobCancelled, "job cancelled", false)
	}
	return e.setProgress(ctx, job, stage, progress)
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
