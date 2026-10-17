// Package filestore keeps uploaded files on the local filesystem, addressed by SHA-256.
package filestore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// Store writes files below a root directory.
type Store struct{ root string }

// New creates the root directory if needed.
func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create attachment directory: %w", err)
	}
	return &Store{root: root}, nil
}

// Saved describes a stored file.
type Saved struct {
	Key    string
	SHA256 string
	Size   int64
}

var keyRe = regexp.MustCompile(`^[0-9a-f]{2}/[0-9a-f]{64}$`)

// ErrTooLarge is returned when the input exceeds the size limit.
var ErrTooLarge = errors.New("file exceeds the upload size limit")

// Save streams r to disk, enforcing max bytes, and returns its content address.
func (s *Store) Save(r io.Reader, max int64) (Saved, error) {
	tmp, err := os.CreateTemp(s.root, ".upload-*")
	if err != nil {
		return Saved{}, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, max+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Saved{}, err
	}
	if n > max {
		return Saved{}, ErrTooLarge
	}
	sum := hex.EncodeToString(h.Sum(nil))
	key := sum[:2] + "/" + sum
	dst := filepath.Join(s.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return Saved{}, err
	}
	if _, err := os.Stat(dst); err == nil {
		return Saved{Key: key, SHA256: sum, Size: n}, nil
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return Saved{}, err
	}
	_ = os.Chmod(dst, 0o640)
	return Saved{Key: key, SHA256: sum, Size: n}, nil
}

// Open opens a stored file by key.
func (s *Store) Open(key string) (*os.File, error) {
	if !keyRe.MatchString(key) {
		return nil, errors.New("invalid storage key")
	}
	return os.Open(filepath.Join(s.root, filepath.FromSlash(key)))
}

// Delete removes a stored file by key. Missing files are ignored.
func (s *Store) Delete(key string) error {
	if !keyRe.MatchString(key) {
		return errors.New("invalid storage key")
	}
	err := os.Remove(filepath.Join(s.root, filepath.FromSlash(key)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Keys lists every stored key (used by garbage collection).
func (s *Store) Keys() ([]string, error) {
	var keys []string
	err := filepath.WalkDir(s.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if keyRe.MatchString(rel) {
			keys = append(keys, rel)
		}
		return nil
	})
	return keys, err
}

// Path returns the on-disk path of a stored file for read-only processing.
func (s *Store) Path(key string) (string, error) {
	if !keyRe.MatchString(key) {
		return "", errors.New("invalid storage key")
	}
	return filepath.Join(s.root, filepath.FromSlash(key)), nil
}
