// The check and products subcommands: one-off lookups against the
// snapshot, no repository required.
package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/JaydenCJ/eolvet/internal/eoldata"
)

// runCheck answers "is <product> <version> still supported?" with the
// same date math the scanner uses. Exit 1 when the answer is EOL (or
// eol-soon under --fail-on eol-soon), so it drops straight into CI.
func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("check", stderr)
	asOfFlag := fs.String("as-of", "", "")
	warnDays := fs.Int("warn-within", 90, "")
	failOn := fs.String("fail-on", "eol", "")
	dataPath := fs.String("data", "", "")
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(positional) != 2 {
		fmt.Fprintln(stderr, "eolvet: usage: eolvet check <product> <version>")
		return exitUsage
	}
	name, ver := positional[0], positional[1]
	asOf, err := parseAsOf(*asOfFlag)
	if err != nil {
		fmt.Fprintf(stderr, "eolvet: %v\n", err)
		return exitUsage
	}
	snap, err := loadSnapshot(*dataPath)
	if err != nil {
		fmt.Fprintf(stderr, "eolvet: %v\n", err)
		return exitRuntime
	}
	product, ok := snap.Product(name)
	if !ok {
		fmt.Fprintf(stderr, "eolvet: unknown product %q — run `eolvet products` for the list\n", name)
		return exitUsage
	}
	cycle, ok := product.Resolve(ver)
	if !ok {
		fmt.Fprintf(stderr, "eolvet: no release cycle matching %s %s in snapshot %s\n",
			product.Label, ver, snap.SnapshotDate)
		return exitUsage
	}
	eol, err := eoldata.ParseDate(cycle.EOL)
	if err != nil {
		fmt.Fprintf(stderr, "eolvet: %v\n", err)
		return exitRuntime
	}
	days := eoldata.DaysUntil(asOf, eol)
	suffix := fmt.Sprintf("(as of %s; snapshot %s)", asOf.Format("2006-01-02"), snap.SnapshotDate)
	switch {
	case days <= 0:
		fmt.Fprintf(stdout, "%s %s — EOL since %s, %s ago %s\n",
			product.Label, cycle.Cycle, cycle.EOL, nDays(-days), suffix)
		return exitBreach
	case days <= *warnDays:
		fmt.Fprintf(stdout, "%s %s — EOL SOON on %s, in %s %s\n",
			product.Label, cycle.Cycle, cycle.EOL, nDays(days), suffix)
		if *failOn == "eol-soon" {
			return exitBreach
		}
	default:
		fmt.Fprintf(stdout, "%s %s — supported until %s, %s left %s\n",
			product.Label, cycle.Cycle, cycle.EOL, nDays(days), suffix)
	}
	return exitOK
}

// nDays formats a day count with the right plural ("1 day", "49 days").
func nDays(n int) string { return counted(n, "day") }

// counted joins a count and a noun, pluralizing with a plain "s".
func counted(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// runProducts lists everything the bundled (or --data) snapshot covers,
// so users can see exactly what an offline scan can and cannot judge.
func runProducts(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("products", stderr)
	format := fs.String("format", "text", "")
	dataPath := fs.String("data", "", "")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "eolvet: products takes no arguments")
		return exitUsage
	}
	snap, err := loadSnapshot(*dataPath)
	if err != nil {
		fmt.Fprintf(stderr, "eolvet: %v\n", err)
		return exitRuntime
	}
	switch *format {
	case "text":
		fmt.Fprint(stdout, productsText(snap))
	case "json":
		out, err := productsJSON(snap)
		if err != nil {
			fmt.Fprintf(stderr, "eolvet: %v\n", err)
			return exitRuntime
		}
		fmt.Fprint(stdout, out)
	default:
		fmt.Fprintf(stderr, "eolvet: unknown --format %q (want text or json)\n", *format)
		return exitUsage
	}
	return exitOK
}

func productsText(snap *eoldata.Snapshot) string {
	rows := [][]string{{"PRODUCT", "LABEL", "CYCLES", "OLDEST", "NEWEST", "CODENAMES"}}
	for _, name := range snap.ProductNames() {
		p, _ := snap.Product(name)
		codenames := "—"
		if n := len(p.Codenames); n > 0 {
			codenames = fmt.Sprintf("%d", n)
		}
		rows = append(rows, []string{
			name, p.Label, fmt.Sprintf("%d", len(p.Cycles)),
			p.Cycles[0].Cycle, p.Cycles[len(p.Cycles)-1].Cycle, codenames,
		})
	}
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if w := len([]rune(cell)); w > widths[i] {
				widths[i] = w
			}
		}
	}
	out := fmt.Sprintf("eolvet products — snapshot %s (schema %d)\n\n", snap.SnapshotDate, snap.SchemaVersion)
	for _, row := range rows {
		for i, cell := range row {
			if i == len(row)-1 {
				out += cell
				continue
			}
			out += fmt.Sprintf("%-*s  ", widths[i], cell)
		}
		out += "\n"
	}
	out += fmt.Sprintf("\n%s, %s\n",
		counted(len(snap.Products), "product"), counted(snap.CycleCount(), "cycle"))
	return out
}

func productsJSON(snap *eoldata.Snapshot) (string, error) {
	type entry struct {
		Product string `json:"product"`
		Label   string `json:"label"`
		Cycles  int    `json:"cycles"`
		Oldest  string `json:"oldest"`
		Newest  string `json:"newest"`
	}
	doc := struct {
		Tool          string  `json:"tool"`
		SchemaVersion int     `json:"schema_version"`
		SnapshotDate  string  `json:"snapshot_date"`
		Source        string  `json:"source"`
		Products      []entry `json:"products"`
	}{Tool: "eolvet", SchemaVersion: 1, SnapshotDate: snap.SnapshotDate, Source: snap.Source}
	for _, name := range snap.ProductNames() {
		p, _ := snap.Product(name)
		doc.Products = append(doc.Products, entry{
			Product: name, Label: p.Label, Cycles: len(p.Cycles),
			Oldest: p.Cycles[0].Cycle, Newest: p.Cycles[len(p.Cycles)-1].Cycle,
		})
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out) + "\n", nil
}
