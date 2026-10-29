//go:build linux || darwin

package terminalhost

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
	"golang.org/x/sys/unix"
)

var (
	ErrUploadDisabled = errors.New("image uploads are disabled")
	ErrUploadTooLarge = errors.New("image upload exceeds the size limit")
	ErrUploadType     = errors.New("uploaded file is not an accepted image type")
	ErrUploadQuota    = errors.New("image upload quota reached")
	ErrUploadMismatch = errors.New("uploaded bytes do not match their declared size or hash")
	ErrUploadFailed   = errors.New("image upload could not be stored")
)

const maxUploadAge = 24 * time.Hour

// uploadDeadline bounds one upload on the machine, including a peer that
// stops sending. It is longer than the controller's own request deadline.
var uploadDeadline = 60 * time.Second

// uploadSweepInterval is how often the running connector applies the 24-hour
// lifetime to uploads for sessions that stay open.
var uploadSweepInterval = time.Hour

type uploadReservation struct {
	files int
	bytes int64
}

// WriteUpload stores an image sent by the controller in a private folder for
// the session. The controller supplies no name; the folder and file name are
// generated here. It never holds the manager lock while reading the bytes.
func (m *Manager) WriteUpload(ctx context.Context, sessionID string, req terminalproto.Upload, src io.Reader) (string, error) {
	if !terminalproto.ValidID(sessionID) || src == nil {
		return "", terminalproto.ErrInvalid
	}
	if err := validateUploadRequest(req); err != nil {
		return "", err
	}
	m.mu.Lock()
	if err := m.allowed(); err != nil {
		m.mu.Unlock()
		return "", err
	}
	if m.uploadConsent != nil && m.uploadConsent() != nil {
		m.mu.Unlock()
		return "", ErrUploadDisabled
	}
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, uploadDeadline)
	defer cancel()
	if err := m.uploadSessionRunning(ctx, sessionID); err != nil {
		return "", err
	}
	return m.storeUpload(ctx, sessionID, req, src)
}

// uploadSessionRunning checks the durable registry without holding m.mu while
// the bytes are read.
func (m *Manager) uploadSessionRunning(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	lock, err := m.registryLock(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrUploadFailed
	}
	defer lock.Close()
	registry, err := m.loadRegistry()
	if err != nil {
		return ErrRegistry
	}
	s, ok := registry.Sessions[sessionID]
	if !ok || s.State != terminalproto.Running {
		return ErrSessionMissing
	}
	return nil
}

func validateUploadRequest(req terminalproto.Upload) error {
	if !terminalproto.ValidID(req.StreamToken) || req.Size < 1 {
		return terminalproto.ErrInvalid
	}
	if req.Size > terminalproto.MaxUploadBytes {
		return ErrUploadTooLarge
	}
	if len(req.SHA256) != 64 {
		return terminalproto.ErrInvalid
	}
	for _, c := range req.SHA256 {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return terminalproto.ErrInvalid
		}
	}
	switch req.MIME {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return nil
	}
	return terminalproto.ErrInvalid
}

func (m *Manager) storeUpload(ctx context.Context, sessionID string, req terminalproto.Upload, src io.Reader) (string, error) {
	root := filepath.Join(m.layout.Dir, "uploads")
	m.uploadMu.Lock()
	if err := m.checkUploadQuotaLocked(root, sessionID, req.Size); err != nil {
		m.uploadMu.Unlock()
		return "", err
	}
	m.uploadMu.Unlock()
	defer m.releaseUpload(sessionID, req.Size)
	// Recheck after reserving: a session can end between the first check and
	// the quota decision, and cleanup may already have removed its folder.
	if err := m.uploadSessionRunning(ctx, sessionID); err != nil {
		return "", err
	}
	if err := ensurePrivateDir(root); err != nil {
		return "", ErrUploadFailed
	}
	dir := filepath.Join(root, sessionID)
	if err := ensurePrivateDir(dir); err != nil {
		return "", ErrUploadFailed
	}
	tempPath, file, err := createUploadTemp(dir)
	if err != nil {
		return "", ErrUploadFailed
	}
	closed, renamed := false, false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		if !renamed {
			_ = os.Remove(tempPath)
		}
	}()

	hasher := sha256.New()
	header := &uploadHeader{}
	reader := &uploadReader{ctx: ctx, reader: io.LimitReader(src, req.Size+1)}
	written, err := io.CopyBuffer(io.MultiWriter(file, hasher, header), reader, make([]byte, 32<<10))
	if err != nil {
		return "", ErrUploadFailed
	}
	if written > req.Size {
		return "", ErrUploadMismatch
	}
	if written != req.Size || hex.EncodeToString(hasher.Sum(nil)) != req.SHA256 {
		return "", ErrUploadMismatch
	}
	mime, ext, ok := terminalproto.SniffUploadMIME(header.bytes())
	if !ok || mime != req.MIME {
		return "", ErrUploadType
	}
	if err := file.Sync(); err != nil {
		return "", ErrUploadFailed
	}
	if err := file.Close(); err != nil {
		return "", ErrUploadFailed
	}
	closed = true
	finalPath, err := publishUpload(dir, tempPath, ext)
	if err != nil {
		return "", ErrUploadFailed
	}
	renamed = true
	return finalPath, nil
}

type uploadReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *uploadReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

type uploadHeader struct {
	buf [12]byte
	n   int
}

func (h *uploadHeader) Write(p []byte) (int, error) {
	if room := len(h.buf) - h.n; room > 0 {
		take := len(p)
		if take > room {
			take = room
		}
		copy(h.buf[h.n:], p[:take])
		h.n += take
	}
	return len(p), nil
}

func (h *uploadHeader) bytes() []byte { return h.buf[:h.n] }

func createUploadFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "upload"), nil
}

func createUploadTemp(dir string) (string, *os.File, error) {
	for i := 0; i < 8; i++ {
		suffix, err := randomUploadHex()
		if err != nil {
			return "", nil, err
		}
		path := filepath.Join(dir, ".tmp-"+suffix)
		file, err := createUploadFile(path)
		if err == nil {
			return path, file, nil
		}
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return "", nil, err
	}
	return "", nil, fs.ErrExist
}

func publishUpload(dir, tempPath, ext string) (string, error) {
	for i := 0; i < 8; i++ {
		suffix, err := randomUploadHex()
		if err != nil {
			return "", err
		}
		finalPath := filepath.Join(dir, "img-"+suffix+"."+ext)
		if _, err := os.Lstat(finalPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if err := os.Rename(tempPath, finalPath); err != nil {
			return "", err
		}
		return finalPath, nil
	}
	return "", fs.ErrExist
}

func randomUploadHex() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// checkUploadQuotaLocked reserves one file and size bytes for an upload.
// Callers hold uploadMu; the reservation is released when the transfer ends.
func (m *Manager) checkUploadQuotaLocked(root, sessionID string, size int64) error {
	files, used, err := uploadFolderUsage(filepath.Join(root, sessionID))
	if err != nil {
		return ErrUploadFailed
	}
	held := m.uploadReserved[sessionID]
	if files+held.files+1 > terminalproto.MaxUploadsPerSession || used+held.bytes+size > terminalproto.MaxUploadSessionBytes {
		return ErrUploadQuota
	}
	if m.uploadReserved == nil {
		m.uploadReserved = map[string]uploadReservation{}
	}
	total := int64(0)
	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		return ErrUploadFailed
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() || !terminalproto.ValidID(name) {
			continue
		}
		_, bytes, err := uploadFolderUsage(filepath.Join(root, name))
		if err != nil {
			return ErrUploadFailed
		}
		total += bytes
	}
	for _, reservation := range m.uploadReserved {
		total += reservation.bytes
	}
	if total+size > terminalproto.MaxUploadMachineBytes {
		return ErrUploadQuota
	}
	held.files++
	held.bytes += size
	m.uploadReserved[sessionID] = held
	return nil
}

func (m *Manager) releaseUpload(sessionID string, size int64) {
	m.uploadMu.Lock()
	defer m.uploadMu.Unlock()
	held := m.uploadReserved[sessionID]
	held.files--
	held.bytes -= size
	if held.files <= 0 || held.bytes <= 0 {
		delete(m.uploadReserved, sessionID)
		return
	}
	m.uploadReserved[sessionID] = held
}

func uploadFolderUsage(dir string) (int, int64, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	files := 0
	var used int64
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !strings.HasPrefix(entry.Name(), "img-") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files++
		used += info.Size()
	}
	return files, used, nil
}

// sweepUploadRoot removes folders for sessions that no longer reserve quota,
// sweeps files older than 24 hours in the remaining folders and removes empty
// folders. It never follows a symlink or touches anything outside root.
func sweepUploadRoot(root string, active map[string]bool, now time.Time) error {
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrUploadFailed
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !terminalproto.ValidID(name) {
			continue
		}
		dir := filepath.Join(root, name)
		if !active[name] {
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
			continue
		}
		inner, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, file := range inner {
			if file.Type()&os.ModeSymlink != 0 {
				continue
			}
			info, err := file.Info()
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			if info.ModTime().Before(now.Add(-maxUploadAge)) {
				_ = os.Remove(filepath.Join(dir, file.Name()))
			}
		}
		_ = os.Remove(dir)
	}
	return nil
}

func removeUploadFolder(root, sessionID string) error {
	if !terminalproto.ValidID(sessionID) {
		return terminalproto.ErrInvalid
	}
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrUploadFailed
	}
	path := filepath.Join(root, sessionID)
	info, err = os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return os.Remove(path)
	}
	if !info.IsDir() {
		return nil
	}
	return os.RemoveAll(path)
}

func (m *Manager) sweepUploads(active map[string]bool, now time.Time) error {
	// Cleanup deliberately does not wait on uploadMu: a folder removed under
	// an in-flight upload makes its publish fail, which is the right result
	// for a session that has ended.
	return sweepUploadRoot(filepath.Join(m.layout.Dir, "uploads"), active, now)
}

func (m *Manager) removeUploads(sessionID string) error {
	return removeUploadFolder(filepath.Join(m.layout.Dir, "uploads"), sessionID)
}
