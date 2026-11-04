//go:build linux || darwin

package terminalagent

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const previewCacheBytes = 64 << 20
const previewCacheAge = 24 * time.Hour

// previewCacheFiles bounds the cache by count as well as by bytes. Pruning
// leaves one free slot for the snapshot being written, so a full cache holds at
// most this many files. Tests lower it instead of creating a thousand files.
var previewCacheFiles = 1024

type chatPreviewStore struct {
	mu        sync.Mutex
	dir, home string
}

func newChatPreviewStore(dir, home string) *chatPreviewStore {
	if dir == "" {
		return nil
	}
	return &chatPreviewStore{dir: filepath.Join(dir, "chat-previews"), home: home}
}

func previewName(name, path string) string {
	if name == "" {
		name = filepath.Base(path)
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." {
		name = "Agent image"
	}
	return cutText(name, 200)
}

func (s *chatPreviewStore) capture(session, dir string, p *acpagent.Preview) terminalproto.ACPPreviewRef {
	ref := terminalproto.ACPPreviewRef{Name: previewName(p.Name, p.Path), Reason: "unavailable"}
	if p.Reason != "" {
		ref.Reason = p.Reason
		return ref
	}
	if s == nil || !terminalproto.ValidID(session) {
		return ref
	}
	data := p.Data
	if len(data) == 0 && p.Path != "" {
		var err error
		data, err = s.readAgentFile(dir, p.Path)
		if err != nil {
			return ref
		}
	}
	if len(data) > terminalproto.MaxACPPreviewBytes {
		ref.Reason = "too_large"
		return ref
	}
	mime, _, ok := terminalproto.SniffUploadMIME(data)
	if !ok || !boundedPreviewImage(data, mime) {
		ref.Reason = "unsupported"
		return ref
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	s.mu.Lock()
	defer s.mu.Unlock()
	if os.MkdirAll(s.dir, 0700) != nil {
		return ref
	}
	s.pruneLocked(int64(len(data)))
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return ref
	}
	defer root.Close()
	if root.MkdirAll(session, 0700) != nil {
		return ref
	}
	target := filepath.Join(session, hash)
	temporary := target + ".tmp-" + newID()
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return ref
	}
	_, err = file.Write(data)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = root.Rename(temporary, target)
	}
	if err != nil {
		_ = root.Remove(temporary)
		return ref
	}
	return terminalproto.ACPPreviewRef{ID: hash, Name: ref.Name, MIME: mime, Size: int64(len(data))}
}

func (s *chatPreviewStore) readAgentFile(dir, path string) ([]byte, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	path = filepath.Clean(path)
	for _, base := range []string{s.home, os.TempDir()} {
		if base == "" {
			continue
		}
		rel, err := filepath.Rel(base, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		root, err := os.OpenRoot(base)
		if err != nil {
			continue
		}
		f, err := root.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		root.Close()
		if err != nil {
			return nil, err
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || !stat.Mode().IsRegular() || stat.Size() > terminalproto.MaxACPPreviewBytes {
			return nil, os.ErrInvalid
		}
		if owner, ok := stat.Sys().(*syscall.Stat_t); !ok || int(owner.Uid) != os.Geteuid() {
			return nil, os.ErrPermission
		}
		return io.ReadAll(io.LimitReader(f, terminalproto.MaxACPPreviewBytes+1))
	}
	return nil, os.ErrPermission
}

func (s *chatPreviewStore) read(session string, in terminalproto.ACPPreview) (terminalproto.ACPPreviewResult, error) {
	if s == nil || !terminalproto.ValidID(session) || !terminalproto.ValidACPPreviewID(in.ID) || in.Offset < 0 || in.Offset >= terminalproto.MaxACPPreviewBytes {
		return terminalproto.ACPPreviewResult{}, os.ErrNotExist
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return terminalproto.ACPPreviewResult{}, err
	}
	defer root.Close()
	name := filepath.Join(session, in.ID)
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return terminalproto.ACPPreviewResult{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() < 1 || st.Size() > terminalproto.MaxACPPreviewBytes || time.Since(st.ModTime()) > previewCacheAge || in.Offset >= st.Size() {
		return terminalproto.ACPPreviewResult{}, os.ErrNotExist
	}
	header := make([]byte, 16)
	n, _ := f.ReadAt(header, 0)
	mime, _, ok := terminalproto.SniffUploadMIME(header[:n])
	if !ok {
		return terminalproto.ACPPreviewResult{}, os.ErrInvalid
	}
	data := make([]byte, min(int64(terminalproto.MaxACPPreviewChunk), st.Size()-in.Offset))
	if _, err = f.ReadAt(data, in.Offset); err != nil {
		return terminalproto.ACPPreviewResult{}, err
	}
	return terminalproto.ACPPreviewResult{ID: in.ID, Offset: in.Offset, Size: st.Size(), MIME: mime, Data: data}, nil
}

func (s *chatPreviewStore) pruneLocked(reserve int64) {
	type entry struct {
		path string
		size int64
		at   time.Time
	}
	var files []entry
	var dirs []string
	var total int64
	_ = filepath.WalkDir(s.dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != s.dir {
			dirs = append(dirs, path)
		}
		if err != nil || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if time.Since(info.ModTime()) > previewCacheAge {
			_ = os.Remove(path)
			return nil
		}
		files = append(files, entry{path, info.Size(), info.ModTime()})
		total += info.Size()
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].at.Before(files[j].at) })
	for i, f := range files {
		if total+reserve <= previewCacheBytes && len(files)-i < previewCacheFiles {
			break
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i])
	}
}

func boundedPreviewImage(data []byte, mime string) bool {
	var width, height int
	if mime != "image/webp" {
		c, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return false
		}
		width, height = c.Width, c.Height
	} else if len(data) >= 30 {
		switch string(data[12:16]) {
		case "VP8X":
			width = 1 + int(data[24]) + int(data[25])<<8 + int(data[26])<<16
			height = 1 + int(data[27]) + int(data[28])<<8 + int(data[29])<<16
		case "VP8 ":
			if string(data[23:26]) != "\x9d\x01\x2a" {
				return false
			}
			width = int(binary.LittleEndian.Uint16(data[26:28]) & 0x3fff)
			height = int(binary.LittleEndian.Uint16(data[28:30]) & 0x3fff)
		case "VP8L":
			if data[20] != 0x2f {
				return false
			}
			bits := binary.LittleEndian.Uint32(data[21:25])
			width = 1 + int(bits&0x3fff)
			height = 1 + int(bits>>14&0x3fff)
		}
	}
	return width > 0 && height > 0 && width <= 16384 && height <= 16384 && int64(width)*int64(height) <= 32_000_000
}
