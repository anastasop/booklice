package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	maxOutputSizeText  = 100 * 1024 * 1024 // 100MB
	maxOutputSizeCover = 10 * 1024 * 1024  // 10MB
)

var (
	gsExe = "gs"

	//go:embed emptypage.pdf
	emptyPage []byte
)

// PDF is a handle for a pdf file
type PDF struct {
	path string
	data []byte
}

func newPDF(p string) (PDF, error) {
	var pdf PDF
	data, err := os.ReadFile(p)
	if err != nil {
		return pdf, err
	}
	pdf.path = p
	pdf.data = data
	return pdf, nil
}

func (p PDF) Path() string {
	return p.path
}

func (p PDF) Data() io.Reader {
	return bytes.NewBuffer(p.data)
}

// FullText uses ghostscript to extract the full text of the pdf
func (p PDF) FullText(ctx context.Context) ([]byte, error) {
	args := []string{
		"-dNOPAUSE",
		"-dBATCH",
		"-dSAFER",
		"-dQUIET",
		"-sDEVICE=txtwrite",
		"-sOutputFile=-",
		"-",
	}
	cmd := exec.CommandContext(ctx, gsExe, args...)
	cmd.Stdin = p.Data()
	b := newBoundedBuffer(maxOutputSizeText)
	cmd.Stdout = b
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to get full text of %q: %w", p.Path(), err)
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("failed to get full text of %q: %w", p.Path(), err)
	}

	if !b.filled {
		return b.buf.Bytes(), nil
	}
	return nil, nil
}

// FullText uses ghostscript to extract the cover of the pdf
func (p PDF) Cover(ctx context.Context) ([]byte, error) {
	args := []string{
		"-dNOPAUSE",
		"-dBATCH",
		"-dSAFER",
		"-dQUIET",
		"-sDEVICE=pdfwrite",
		"-sOutputFile=-",
		"-dFirstPage=1",
		"-dLastPage=1",
		"-",
	}
	cmd := exec.CommandContext(ctx, gsExe, args...)
	cmd.Stdin = p.Data()
	b := newBoundedBuffer(maxOutputSizeCover)
	cmd.Stdout = b
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to get cover of %q: %w", p.Path(), err)
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("failed to get cover of %q: %w", p.Path(), err)
	}

	if !b.filled {
		return b.buf.Bytes(), nil
	}
	return emptyPage, nil
}

// Pages uses ghostscript to count the pages of the pdf
func (p PDF) Pages(ctx context.Context) (int, error) {
	args := []string{
		"-dNOPAUSE",
		"-dBATCH",
		"-dSAFER",
		"-dQUIET",
		"-dNODISPLAY",
		fmt.Sprintf(`--permit-file-read=%s`, p.Path()),
		"-c",
		fmt.Sprintf(`(%s) (r) file runpdfbegin pdfpagecount = quit`, p.Path()),
	}
	cmd := exec.CommandContext(ctx, gsExe, args...)
	cmd.Stdin = p.Data()
	data, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get pages of %q: %w", p.Path(), err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("failed to get pages of %q: %w", p.Path(), err)
	}
	return n, nil
}

// Sig returns a SHA256 hash of the pdf, useful to find duplicates in the index
func (p PDF) Sig() (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, p.Data()); err != nil {
		return "", fmt.Errorf("failed to build signature of %q: %w", p.Path(), err)
	}
	return fmt.Sprintf("%0x", h.Sum(nil)), nil
}

type boundedBuffer struct {
	buf    bytes.Buffer
	limit  int
	filled bool
}

func newBoundedBuffer(n int) *boundedBuffer {
	return &boundedBuffer{limit: n}
}

func (b *boundedBuffer) Write(p []byte) (n int, err error) {
	if remain := b.limit - b.buf.Len(); len(p) <= remain {
		return b.buf.Write(p)
	}
	// don't care if we are near the limit
	b.filled = true
	return len(p), nil
}
