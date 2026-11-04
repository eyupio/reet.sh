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
// destination. Symbolic links are not taken from the archive; the two npm bin
// shims are created from constants afterwards.
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
	root := filepath.Clean(destination)
	if err := os.MkdirAll(root, 0700); err != nil {
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
		relative = filepath.FromSlash(relative)
		// IsLocal rejects absolute paths and any ".." element, so a crafted
		// entry cannot address a file outside the extraction root.
		if !filepath.IsLocal(relative) {
			return fmt.Errorf("%w: archive entry escapes its destination", ErrShape)
		}
		target := filepath.Join(root, relative)
		if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return fmt.Errorf("%w: archive entry escapes its destination", ErrShape)
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
		}
	}
	return nodeBinLinks(root)
}

// nodeBinLinks recreates the two npm bin shims that the reviewed archive
// carries as symbolic links. Their destinations are constants, never archive
// header data, and they are only written when the npm files are present.
func nodeBinLinks(root string) error {
	// The reviewed Node.js build has one top-level directory that extract drops.
	links := map[string]string{
		"npm": "../lib/node_modules/npm/bin/npm-cli.js",
		"npx": "../lib/node_modules/npm/bin/npx-cli.js",
	}
	for name, target := range links {
		link := filepath.Join(root, "bin", name)
		if _, err := os.Lstat(link); err == nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(link), target)); err != nil {
			continue
		}
		if err := os.Symlink(target, link); err != nil {
			return err
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
