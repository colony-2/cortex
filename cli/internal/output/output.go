package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
)

// Printer renders CLI output in table or JSON formats.
type Printer struct {
	format string
	out    io.Writer
}

// New returns a Printer for the requested format.
func New(format string, out io.Writer) *Printer {
	if out == nil {
		out = os.Stdout
	}
	return &Printer{
		format: strings.ToLower(format),
		out:    out,
	}
}

func (p *Printer) isJSON() bool {
	return p.format == "json"
}

// JSON renders the value as JSON (pretty printed).
func (p *Printer) JSON(v any) error {
	enc := json.NewEncoder(p.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Table renders rows as a simple table. If format is JSON, emits JSON instead.
func (p *Printer) Table(headers []string, rows [][]string) error {
	if p.isJSON() {
		return p.JSON(rows)
	}
	t := table.NewWriter()
	t.SetOutputMirror(p.out)
	headerRow := make(table.Row, len(headers))
	for i, h := range headers {
		headerRow[i] = h
	}
	t.AppendHeader(headerRow)
	for _, r := range rows {
		row := make(table.Row, len(r))
		for i := range r {
			row[i] = r[i]
		}
		t.AppendRow(row)
	}
	t.SetStyle(table.StyleRounded)
	t.Render()
	return nil
}

// Text writes a single line (used for ids or confirmations).
func (p *Printer) Text(line string) error {
	_, err := fmt.Fprintln(p.out, line)
	return err
}
