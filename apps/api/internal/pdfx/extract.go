package pdfx

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/sync/errgroup"
	"rsc.io/pdf"

	"github.com/thanhtai9606/DocForge/packages/cdom"
)

// PageText is structured text extracted from one PDF page.
type PageText struct {
	PageNumber int
	Width      float64
	Height     float64
	Language   string
	CharCount  int
	Blocks     []TextBlock
}

// TextBlock is a positioned text unit suitable for layout/CDOM mapping.
type TextBlock struct {
	Text       string
	X          float64
	Y          float64
	W          float64
	H          float64
	FontSize   float64
	Confidence float64
}

// DocumentText is the full extraction result.
type DocumentText struct {
	PageCount int
	Pages     []PageText
}

// ExtractOptions controls parallel extraction.
type ExtractOptions struct {
	// MaxParallelPages bounds concurrent page parsing (production default: NumCPU capped).
	MaxParallelPages int
}

// Extract reads a PDF from memory and extracts positioned text per page.
func Extract(data []byte, opt ExtractOptions) (*DocumentText, error) {
	if len(data) < 5 || !bytes.HasPrefix(data, []byte("%PDF")) {
		return nil, fmt.Errorf("invalid PDF magic header")
	}
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}
	n := r.NumPage()
	if n < 1 {
		return nil, fmt.Errorf("pdf has no pages")
	}
	parallel := opt.MaxParallelPages
	if parallel < 1 {
		parallel = 4
	}
	if parallel > n {
		parallel = n
	}

	pages := make([]PageText, n)
	var eg errgroup.Group
	sem := make(chan struct{}, parallel)
	var mu sync.Mutex
	var firstErr error

	for i := 1; i <= n; i++ {
		i := i
		eg.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			pt, err := extractPage(r, i)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("page %d: %w", i, err)
				}
				mu.Unlock()
				// Continue other pages; scanned/corrupt pages become empty digital signals.
				pages[i-1] = PageText{PageNumber: i, Width: 612, Height: 792}
				return nil
			}
			pages[i-1] = *pt
			return nil
		})
	}
	_ = eg.Wait()
	if firstErr != nil && allEmpty(pages) {
		return nil, firstErr
	}
	return &DocumentText{PageCount: n, Pages: pages}, nil
}

func allEmpty(pages []PageText) bool {
	for _, p := range pages {
		if p.CharCount > 0 {
			return false
		}
	}
	return true
}

func extractPage(r *pdf.Reader, pageNum int) (*PageText, error) {
	p := r.Page(pageNum)
	if p.V.IsNull() {
		return nil, fmt.Errorf("missing page")
	}
	width, height := pageSize(p)
	content := p.Content()
	texts := content.Text
	sort.Sort(pdf.TextVertical(texts))

	blocks := groupIntoLines(texts)
	charCount := 0
	for _, b := range blocks {
		charCount += utf8.RuneCountInString(b.Text)
	}
	return &PageText{
		PageNumber: pageNum,
		Width:      width,
		Height:     height,
		Language:   guessLanguage(blocks),
		CharCount:  charCount,
		Blocks:     blocks,
	}, nil
}

func pageSize(p pdf.Page) (float64, float64) {
	// MediaBox is often [0 0 width height]
	box := p.V.Key("MediaBox")
	if box.Len() >= 4 {
		x1 := box.Index(0).Float64()
		y1 := box.Index(1).Float64()
		x2 := box.Index(2).Float64()
		y2 := box.Index(3).Float64()
		w := x2 - x1
		h := y2 - y1
		if w > 0 && h > 0 {
			return w, h
		}
	}
	return 612, 792
}

func groupIntoLines(texts []pdf.Text) []TextBlock {
	if len(texts) == 0 {
		return nil
	}
	const yTol = 2.0
	type line struct {
		y     float64
		items []pdf.Text
	}
	var lines []line
	for _, t := range texts {
		placed := false
		for i := range lines {
			if abs(lines[i].y-t.Y) <= yTol {
				lines[i].items = append(lines[i].items, t)
				placed = true
				break
			}
		}
		if !placed {
			lines = append(lines, line{y: t.Y, items: []pdf.Text{t}})
		}
	}
	out := make([]TextBlock, 0, len(lines))
	for _, ln := range lines {
		sort.Sort(pdf.TextHorizontal(ln.items))
		// Split a visual line into column-like segments when horizontal gaps are large.
		// Small gaps stay within a cell; large gaps become separate TextBlocks for table layout.
		type seg struct {
			b          strings.Builder
			minX, maxX float64
			font       float64
			started    bool
		}
		flush := func(s *seg) {
			if !s.started {
				return
			}
			text := strings.TrimSpace(s.b.String())
			if text == "" {
				*s = seg{}
				return
			}
			h := s.font
			if h <= 0 {
				h = 10
			}
			out = append(out, TextBlock{
				Text:       text,
				X:          s.minX,
				Y:          ln.y,
				W:          s.maxX - s.minX,
				H:          h,
				FontSize:   s.font,
				Confidence: 0.98,
			})
			*s = seg{}
		}
		var cur seg
		for _, it := range ln.items {
			font := it.FontSize
			if font <= 0 {
				font = 10
			}
			colGap := font * 2.5
			if colGap < 18 {
				colGap = 18
			}
			if cur.started {
				gap := it.X - cur.maxX
				if gap > colGap {
					flush(&cur)
				} else if gap > font*0.3 {
					cur.b.WriteByte(' ')
				}
			}
			if !cur.started {
				cur.started = true
				cur.minX = it.X
				cur.maxX = it.X + it.W
				cur.font = font
			}
			cur.b.WriteString(it.S)
			if it.X < cur.minX {
				cur.minX = it.X
			}
			right := it.X + it.W
			if right > cur.maxX {
				cur.maxX = right
			}
			if font > cur.font {
				cur.font = font
			}
		}
		flush(&cur)
	}
	return out
}

func guessLanguage(blocks []TextBlock) string {
	for _, b := range blocks {
		for _, r := range b.Text {
			if r >= 0x0100 {
				return "vi"
			}
		}
	}
	if len(blocks) == 0 {
		return "und"
	}
	return "en"
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// Classify returns document kind from per-page character density.
func Classify(doc *DocumentText, minCharsDigital int) (kind string, digitalPages, scannedPages int) {
	if minCharsDigital <= 0 {
		minCharsDigital = 40
	}
	for _, p := range doc.Pages {
		if p.CharCount >= minCharsDigital {
			digitalPages++
		} else {
			scannedPages++
		}
	}
	switch {
	case scannedPages == 0:
		return "digital", digitalPages, scannedPages
	case digitalPages == 0:
		return "scanned", digitalPages, scannedPages
	default:
		return "mixed", digitalPages, scannedPages
	}
}

// ToCDOMBlocks maps extractor blocks into CDOM blocks with geometry.
func ToCDOMBlocks(page PageText, startOrder int) ([]cdom.Block, int) {
	blocks := make([]cdom.Block, 0, len(page.Blocks))
	order := startOrder
	for i, b := range page.Blocks {
		order++
		typ := cdom.BlockParagraph
		if b.FontSize >= 16 {
			typ = cdom.BlockHeading
		}
		blocks = append(blocks, cdom.Block{
			ID:           fmt.Sprintf("p%d-b%d", page.PageNumber, i+1),
			Type:         typ,
			BBox:         cdom.BBox{b.X, page.Height - b.Y - b.H, b.X + b.W, page.Height - b.Y},
			ReadingOrder: order,
			Confidence:   b.Confidence,
			Language:     page.Language,
			Text:         b.Text,
		})
	}
	return blocks, order
}
