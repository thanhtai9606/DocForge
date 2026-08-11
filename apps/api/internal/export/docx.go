package export

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gomutex/godocx"
	"github.com/gomutex/godocx/docx"

	"github.com/thanhtai9606/DocForge/packages/cdom"
)

// DOCXExporter maps CDOM blocks into an Office Open XML document.
type DOCXExporter struct{}

func NewDOCX() *DOCXExporter { return &DOCXExporter{} }

func (e *DOCXExporter) Name() string    { return "cdom-docx" }
func (e *DOCXExporter) Version() string { return "1.0.0" }
func (e *DOCXExporter) Format() string  { return "docx" }

func (e *DOCXExporter) Export(_ context.Context, doc *cdom.Document) (Artifact, error) {
	if err := doc.Validate(); err != nil {
		return Artifact{}, err
	}
	root, err := godocx.NewDocument()
	if err != nil {
		return Artifact{}, fmt.Errorf("create docx: %w", err)
	}

	for pi, page := range doc.Pages {
		blocks := append([]cdom.Block(nil), page.Blocks...)
		sort.SliceStable(blocks, func(i, j int) bool {
			return blocks[i].ReadingOrder < blocks[j].ReadingOrder
		})
		for _, blk := range blocks {
			if err := writeDOCXBlock(root, blk, 0, false); err != nil {
				return Artifact{}, err
			}
		}
		if pi < len(doc.Pages)-1 {
			root.AddPageBreak()
		}
	}

	var buf bytes.Buffer
	if err := root.Write(&buf); err != nil {
		return Artifact{}, fmt.Errorf("serialize docx: %w", err)
	}
	return Artifact{
		Format:      "docx",
		ContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		Body:        buf.Bytes(),
	}, nil
}

func writeDOCXBlock(root *docx.RootDoc, blk cdom.Block, depth int, orderedList bool) error {
	switch blk.Type {
	case cdom.BlockHeading:
		level := uint(1)
		if depth > 0 {
			level = uint(depth + 1)
		}
		if level > 6 {
			level = 6
		}
		if _, err := root.AddHeading(strings.TrimSpace(blk.Text), level); err != nil {
			return err
		}
	case cdom.BlockParagraph, cdom.BlockUnknown:
		if strings.TrimSpace(blk.Text) != "" {
			root.AddParagraph(strings.TrimSpace(blk.Text))
		}
	case cdom.BlockQuote:
		p := root.AddParagraph(strings.TrimSpace(blk.Text))
		p.Style("Intense Quote")
	case cdom.BlockCode:
		root.AddParagraph(blk.Text).Style("No Spacing")
	case cdom.BlockList:
		ordered := orderedList
		if blk.Attributes != nil {
			if v, ok := blk.Attributes["ordered"].(bool); ok {
				ordered = v
			}
		}
		for _, child := range blk.Children {
			if err := writeDOCXBlock(root, child, depth+1, ordered); err != nil {
				return err
			}
		}
		return nil
	case cdom.BlockListItem:
		ordered := orderedList
		if blk.Attributes != nil {
			if v, ok := blk.Attributes["ordered"].(bool); ok {
				ordered = v
			}
		}
		p := root.AddParagraph(strings.TrimSpace(blk.Text))
		if ordered {
			p.Style("List Number")
		} else {
			p.Style("List Bullet")
		}
		p.Numbering(1, depth)
	case cdom.BlockTable:
		writeDOCXTable(root, blk)
	case cdom.BlockImage:
		alt := "image"
		asset := ""
		if blk.Attributes != nil {
			if v, ok := blk.Attributes["alt"].(string); ok && v != "" {
				alt = v
			}
			if v, ok := blk.Attributes["asset_id"].(string); ok {
				asset = v
			}
		}
		root.AddParagraph(fmt.Sprintf("[%s: asset:%s]", alt, asset))
	case cdom.BlockPageBreak:
		root.AddPageBreak()
	case cdom.BlockFormula:
		root.AddParagraph(strings.TrimSpace(blk.Text))
	default:
		if strings.TrimSpace(blk.Text) != "" {
			root.AddParagraph(strings.TrimSpace(blk.Text))
		}
	}

	for _, child := range blk.Children {
		if blk.Type == cdom.BlockList || blk.Type == cdom.BlockTable {
			continue
		}
		if err := writeDOCXBlock(root, child, depth+1, orderedList); err != nil {
			return err
		}
	}
	return nil
}

func writeDOCXTable(root *docx.RootDoc, tableBlk cdom.Block) {
	table := root.AddTable()
	table.Style("TableGrid")
	for _, rowBlk := range tableBlk.Children {
		if rowBlk.Type != cdom.BlockTableRow {
			continue
		}
		row := table.AddRow()
		for _, cellBlk := range rowBlk.Children {
			text := strings.TrimSpace(cellBlk.Text)
			if text == "" && len(cellBlk.Children) > 0 {
				var parts []string
				for _, c := range cellBlk.Children {
					if t := strings.TrimSpace(c.Text); t != "" {
						parts = append(parts, t)
					}
				}
				text = strings.Join(parts, " ")
			}
			row.AddCell().AddParagraph(text)
		}
	}
}
