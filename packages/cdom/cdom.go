// Package cdom defines the Canonical Document Object Model shared by providers and exporters.
package cdom

import (
	"encoding/json"
	"fmt"
)

const SchemaVersion = "1.0"

// BlockType enumerates supported CDOM block kinds.
type BlockType string

const (
	BlockHeading   BlockType = "heading"
	BlockParagraph BlockType = "paragraph"
	BlockList      BlockType = "list"
	BlockListItem  BlockType = "list_item"
	BlockTable     BlockType = "table"
	BlockTableRow  BlockType = "table_row"
	BlockTableCell BlockType = "table_cell"
	BlockImage     BlockType = "image"
	BlockCode      BlockType = "code"
	BlockFormula   BlockType = "formula"
	BlockQuote     BlockType = "quote"
	BlockPageBreak BlockType = "page_break"
	BlockUnknown   BlockType = "unknown"
)

// BBox is [x1, y1, x2, y2].
type BBox [4]float64

// Document is the CDOM root.
type Document struct {
	SchemaVersion string         `json:"schema_version"`
	DocumentID    string         `json:"document_id"`
	Source        Source         `json:"source"`
	Metadata      map[string]any `json:"metadata"`
	Processing    map[string]any `json:"processing"`
	Pages         []Page         `json:"pages"`
}

// Source describes the original uploaded file.
type Source struct {
	Filename  string `json:"filename"`
	MimeType  string `json:"mime_type"`
	PageCount int    `json:"page_count"`
}

// Page holds layout blocks for one page.
type Page struct {
	PageNumber int     `json:"page_number"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	Rotation   int     `json:"rotation"`
	Language   string  `json:"language,omitempty"`
	Blocks     []Block `json:"blocks"`
}

// Block is a semantic content unit with geometry and reading order.
type Block struct {
	ID           string         `json:"id"`
	Type         BlockType      `json:"type"`
	BBox         BBox           `json:"bbox"`
	ReadingOrder int            `json:"reading_order"`
	Confidence   float64        `json:"confidence"`
	Language     string         `json:"language,omitempty"`
	Text         string         `json:"text,omitempty"`
	Children     []Block        `json:"children,omitempty"`
	Attributes   map[string]any `json:"attributes,omitempty"`
}

var allowedBlockTypes = map[BlockType]struct{}{
	BlockHeading:   {},
	BlockParagraph: {},
	BlockList:      {},
	BlockListItem:  {},
	BlockTable:     {},
	BlockTableRow:  {},
	BlockTableCell: {},
	BlockImage:     {},
	BlockCode:      {},
	BlockFormula:   {},
	BlockQuote:     {},
	BlockPageBreak: {},
	BlockUnknown:   {},
}

// Validate checks required CDOM invariants.
func (d *Document) Validate() error {
	if d == nil {
		return fmt.Errorf("document is nil")
	}
	if d.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if d.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %q", d.SchemaVersion)
	}
	if d.DocumentID == "" {
		return fmt.Errorf("document_id is required")
	}
	if d.Source.Filename == "" {
		return fmt.Errorf("source.filename is required")
	}
	if d.Source.MimeType == "" {
		return fmt.Errorf("source.mime_type is required")
	}
	if d.Source.PageCount < 0 {
		return fmt.Errorf("source.page_count must be >= 0")
	}
	if d.Pages == nil {
		return fmt.Errorf("pages must be present")
	}
	for i := range d.Pages {
		if err := d.Pages[i].Validate(); err != nil {
			return fmt.Errorf("pages[%d]: %w", i, err)
		}
	}
	return nil
}

// Validate checks page-level invariants.
func (p *Page) Validate() error {
	if p.PageNumber < 1 {
		return fmt.Errorf("page_number must be >= 1")
	}
	if p.Width <= 0 || p.Height <= 0 {
		return fmt.Errorf("width and height must be > 0")
	}
	for i := range p.Blocks {
		if err := p.Blocks[i].Validate(); err != nil {
			return fmt.Errorf("blocks[%d]: %w", i, err)
		}
	}
	return nil
}

// Validate checks block-level invariants.
func (b *Block) Validate() error {
	if b.ID == "" {
		return fmt.Errorf("id is required")
	}
	if _, ok := allowedBlockTypes[b.Type]; !ok {
		return fmt.Errorf("unsupported block type %q", b.Type)
	}
	if b.BBox[0] > b.BBox[2] || b.BBox[1] > b.BBox[3] {
		return fmt.Errorf("invalid bbox")
	}
	if b.Confidence < 0 || b.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	if b.Type == BlockImage {
		if b.Attributes == nil {
			return fmt.Errorf("image block requires attributes.asset_id")
		}
		if _, ok := b.Attributes["asset_id"]; !ok {
			return fmt.Errorf("image block requires attributes.asset_id")
		}
	}
	for i := range b.Children {
		if err := b.Children[i].Validate(); err != nil {
			return fmt.Errorf("children[%d]: %w", i, err)
		}
	}
	return nil
}

// MarshalJSON encodes the document as JSON.
func (d *Document) MarshalJSON() ([]byte, error) {
	type alias Document
	return json.Marshal((*alias)(d))
}

// UnmarshalJSON decodes and validates a document.
func UnmarshalDocument(data []byte) (*Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return &doc, nil
}
