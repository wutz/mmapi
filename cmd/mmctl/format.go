package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// line builds one output row. mm commands lay their listings out by column
// position rather than by fixed field widths: a value is padded to its column,
// and a value that overruns its column is followed by a single space, pushing
// the rest of the row right instead of colliding with it. That is why a long
// fileset name (pvc-<uuid>) still produces a readable mmlsfileset row.
type line struct {
	b []byte
}

func (l *line) padTo(col int) *line {
	for len(l.b) < col {
		l.b = append(l.b, ' ')
	}
	return l
}

// at writes s starting at column col, or one space after the current end when
// the row already reaches past it.
func (l *line) at(col int, s string) *line {
	if len(l.b) > 0 && col <= len(l.b) {
		col = len(l.b) + 1
	}
	l.padTo(col)
	l.b = append(l.b, s...)
	return l
}

// rightAt writes s so that it ends at column end, right-aligning it in the
// column that precedes it.
func (l *line) rightAt(end int, s string) *line {
	return l.at(end-len(s), s)
}

func (l *line) String() string { return string(l.b) }

// cell appends s left-aligned in a field of the given width. The quota reports
// are laid out in fixed-width fields rather than at fixed positions, and a
// value that fills its field is separated from the next by a single space.
func (l *line) cell(width int, s string) *line {
	start := len(l.b)
	l.b = append(l.b, s...)
	if len(s) >= width {
		l.b = append(l.b, ' ')
		return l
	}
	return l.padTo(start + width)
}

// rcell appends s right-aligned in a field of the given width.
func (l *line) rcell(width int, s string) *line {
	if len(s) >= width {
		l.b = append(l.b, ' ')
		l.b = append(l.b, s...)
		return l
	}
	l.padTo(len(l.b) + width - len(s))
	l.b = append(l.b, s...)
	return l
}

// -Y output. mm commands render machine-readable output as colon-delimited
// records: a HEADER record naming the fields, then data records carrying the
// values in the same order. Consumers look fields up by name in the header, so
// emitting the subset of fields the GUI exposes is safe; the names are the ones
// the real commands use.
type yRecord struct {
	cmd     string
	section string
	fields  []string
	// paths names the fields holding file system paths. mm commands escape
	// those more heavily than the rest — a dash inside a path is escaped too,
	// while a dash in a fileset name is not.
	paths map[string]bool
}

func newYRecord(cmd, section string, fields ...string) *yRecord {
	return &yRecord{cmd: cmd, section: section, fields: fields, paths: map[string]bool{}}
}

// withPathFields marks fields whose values are paths.
func (y *yRecord) withPathFields(fields ...string) *yRecord {
	for _, field := range fields {
		y.paths[field] = true
	}
	return y
}

func (y *yRecord) printHeader() {
	fmt.Printf("%s:%s:HEADER:version:reserved:reserved:%s:\n",
		y.cmd, y.section, strings.Join(y.fields, ":"))
}

func (y *yRecord) printRow(values ...string) {
	encoded := make([]string, len(values))
	for i, v := range values {
		if i < len(y.fields) && y.paths[y.fields[i]] {
			encoded[i] = yEncodePath(v)
			continue
		}
		encoded[i] = yEncode(v)
	}
	fmt.Printf("%s:%s:0:1:::%s:\n", y.cmd, y.section, strings.Join(encoded, ":"))
}

// yEncode percent-encodes the characters that would otherwise break the
// colon-delimited record: the separator itself and the escape character (a
// timestamp prints as Mon May 18 14%3A56%3A37 2026).
func yEncode(s string) string {
	return yEncodeChars(s, "%:")
}

// yEncodePath escapes a file system path, which mm commands escape more
// heavily — /fs0/fset-1 prints as %2Ffs0%2Ffset%2D1 — while leaving paths in
// other records, such as mmlscluster's remote shell command, untouched.
func yEncodePath(s string) string {
	return yEncodeChars(s, "%:/-")
}

func yEncodeChars(s, escape string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || strings.IndexByte(escape, c) >= 0 {
			fmt.Fprintf(&b, "%%%02X", c)
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// blockFormatter renders block values, which the GUI always reports in KiB, the
// way mm quota commands do: raw KiB by default, GPFS "auto" scaling with
// --block-size auto, or a fixed unit such as 1M.
type blockFormatter struct {
	auto    bool
	divisor float64
	header  string
}

func newBlockFormatter(spec string) (blockFormatter, error) {
	switch spec {
	case "":
		return blockFormatter{divisor: 1, header: "KB"}, nil
	case "auto":
		return blockFormatter{auto: true, header: "blocks"}, nil
	}

	kb, err := parseBlockSize(spec)
	if err != nil {
		return blockFormatter{}, err
	}
	return blockFormatter{divisor: kb, header: spec}, nil
}

// parseBlockSize converts a GPFS size such as 1M or 512K into KiB.
func parseBlockSize(spec string) (float64, error) {
	units := map[byte]float64{'K': 1, 'M': 1024, 'G': 1024 * 1024, 'T': 1024 * 1024 * 1024}
	s := strings.ToUpper(strings.TrimSpace(spec))
	mult := float64(1) / 1024 // a bare number is bytes
	if len(s) > 0 {
		if u, ok := units[s[len(s)-1]]; ok {
			mult = u
			s = s[:len(s)-1]
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n <= 0 {
		return 0, usagef("Invalid block size: %s", spec)
	}
	return n * mult, nil
}

func (b blockFormatter) format(kb int64) string {
	if !b.auto {
		if b.divisor == 1 {
			return strconv.FormatInt(kb, 10)
		}
		return strconv.FormatFloat(float64(kb)/b.divisor, 'f', 0, 64)
	}

	// "auto" picks the largest unit that keeps the value below 1024 and prints
	// four significant digits: 83873808 KB becomes 79.99G.
	value := float64(kb)
	if value == 0 {
		return "0"
	}
	units := []string{"K", "M", "G", "T", "P"}
	i := 0
	for (value >= 1024 || value <= -1024) && i < len(units)-1 {
		value /= 1024
		i++
	}
	return strconv.FormatFloat(value, 'g', 4, 64) + units[i]
}

// gpfsTime converts a GUI timestamp ("2026-05-18 14:56:39,000") into the ctime
// layout mm commands print ("Mon May 18 14:56:39 2026"). Unparseable values are
// passed through so a GUI change degrades to raw output rather than an error.
func gpfsTime(s string) string {
	if s == "" {
		return ""
	}
	clean := s
	if idx := strings.IndexByte(clean, ','); idx >= 0 {
		clean = clean[:idx]
	}
	t, err := time.Parse("2006-01-02 15:04:05", clean)
	if err != nil {
		return s
	}
	return t.Format("Mon Jan _2 15:04:05 2006")
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
