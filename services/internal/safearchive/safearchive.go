// Package safearchive reads ZIP and tar.gz archives for analysis without extracting them to disk.
// It rejects path traversal, absolute paths and links, and stops on decompression bombs by
// counting the bytes actually produced rather than trusting archive headers.
package safearchive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// Limits bound the work done on one archive.
type Limits struct {
	MaxFiles      int   // maximum number of regular files
	MaxTotalBytes int64 // maximum uncompressed bytes read across all files
	MaxFileBytes  int64 // files larger than this are listed but their content is not read
	MaxRatio      int64 // maximum uncompressed/compressed ratio for a zip entry larger than 1 MiB
}

// DefaultLimits are used when a zero Limits is passed.
var DefaultLimits = Limits{MaxFiles: 20000, MaxTotalBytes: 300 << 20, MaxFileBytes: 2 << 20, MaxRatio: 100}

// File is one archive member.
type File struct {
	Path    string
	Size    int64
	Content []byte // nil when the file was skipped (too large or binary filter)
	Skipped string // reason the content was not read
}

// Errors returned for unsafe archives.
var (
	ErrTraversal   = errors.New("archive contains an unsafe path")
	ErrTooMany     = errors.New("archive contains too many files")
	ErrTooLarge    = errors.New("archive expands beyond the size limit")
	ErrBomb        = errors.New("archive entry has a suspicious compression ratio")
	ErrUnsupported = errors.New("unsupported archive format; use .zip or .tar.gz")
)

// CleanPath validates and normalises an archive member name.
func CleanPath(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, 0) {
		return "", ErrTraversal
	}
	n := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(n, "/") || (len(n) > 1 && n[1] == ':') {
		return "", ErrTraversal
	}
	for _, part := range strings.Split(n, "/") {
		if part == ".." {
			return "", ErrTraversal
		}
	}
	clean := path.Clean(n)
	if clean == "." || strings.HasPrefix(clean, "../") {
		return "", ErrTraversal
	}
	return clean, nil
}

// Want decides whether a file's content should be read.
type Want func(p string, size int64) bool

// Read opens the archive at filePath and returns its regular files.
func Read(filePath string, lim Limits, want Want) ([]File, error) {
	if lim.MaxFiles == 0 {
		lim = DefaultLimits
	}
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	head := make([]byte, 4)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	switch {
	case bytes.HasPrefix(head, []byte("PK\x03\x04")) || bytes.HasPrefix(head, []byte("PK\x05\x06")):
		st, err := f.Stat()
		if err != nil {
			return nil, err
		}
		return readZip(f, st.Size(), lim, want)
	case bytes.HasPrefix(head, []byte{0x1f, 0x8b}):
		return readTarGz(f, lim, want)
	}
	return nil, ErrUnsupported
}

func readZip(r io.ReaderAt, size int64, lim Limits, want Want) ([]File, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("invalid zip archive: %w", err)
	}
	var out []File
	var total int64
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		p, err := CleanPath(zf.Name)
		if err != nil {
			return nil, ErrTraversal
		}
		mode := zf.Mode()
		if mode&os.ModeSymlink != 0 || !mode.IsRegular() {
			out = append(out, File{Path: p, Skipped: "link or special file"})
			continue
		}
		if len(out) >= lim.MaxFiles {
			return nil, ErrTooMany
		}
		file := File{Path: p, Size: int64(zf.UncompressedSize64)}
		if zf.UncompressedSize64 > 1<<20 && zf.CompressedSize64 > 0 && int64(zf.UncompressedSize64/zf.CompressedSize64) > lim.MaxRatio {
			return nil, ErrBomb
		}
		if int64(zf.UncompressedSize64) > lim.MaxFileBytes {
			file.Skipped = "larger than the per-file analysis limit"
		} else if want != nil && !want(p, int64(zf.UncompressedSize64)) {
			file.Skipped = "not needed for analysis"
		} else {
			rc, err := zf.Open()
			if err != nil {
				return nil, err
			}
			// Read at most the per-file limit plus one byte regardless of the header's claims.
			data, err := io.ReadAll(io.LimitReader(rc, lim.MaxFileBytes+1))
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", p, err)
			}
			if int64(len(data)) > lim.MaxFileBytes {
				return nil, ErrBomb
			}
			total += int64(len(data))
			if total > lim.MaxTotalBytes {
				return nil, ErrTooLarge
			}
			file.Content = data
			file.Size = int64(len(data))
		}
		out = append(out, file)
	}
	return out, nil
}

func readTarGz(r io.Reader, lim Limits, want Want) ([]File, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("invalid gzip stream: %w", err)
	}
	defer gz.Close()
	// Bound the total decompressed stream, including skipped content.
	limited := &countingReader{r: gz, max: lim.MaxTotalBytes * 2}
	tr := tar.NewReader(limited)
	var out []File
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if errors.Is(err, ErrTooLarge) {
				return nil, ErrTooLarge
			}
			return nil, fmt.Errorf("invalid tar archive: %w", err)
		}
		if h.Typeflag == tar.TypeDir || h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		p, err := CleanPath(h.Name)
		if err != nil {
			return nil, ErrTraversal
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			out = append(out, File{Path: p, Skipped: "link or special file"})
			continue
		}
		if len(out) >= lim.MaxFiles {
			return nil, ErrTooMany
		}
		file := File{Path: p, Size: h.Size}
		if h.Size > lim.MaxFileBytes {
			file.Skipped = "larger than the per-file analysis limit"
		} else if want != nil && !want(p, h.Size) {
			file.Skipped = "not needed for analysis"
		} else {
			data, err := io.ReadAll(io.LimitReader(tr, lim.MaxFileBytes+1))
			if err != nil {
				if errors.Is(err, ErrTooLarge) {
					return nil, ErrTooLarge
				}
				return nil, err
			}
			total += int64(len(data))
			if total > lim.MaxTotalBytes {
				return nil, ErrTooLarge
			}
			file.Content = data
		}
		out = append(out, file)
	}
	return out, nil
}

type countingReader struct {
	r   io.Reader
	n   int64
	max int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.n > c.max {
		return n, ErrTooLarge
	}
	return n, err
}
