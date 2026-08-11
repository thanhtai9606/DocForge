package layout

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/thanhtai9606/DocForge/packages/cdom"
)

var (
	reBullet  = regexp.MustCompile(`^[-*•◦●]\s+(.*)$`)
	reOrdered = regexp.MustCompile(`^(?:\(?\d+[).]|\d+\))\s+(.*)$`)
)

// reconstructListsAndTables groups consecutive list-like and pipe-delimited
// paragraph blocks into structured CDOM list/table trees. Reading order is
// reassigned by the caller after this runs.
func reconstructListsAndTables(page *cdom.Page) {
	if page == nil || len(page.Blocks) == 0 {
		return
	}
	out := make([]cdom.Block, 0, len(page.Blocks))
	i := 0
	for i < len(page.Blocks) {
		blk := page.Blocks[i]
		if isPlainTextBlock(blk) {
			if items, n, ordered := collectListRun(page.Blocks, i); n > 1 || (n == 1 && looksLikeListItem(blk.Text)) {
				out = append(out, buildListBlock(page.PageNumber, len(out)+1, items, ordered))
				i += n
				continue
			}
			if rows, n := collectPipeTableRun(page.Blocks, i); n >= 2 {
				out = append(out, buildTableBlock(page.PageNumber, len(out)+1, rows))
				i += n
				continue
			}
		}
		out = append(out, blk)
		i++
	}
	page.Blocks = out
}

func isPlainTextBlock(blk cdom.Block) bool {
	switch blk.Type {
	case cdom.BlockParagraph, cdom.BlockUnknown, cdom.BlockHeading:
		return len(blk.Children) == 0
	default:
		return false
	}
}

func looksLikeListItem(text string) bool {
	t := strings.TrimSpace(text)
	return reBullet.MatchString(t) || reOrdered.MatchString(t)
}

func collectListRun(blocks []cdom.Block, start int) (items []cdom.Block, n int, ordered bool) {
	first := strings.TrimSpace(blocks[start].Text)
	if reOrdered.MatchString(first) {
		ordered = true
	} else if !reBullet.MatchString(first) {
		return nil, 0, false
	}
	for i := start; i < len(blocks); i++ {
		blk := blocks[i]
		if !isPlainTextBlock(blk) {
			break
		}
		text := strings.TrimSpace(blk.Text)
		var body string
		if ordered {
			m := reOrdered.FindStringSubmatch(text)
			if m == nil {
				break
			}
			body = m[1]
		} else {
			m := reBullet.FindStringSubmatch(text)
			if m == nil {
				break
			}
			body = m[1]
		}
		item := blk
		item.Type = cdom.BlockListItem
		item.Text = strings.TrimSpace(body)
		item.Children = nil
		if item.Attributes == nil {
			item.Attributes = map[string]any{}
		}
		item.Attributes["ordered"] = ordered
		items = append(items, item)
		n++
	}
	return items, n, ordered
}

func collectPipeTableRun(blocks []cdom.Block, start int) (rows [][]string, n int) {
	cols := -1
	for i := start; i < len(blocks); i++ {
		blk := blocks[i]
		if !isPlainTextBlock(blk) {
			break
		}
		cells := splitPipeCells(blk.Text)
		if len(cells) < 2 {
			break
		}
		if cols < 0 {
			cols = len(cells)
		} else if len(cells) != cols {
			break
		}
		// Skip markdown separator rows like |---|---|
		if isMDSeparatorRow(cells) {
			n++
			continue
		}
		rows = append(rows, cells)
		n++
	}
	if len(rows) < 2 {
		return nil, 0
	}
	return rows, n
}

func splitPipeCells(text string) []string {
	t := strings.TrimSpace(text)
	if !strings.Contains(t, "|") {
		return nil
	}
	t = strings.Trim(t, "|")
	parts := strings.Split(t, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		cell := strings.TrimSpace(p)
		out = append(out, cell)
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

func isMDSeparatorRow(cells []string) bool {
	for _, c := range cells {
		s := strings.TrimSpace(c)
		if s == "" {
			return false
		}
		for _, r := range s {
			if r != '-' && r != ':' && r != ' ' {
				return false
			}
		}
	}
	return true
}

func buildListBlock(pageNumber, idx int, items []cdom.Block, ordered bool) cdom.Block {
	bbox := unionBBox(items)
	conf := minConfidence(items)
	attrs := map[string]any{"ordered": ordered}
	return cdom.Block{
		ID:         fmt.Sprintf("p%d-list%d", pageNumber, idx),
		Type:       cdom.BlockList,
		BBox:       bbox,
		Confidence: conf,
		Children:   items,
		Attributes: attrs,
	}
}

func buildTableBlock(pageNumber, idx int, rows [][]string) cdom.Block {
	children := make([]cdom.Block, 0, len(rows))
	for ri, row := range rows {
		cells := make([]cdom.Block, 0, len(row))
		for ci, text := range row {
			cells = append(cells, cdom.Block{
				ID:         fmt.Sprintf("p%d-t%d-r%d-c%d", pageNumber, idx, ri+1, ci+1),
				Type:       cdom.BlockTableCell,
				BBox:       cdom.BBox{},
				Confidence: 0.7,
				Text:       text,
			})
		}
		children = append(children, cdom.Block{
			ID:         fmt.Sprintf("p%d-t%d-r%d", pageNumber, idx, ri+1),
			Type:       cdom.BlockTableRow,
			BBox:       cdom.BBox{},
			Confidence: 0.7,
			Children:   cells,
		})
	}
	return cdom.Block{
		ID:         fmt.Sprintf("p%d-table%d", pageNumber, idx),
		Type:       cdom.BlockTable,
		BBox:       cdom.BBox{},
		Confidence: 0.7,
		Children:   children,
	}
}

func unionBBox(blocks []cdom.Block) cdom.BBox {
	if len(blocks) == 0 {
		return cdom.BBox{}
	}
	b := blocks[0].BBox
	for _, blk := range blocks[1:] {
		if blk.BBox[0] < b[0] {
			b[0] = blk.BBox[0]
		}
		if blk.BBox[1] < b[1] {
			b[1] = blk.BBox[1]
		}
		if blk.BBox[2] > b[2] {
			b[2] = blk.BBox[2]
		}
		if blk.BBox[3] > b[3] {
			b[3] = blk.BBox[3]
		}
	}
	return b
}

func minConfidence(blocks []cdom.Block) float64 {
	if len(blocks) == 0 {
		return 0
	}
	m := blocks[0].Confidence
	for _, b := range blocks[1:] {
		if b.Confidence < m {
			m = b.Confidence
		}
	}
	return m
}

func looksLikeHeading(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" || len(t) > 80 {
		return false
	}
	if looksLikeListItem(t) {
		return false
	}
	if strings.Contains(t, "|") {
		return false
	}
	for _, r := range t {
		if r == '.' || r == '?' || r == '!' {
			// Allow abbreviation-like single trailing period only when short title-ish.
			if r == '.' && strings.Count(t, ".") == 1 && strings.HasSuffix(t, ".") {
				continue
			}
			if r == '.' {
				return false
			}
			return false
		}
	}
	// Reject mostly lowercase long sentences without capitals.
	letters := 0
	upper := 0
	for _, r := range t {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	if letters == 0 {
		return false
	}
	return true
}
