package views

import (
	"fmt"
	"strings"
)

// CuratedModels is the single source of truth for the §6.1 model-curation tables.
// SPEC.md's §6.1 is GENERATED from it (RenderCuratedTables) between the marker
// comments below, so the two cannot drift; `make curated-sync` rewrites the region
// and `make test` fails if it is stale (TestCuratedTablesInSync).
const (
	CuratedMarkerBegin = "<!-- BEGIN generated: §6.1 tables from tui/views/curated.go CuratedModels; run `make curated-sync` -->"
	CuratedMarkerEnd   = "<!-- END generated: §6.1 tables -->"
)

// RenderCuratedTables renders the §6.1 general + coding markdown tables from
// CuratedModels. rows follow the order in CuratedModels; columns are left-aligned
// and padded to the widest cell, matching the repo's manual-table convention.
func RenderCuratedTables() string {
	var b strings.Builder
	b.WriteString("#### general\n\n")
	b.WriteString(renderRoleTable("general"))
	b.WriteString("\n#### coding\n\n")
	b.WriteString(renderRoleTable("coding"))
	return b.String()
}

// renderRoleTable builds the markdown table for one role from CuratedModels.
func renderRoleTable(role string) string {
	rows := [][]string{{"tier", "model", "size", "notes"}}
	for _, m := range CuratedModels {
		if m.Role != role {
			continue
		}
		rows = append(rows, []string{fmt.Sprintf("%dgb", m.TierGB), m.Model, m.Size, m.Notes})
	}
	return renderMarkdownTable(rows)
}

// renderMarkdownTable renders rows[0]=header, the rest=body: every column left-aligned
// (`:---`) and padded to its widest cell for raw readability.
func renderMarkdownTable(rows [][]string) string {
	cols := len(rows[0])
	width := make([]int, cols)
	for _, r := range rows {
		for c, cell := range r {
			if len(cell) > width[c] {
				width[c] = len(cell)
			}
		}
	}
	var b strings.Builder
	writeRow := func(cells []string) {
		b.WriteString("|")
		for c, cell := range cells {
			b.WriteString(" " + cell + strings.Repeat(" ", width[c]-len(cell)) + " |")
		}
		b.WriteString("\n")
	}
	writeRow(rows[0])
	// separator: left-align marker `:` + dashes, total run == column width.
	b.WriteString("|")
	for c := 0; c < cols; c++ {
		b.WriteString(" :" + strings.Repeat("-", width[c]-1) + " |")
	}
	b.WriteString("\n")
	for _, r := range rows[1:] {
		writeRow(r)
	}
	return b.String()
}
