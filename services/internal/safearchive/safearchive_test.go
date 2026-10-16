package safearchive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(content))
	}
	_ = zw.Close()
	_ = f.Close()
	return p
}

func TestReadsNormalZip(t *testing.T) {
	p := writeZip(t, map[string]string{"src/main.go": "package main", "README.md": "# Hi"})
	files, err := Read(p, Limits{}, nil)
	if err != nil || len(files) != 2 {
		t.Fatalf("read failed: %v %d", err, len(files))
	}
}

func TestRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../evil.sh", "a/../../evil", "/etc/passwd", "C:/win.ini", "..\\evil"} {
		p := writeZip(t, map[string]string{name: "x"})
		if _, err := Read(p, Limits{}, nil); !errors.Is(err, ErrTraversal) {
			t.Errorf("%q: expected traversal error, got %v", name, err)
		}
	}
}

func TestRejectsZipBomb(t *testing.T) {
	big := strings.Repeat("A", 10<<20) // compresses ~1000:1
	p := writeZip(t, map[string]string{"bomb.txt": big})
	_, err := Read(p, Limits{MaxFiles: 10, MaxTotalBytes: 100 << 20, MaxFileBytes: 20 << 20, MaxRatio: 100}, nil)
	if !errors.Is(err, ErrBomb) {
		t.Fatalf("expected bomb error, got %v", err)
	}
}

func TestTotalSizeLimit(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 5; i++ {
		files[string(rune('a'+i))+".txt"] = strings.Repeat("x", 1000)
	}
	p := writeZip(t, files)
	if _, err := Read(p, Limits{MaxFiles: 100, MaxTotalBytes: 2500, MaxFileBytes: 2000, MaxRatio: 1000}, nil); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected size error, got %v", err)
	}
	if _, err := Read(p, Limits{MaxFiles: 3, MaxTotalBytes: 1 << 20, MaxFileBytes: 2000, MaxRatio: 1000}, nil); !errors.Is(err, ErrTooMany) {
		t.Fatalf("expected file count error, got %v", err)
	}
}

func TestTarGzSkipsLinks(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "app/index.js", Mode: 0o644, Size: 5, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("hello"))
	_ = tw.WriteHeader(&tar.Header{Name: "app/link", Linkname: "/etc/shadow", Typeflag: tar.TypeSymlink})
	_ = tw.Close()
	_ = gz.Close()
	p := filepath.Join(t.TempDir(), "a.tgz")
	_ = os.WriteFile(p, buf.Bytes(), 0o600)
	files, err := Read(p, Limits{}, nil)
	if err != nil || len(files) != 2 || files[1].Content != nil || files[1].Skipped == "" || string(files[0].Content) != "hello" {
		t.Fatalf("tar handling wrong: %v %+v", err, files)
	}
}

func TestUnsupported(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.rar")
	_ = os.WriteFile(p, []byte("Rar!"), 0o600)
	if _, err := Read(p, Limits{}, nil); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}
