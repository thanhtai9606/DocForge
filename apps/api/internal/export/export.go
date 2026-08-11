package export

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/thanhtai9606/DocForge/packages/cdom"
)

// Artifact is an exported payload.
type Artifact struct {
	Format      string
	ContentType string
	Body        []byte
}

// Exporter converts CDOM into a downloadable format.
type Exporter interface {
	Name() string
	Version() string
	Format() string
	Export(ctx context.Context, doc *cdom.Document) (Artifact, error)
}

// JSONExporter emits canonical CDOM JSON.
type JSONExporter struct{}

func NewJSON() *JSONExporter { return &JSONExporter{} }

func (e *JSONExporter) Name() string    { return "cdom-json" }
func (e *JSONExporter) Version() string { return "1.0.0" }
func (e *JSONExporter) Format() string  { return "json" }

func (e *JSONExporter) Export(_ context.Context, doc *cdom.Document) (Artifact, error) {
	if err := doc.Validate(); err != nil {
		return Artifact{}, err
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{Format: "json", ContentType: "application/json", Body: body}, nil
}

// MarkdownExporter maps CDOM blocks to Markdown.
type MarkdownExporter struct{}

func NewMarkdown() *MarkdownExporter { return &MarkdownExporter{} }

func (e *MarkdownExporter) Name() string    { return "cdom-markdown" }
func (e *MarkdownExporter) Version() string { return "1.0.0" }
func (e *MarkdownExporter) Format() string  { return "markdown" }

func (e *MarkdownExporter) Export(_ context.Context, doc *cdom.Document) (Artifact, error) {
	if err := doc.Validate(); err != nil {
		return Artifact{}, err
	}
	var b bytes.Buffer
	for pi, page := range doc.Pages {
		blocks := append([]cdom.Block(nil), page.Blocks...)
		sort.SliceStable(blocks, func(i, j int) bool {
			return blocks[i].ReadingOrder < blocks[j].ReadingOrder
		})
		for _, blk := range blocks {
			writeBlock(&b, blk, 0)
		}
		if pi < len(doc.Pages)-1 {
			b.WriteString("\n---\n\n")
		}
	}
	return Artifact{Format: "markdown", ContentType: "text/markdown; charset=utf-8", Body: b.Bytes()}, nil
}

func writeBlock(b *bytes.Buffer, blk cdom.Block, depth int) {
	switch blk.Type {
	case cdom.BlockHeading:
		level := 1
		if depth > 0 {
			level = depth + 1
		}
		if level > 6 {
			level = 6
		}
		b.WriteString(strings.Repeat("#", level))
		b.WriteByte(' ')
		b.WriteString(strings.TrimSpace(blk.Text))
		b.WriteString("\n\n")
	case cdom.BlockParagraph, cdom.BlockUnknown:
		if strings.TrimSpace(blk.Text) != "" {
			b.WriteString(strings.TrimSpace(blk.Text))
			b.WriteString("\n\n")
		}
	case cdom.BlockQuote:
		for _, line := range strings.Split(blk.Text, "\n") {
			b.WriteString("> ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	case cdom.BlockCode:
		b.WriteString("```\n")
		b.WriteString(blk.Text)
		if !strings.HasSuffix(blk.Text, "\n") {
			b.WriteByte('\n')
		}
		b.WriteString("```\n\n")
	case cdom.BlockList:
		for _, child := range blk.Children {
			writeBlock(b, child, depth+1)
		}
	case cdom.BlockListItem:
		b.WriteString(strings.Repeat("  ", depth))
		b.WriteString("- ")
		b.WriteString(strings.TrimSpace(blk.Text))
		b.WriteByte('\n')
	case cdom.BlockTable:
		writeTable(b, blk)
	case cdom.BlockImage:
		alt := ""
		asset := ""
		if blk.Attributes != nil {
			if v, ok := blk.Attributes["alt"].(string); ok {
				alt = v
			}
			if v, ok := blk.Attributes["asset_id"].(string); ok {
				asset = v
			}
		}
		fmt.Fprintf(b, "![%s](asset:%s)\n\n", alt, asset)
	case cdom.BlockPageBreak:
		b.WriteString("\n---\n\n")
	case cdom.BlockFormula:
		fmt.Fprintf(b, "$$\n%s\n$$\n\n", strings.TrimSpace(blk.Text))
	default:
		if strings.TrimSpace(blk.Text) != "" {
			b.WriteString(strings.TrimSpace(blk.Text))
			b.WriteString("\n\n")
		}
	}
	for _, child := range blk.Children {
		if blk.Type == cdom.BlockList || blk.Type == cdom.BlockTable {
			continue
		}
		writeBlock(b, child, depth+1)
	}
}

func writeTable(b *bytes.Buffer, table cdom.Block) {
	var rows [][]string
	for _, row := range table.Children {
		if row.Type != cdom.BlockTableRow {
			continue
		}
		var cells []string
		for _, cell := range row.Children {
			cells = append(cells, escapeMDCell(strings.TrimSpace(cell.Text)))
		}
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	}
	if len(rows) == 0 {
		return
	}
	width := len(rows[0])
	writeRow := func(cols []string) {
		b.WriteByte('|')
		for i := 0; i < width; i++ {
			v := ""
			if i < len(cols) {
				v = cols[i]
			}
			b.WriteByte(' ')
			b.WriteString(v)
			b.WriteString(" |")
		}
		b.WriteByte('\n')
	}
	writeRow(rows[0])
	b.WriteByte('|')
	for i := 0; i < width; i++ {
		b.WriteString(" --- |")
	}
	b.WriteByte('\n')
	for _, row := range rows[1:] {
		writeRow(row)
	}
	b.WriteByte('\n')
}

func escapeMDCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
