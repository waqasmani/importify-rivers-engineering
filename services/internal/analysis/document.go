package analysis

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/gif"  // register decoders for dimension detection
	_ "image/jpeg" // register decoders for dimension detection
	_ "image/png"  // register decoders for dimension detection
	"io"
	"path"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/htmlx"
)

func init() {
	register(documentAnalyzer{})
	register(screenshotAnalyzer{})
}

// ExtractText returns the plain text of a supported document. It never executes macros or scripts.
func ExtractText(filename string, data []byte, maxChars int) (string, error) {
	ext := strings.ToLower(path.Ext(filename))
	var text string
	var err error
	switch ext {
	case ".txt", ".md", ".markdown", ".csv", ".json", ".yaml", ".yml", ".log", ".rst":
		text = string(data)
	case ".html", ".htm":
		text = htmlx.Text(string(data), maxChars)
	case ".pdf":
		text, err = pdfText(data, maxChars)
	case ".docx":
		text, err = docxText(data, maxChars)
	default:
		if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
			return "", fmt.Errorf("unsupported binary document type %q; use PDF, DOCX, HTML, Markdown or text", ext)
		}
		text = string(data)
	}
	if err != nil {
		return "", err
	}
	text = strings.ToValidUTF8(text, "")
	if len(text) > maxChars {
		text = text[:maxChars]
	}
	return text, nil
}

func pdfText(data []byte, maxChars int) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the PDF could not be parsed")
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("the PDF could not be read: %w", err)
	}
	var b strings.Builder
	for i := 1; i <= r.NumPage() && b.Len() < maxChars; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		rows, err := p.GetTextByRow()
		if err != nil {
			continue
		}
		for _, row := range rows {
			for _, w := range row.Content {
				b.WriteString(w.S)
			}
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "\n[page %d]\n", i)
	}
	return b.String(), nil
}

func docxText(data []byte, maxChars int) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("the DOCX file is not a valid archive")
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		if f.UncompressedSize64 > 50<<20 {
			return "", fmt.Errorf("the DOCX document is too large")
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		dec := xml.NewDecoder(io.LimitReader(rc, 50<<20))
		var b strings.Builder
		for b.Len() < maxChars {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", fmt.Errorf("the DOCX document is malformed")
			}
			switch t := tok.(type) {
			case xml.CharData:
				b.Write(t)
			case xml.EndElement:
				if t.Name.Local == "p" {
					b.WriteByte('\n')
				}
			case xml.StartElement:
				if t.Name.Local == "tab" {
					b.WriteByte('\t')
				}
			}
		}
		return b.String(), nil
	}
	return "", fmt.Errorf("the DOCX file has no document body")
}

type documentAnalyzer struct{}

func (documentAnalyzer) Name() string { return "document" }

var (
	mdHeadingRe   = regexp.MustCompile(`(?m)^(#{1,4})\s+(.+)$`)
	numberedHdrRe = regexp.MustCompile(`(?m)^(\d+(?:\.\d+){0,3})\.?\s+([A-Z][^\n]{2,100})$`)
	requirementRe = regexp.MustCompile(`(?i)[^.\n]*\b(shall|must|should|is required to|needs to|has to)\b[^.\n]*[.\n]`)
	urlRe         = regexp.MustCompile(`https?://[^\s<>"')\]]+`)
	tableRowRe    = regexp.MustCompile(`(?m)^\s*\|.+\|\s*$`)
)

func (documentAnalyzer) Analyze(ctx context.Context, in Input) (Result, error) {
	b := newBuilder()
	text := in.Text
	if strings.TrimSpace(text) == "" {
		b.unknown("coverage", "No text could be extracted", "The document has no extractable text. Scanned documents need OCR, which REA does not perform locally.")
		return Result{Findings: b.out, Stats: map[string]any{"characters": 0}}, nil
	}
	words := len(strings.Fields(text))
	b.verified("document", "Document: "+clip(firstNonEmpty(in.Title, in.RawFilename, "untitled"), 200),
		fmt.Sprintf("%d words, %d characters of extracted text.", words, len(text)), clip(text, 1200), "beginning of document",
		map[string]any{"words": words})

	var outline []string
	for _, m := range mdHeadingRe.FindAllStringSubmatch(text, 300) {
		outline = append(outline, strings.Repeat("  ", len(m[1])-1)+strings.TrimSpace(m[2]))
	}
	if len(outline) == 0 {
		for _, m := range numberedHdrRe.FindAllStringSubmatch(text, 300) {
			outline = append(outline, strings.Repeat("  ", strings.Count(m[1], "."))+m[1]+" "+strings.TrimSpace(m[2]))
		}
	}
	if len(outline) > 0 {
		b.verified("structure", "Document outline ("+plural(len(outline), "section")+")", "", strings.Join(outline, "\n"), "headings", map[string]any{"outline": outline})
	}
	reqs := requirementRe.FindAllString(text, 400)
	seen := map[string]bool{}
	n := 0
	for _, rq := range reqs {
		rq = strings.Join(strings.Fields(rq), " ")
		if len(rq) < 15 || seen[strings.ToLower(rq)] {
			continue
		}
		seen[strings.ToLower(rq)] = true
		n++
		idx := strings.Index(text, strings.Fields(rq)[0])
		loc := "document"
		if idx >= 0 {
			loc = fmt.Sprintf("line %d", lineOf(text, idx))
		}
		b.inferred("requirement", clip(rq, 200), "A sentence phrased as an obligation.", rq, loc,
			"Detected from modal verbs (must, shall, should). Context such as conditions or exceptions may change its meaning.", 0.65, nil)
		if n >= 150 {
			break
		}
	}
	if urls := uniq(urlRe.FindAllString(text, 200)); len(urls) > 0 {
		b.verified("reference", "Referenced URLs ("+plural(len(urls), "URL")+")", "", strings.Join(urls, "\n"), "document", map[string]any{"urls": urls})
	}
	if rows := tableRowRe.FindAllString(text, -1); len(rows) > 0 {
		b.verified("structure", "Tables present ("+plural(len(rows), "row")+")", "", clip(strings.Join(rows[:min(20, len(rows))], "\n"), 3000), "table rows", nil)
	}
	return Result{Findings: b.out, Stats: map[string]any{"characters": len(text), "requirements": n}}, nil
}

type screenshotAnalyzer struct{}

func (screenshotAnalyzer) Name() string { return "screenshot" }

func (screenshotAnalyzer) Analyze(ctx context.Context, in Input) (Result, error) {
	b := newBuilder()
	cfg, format, err := image.DecodeConfig(bytes.NewReader(in.RawFile))
	data := map[string]any{"bytes": len(in.RawFile)}
	if err == nil {
		data["width"], data["height"], data["format"] = cfg.Width, cfg.Height, format
		b.verified("image", fmt.Sprintf("Image %dx%d (%s)", cfg.Width, cfg.Height, format), "Image properties read from the file header.",
			in.RawFilename, in.RawFilename, data)
	} else {
		b.verified("image", "Image file "+in.RawFilename, "The image dimensions could not be read; the format may be WebP or unsupported.", in.RawFilename, in.RawFilename, data)
	}
	b.unknown("visual_content", "Visual content needs annotation",
		"REA does not perform local OCR or image recognition. Add findings manually from what you see, or run the AI summary tool with a vision-capable provider.")
	return Result{Findings: b.out, Stats: data}, nil
}
