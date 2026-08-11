package postprocess

import (
	"strings"

	"github.com/thanhtai9606/DocForge/packages/cdom"
)

const (
	Name    = "cdom-postprocess"
	Version = "1.0.0"
)

// Clean applies lightweight CDOM cleanup before normalize/export.
// It does not invent structure (layout owns that); it only removes empties,
// trims text, and records processing metadata.
func Clean(doc *cdom.Document) {
	if doc == nil {
		return
	}
	for i := range doc.Pages {
		page := &doc.Pages[i]
		cleaned := make([]cdom.Block, 0, len(page.Blocks))
		for _, blk := range page.Blocks {
			if b, ok := cleanBlock(blk); ok {
				cleaned = append(cleaned, b)
			}
		}
		page.Blocks = cleaned
	}
	if doc.Processing == nil {
		doc.Processing = map[string]any{}
	}
	doc.Processing["postprocess_provider"] = Name
	doc.Processing["postprocess_provider_version"] = Version
}

func cleanBlock(blk cdom.Block) (cdom.Block, bool) {
	blk.Text = strings.TrimSpace(blk.Text)
	if len(blk.Children) > 0 {
		children := make([]cdom.Block, 0, len(blk.Children))
		for _, c := range blk.Children {
			if cc, ok := cleanBlock(c); ok {
				children = append(children, cc)
			}
		}
		blk.Children = children
	}
	switch blk.Type {
	case cdom.BlockParagraph, cdom.BlockUnknown, cdom.BlockHeading, cdom.BlockQuote, cdom.BlockCode, cdom.BlockFormula, cdom.BlockListItem:
		if blk.Text == "" && len(blk.Children) == 0 {
			return cdom.Block{}, false
		}
	case cdom.BlockList:
		if len(blk.Children) == 0 {
			return cdom.Block{}, false
		}
	case cdom.BlockTable:
		if len(blk.Children) == 0 {
			return cdom.Block{}, false
		}
	case cdom.BlockTableRow:
		if len(blk.Children) == 0 {
			return cdom.Block{}, false
		}
	case cdom.BlockImage, cdom.BlockPageBreak:
		return blk, true
	}
	return blk, true
}
