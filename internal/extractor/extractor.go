package extractor

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ledongthuc/pdf"
)

// FromText reads and returns plain text from r.
func FromText(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read text: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// FromPDF extracts all text from a PDF provided as raw bytes.
//
// It writes the bytes to a temp file so pdf.Open can use its path-based
// reader (which handles seeking correctly for large PDFs), then cleans up.
// Each page error is returned immediately — use FromPDFLenient if you want
// to skip unreadable pages instead.
func FromPDF(data []byte) (string, error) {
	// Write to a temp file so pdf.Open can seek through it properly.
	tmp, err := os.CreateTemp("", "rag-pdf-*.pdf")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("write temp file: %w", err)
	}
	tmp.Close()

	return extractFromPath(tmp.Name(), false)
}

// FromPDFLenient is like FromPDF but skips pages that fail to extract
// instead of returning an error. Useful for PDFs with mixed content
// (e.g. some image pages, some text pages).
func FromPDFLenient(data []byte) (string, error) {
	tmp, err := os.CreateTemp("", "rag-pdf-*.pdf")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("write temp file: %w", err)
	}
	tmp.Close()

	return extractFromPath(tmp.Name(), true)
}

// extractFromPath opens a PDF by file path and extracts all text.
// If lenient is true, pages that fail are skipped; otherwise the error
// is returned immediately.
func extractFromPath(path string, lenient bool) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("open PDF: %w", err)
	}
	defer f.Close()

	var sb strings.Builder

	for pageNum := 1; pageNum <= r.NumPage(); pageNum++ {
		page := r.Page(pageNum)

		if page.V.IsNull() {
			continue
		}

		pageText, err := page.GetPlainText(nil)
		if err != nil {
			if lenient {
				continue
			}
			return "", fmt.Errorf("extract text from page %d: %w", pageNum, err)
		}

		sb.WriteString(pageText)
		sb.WriteString("\n")
	}

	result := strings.TrimSpace(sb.String())
	if result == "" {
		return "", fmt.Errorf("PDF contains no extractable text (may be scanned/image-only)")
	}

	return result, nil
}
