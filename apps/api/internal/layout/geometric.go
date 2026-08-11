package layout

import (
	"context"
	"sort"

	"github.com/thanhtai9606/DocForge/packages/cdom"
)

// Provider analyzes page geometry and assigns final reading order.
type Provider interface {
	Name() string
	Version() string
	Analyze(ctx context.Context, doc *cdom.Document) error
}

// GeometricProvider sorts blocks top-to-bottom / left-to-right, reconstructs
// lists/tables, and assigns final reading_order.
type GeometricProvider struct{}

func NewGeometric() *GeometricProvider { return &GeometricProvider{} }

func (p *GeometricProvider) Name() string    { return "geometric-layout" }
func (p *GeometricProvider) Version() string { return "1.2.0" }

func (p *GeometricProvider) Analyze(_ context.Context, doc *cdom.Document) error {
	if doc == nil {
		return nil
	}
	order := 0
	for i := range doc.Pages {
		page := &doc.Pages[i]
		sort.SliceStable(page.Blocks, func(a, b int) bool {
			ba, bb := page.Blocks[a].BBox, page.Blocks[b].BBox
			if abs(ba[3]-bb[3]) > 2 {
				return ba[3] > bb[3]
			}
			return ba[0] < bb[0]
		})
		for j := range page.Blocks {
			if page.Blocks[j].Type == cdom.BlockParagraph && looksLikeHeading(page.Blocks[j].Text) {
				page.Blocks[j].Type = cdom.BlockHeading
			}
		}
		reconstructListsAndTables(page)
		for j := range page.Blocks {
			order++
			page.Blocks[j].ReadingOrder = order
			assignChildReadingOrder(&page.Blocks[j], &order)
		}
	}
	if doc.Processing == nil {
		doc.Processing = map[string]any{}
	}
	doc.Processing["layout_provider"] = p.Name()
	doc.Processing["layout_provider_version"] = p.Version()
	return nil
}

func assignChildReadingOrder(blk *cdom.Block, order *int) {
	for i := range blk.Children {
		*order++
		blk.Children[i].ReadingOrder = *order
		assignChildReadingOrder(&blk.Children[i], order)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

var _ Provider = (*GeometricProvider)(nil)
