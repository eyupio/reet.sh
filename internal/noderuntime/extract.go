package noderuntime

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxExtractedBytes = 2 << 30

// extract unpacks the reviewed Node.js archive under destination, dropping the
// archive's single top-level directory. Every entry must stay inside
// destination, and symbolic links may not point outside it.
func extract(archive, destination string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return ErrShape
	}
	defer compressed.Close()
	if err := os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	entries := tar.NewReader(compressed)
	var total int64
	for {
		header, err := entries.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ErrShape
		}
		relative, ok := stripRoot(header.Name)
		if !ok {
			continue
		}
		target, err := inside(destination, relative)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			total += header.Size
			if total > maxExtractedBytes {
				return errors.New("the Node.js archive was larger than allowed when extracted")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			mode := header.FileInfo().Mode().Perm() | 0600
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, err := io.CopyN(out, entries, header.Size); err != nil {
				out.Close()
				return ErrShape
			}
			if err := out.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(header.Linkname) {
				return fmt.Errorf("%w: absolute symbolic link", ErrShape)
			}
			if _, err := inside(destination, filepath.Join(filepath.Dir(relative), header.Linkname)); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func stripRoot(name string) (string, bool) {
	name = filepath.ToSlash(name)
	parts := strings.SplitN(name, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func inside(root, relative string) (string, error) {
	relative = filepath.FromSlash(relative)
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("%w: absolute archive entry", ErrShape)
	}
	target := filepath.Join(root, relative)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: archive entry escapes its destination", ErrShape)
	}
	return target, nil
}
