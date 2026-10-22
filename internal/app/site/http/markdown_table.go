package http

import (
	"fmt"
	"strings"

	goldmarkast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	renderhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// semanticTableCellRenderer treats the first cell in each Markdown table body
// row as its row header. Documentation tables consistently use that column to
// name the option, field, status, or resource described by the remaining cells.
type semanticTableCellRenderer struct{}

func (semanticTableCellRenderer) RegisterFuncs(registerer renderer.NodeRendererFuncRegisterer) {
	registerer.Register(extensionast.KindTableCell, renderSemanticTableCell)
	registerer.Register(extensionast.KindTable, renderScrollableTable)
}

func renderSemanticTableCell(writer util.BufWriter, _ []byte, node goldmarkast.Node, entering bool) (goldmarkast.WalkStatus, error) {
	cell := node.(*extensionast.TableCell)
	isColumnHeader := cell.Parent().Kind() == extensionast.KindTableHeader
	isRowHeader := cell.Parent().Kind() == extensionast.KindTableRow && cell.PreviousSibling() == nil
	tag := "td"
	if isColumnHeader || isRowHeader {
		tag = "th"
	}

	if !entering {
		_, _ = fmt.Fprintf(writer, "</%s>\n", tag)
		return goldmarkast.WalkContinue, nil
	}

	_, _ = fmt.Fprintf(writer, "<%s", tag)
	if isColumnHeader {
		_, _ = writer.WriteString(` scope="col"`)
	} else if isRowHeader {
		_, _ = writer.WriteString(` scope="row"`)
	}
	if cell.Alignment != extensionast.AlignNone {
		_, _ = fmt.Fprintf(writer, ` style="text-align:%s"`, cell.Alignment.String())
	}
	if cell.Attributes() != nil {
		if tag == "th" {
			renderhtml.RenderAttributes(writer, cell, extension.TableThCellAttributeFilter)
		} else {
			renderhtml.RenderAttributes(writer, cell, extension.TableTdCellAttributeFilter)
		}
	}
	_ = writer.WriteByte('>')
	return goldmarkast.WalkContinue, nil
}

// tableRegionLabelKey is the node attribute that carries a table's accessible
// name from the transformer to the renderer. Table attribute filters never
// serialize it.
const tableRegionLabelKey = "lv-region-label"

// tableRegionTransformer names each table's scroll region after the heading it
// sits under. Several tables under one heading are numbered, and any remaining
// duplicate gets a suffix, so no two regions on a page share an accessible name.
type tableRegionTransformer struct{}

func (tableRegionTransformer) Transform(document *goldmarkast.Document, reader text.Reader, _ parser.Context) {
	type placement struct {
		table   *extensionast.Table
		heading *goldmarkast.Heading
	}
	var placements []placement
	var heading *goldmarkast.Heading
	_ = goldmarkast.Walk(document, func(node goldmarkast.Node, entering bool) (goldmarkast.WalkStatus, error) {
		if !entering {
			return goldmarkast.WalkContinue, nil
		}
		switch node := node.(type) {
		case *goldmarkast.Heading:
			heading = node
		case *extensionast.Table:
			placements = append(placements, placement{table: node, heading: heading})
		}
		return goldmarkast.WalkContinue, nil
	})

	perHeading := map[*goldmarkast.Heading]int{}
	for _, placement := range placements {
		perHeading[placement.heading]++
	}
	seenInHeading := map[*goldmarkast.Heading]int{}
	used := map[string]int{}
	source := reader.Source()
	for _, placement := range placements {
		seenInHeading[placement.heading]++
		subject := "Table"
		if placement.heading != nil {
			if title := strings.TrimSpace(string(placement.heading.Text(source))); title != "" {
				subject = title + " table"
			}
		}
		label := subject
		if perHeading[placement.heading] > 1 {
			label = fmt.Sprintf("%s %d of %d", subject, seenInHeading[placement.heading], perHeading[placement.heading])
		}
		if used[label]++; used[label] > 1 {
			label = fmt.Sprintf("%s (%d)", label, used[label])
		}
		placement.table.SetAttributeString(tableRegionLabelKey, []byte(label))
	}
}

// scrollableTableRegion wraps a table that Go renders directly (rather than
// through Markdown) in the same focusable, named scroll region.
func scrollableTableRegion(label string, table g.Node) g.Node {
	return h.Div(h.Class("site-docs-table-scroll"), g.Attr("role", "region"), g.Attr("aria-label", label), g.Attr("tabindex", "0"), table)
}

// renderScrollableTable wraps every table in a keyboard-focusable, named
// region. A wide table scrolls horizontally, and a scroll container that cannot
// take focus is unreachable for people who do not use a pointer (WCAG 2.1.1).
func renderScrollableTable(writer util.BufWriter, _ []byte, node goldmarkast.Node, entering bool) (goldmarkast.WalkStatus, error) {
	if !entering {
		_, _ = writer.WriteString("</table>\n</div>\n")
		return goldmarkast.WalkContinue, nil
	}
	label := "Table"
	if value, ok := node.AttributeString(tableRegionLabelKey); ok {
		if bytes, ok := value.([]byte); ok {
			label = string(bytes)
		}
	}
	_, _ = writer.WriteString(`<div class="site-docs-table-scroll" role="region" aria-label="`)
	_, _ = writer.Write(util.EscapeHTML([]byte(label)))
	_, _ = writer.WriteString("\" tabindex=\"0\">\n<table")
	if node.Attributes() != nil {
		renderhtml.RenderAttributes(writer, node, extension.TableAttributeFilter)
	}
	_, _ = writer.WriteString(">\n")
	return goldmarkast.WalkContinue, nil
}
