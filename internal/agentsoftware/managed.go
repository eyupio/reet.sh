package agentsoftware

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	// ErrNotManaged means there is no reet-managed installation of that tool.
	// A system or package-manager installation is never treated as reet's own.
	ErrNotManaged = errors.New("reet did not install this tool on that machine")
	// ErrNoRollback means no earlier reet-managed version was kept.
	ErrNoRollback = errors.New("there is no earlier reet-managed version to roll back to")
	// ErrManagedBroken means a receipt exists but its binary is missing or no
	// longer matches the recorded size.
	ErrManagedBroken = errors.New("the reet-managed installation is incomplete or changed")
)

// Receipt is the evidence for one managed version. It is written beside the
// binary and, for the current version, at the root of the managed tree.
type Receipt struct {
	Tool         string `json:"tool"`
	Version      string `json:"version"`
	Tarball      string `json:"tarball"`
	SHA512       string `json:"sha512"`
	BinarySHA256 string `json:"binarySha256"`
	BinarySize   int64  `json:"binarySize"`
	InstalledAt  int64  `json:"installedAt"`
	Previous     string `json:"previous,omitempty"`
}

// Root is the directory reet owns for one tool. Nothing outside it is removed
// by remove or rollback, and a person's configuration and history never live
// here.
func Root(home, tool string) (string, error) {
	if !filepath.IsAbs(home) || !terminalproto.ValidTool(tool) {
		return "", fmt.Errorf("%w: invalid home or tool", ErrNotManaged)
	}
	return filepath.Join(home, ".reet", "software", tool), nil
}

func versionDir(root, version string) (string, error) {
	if !validVersion(version) {
		return "", fmt.Errorf("%w: invalid version", ErrManagedBroken)
	}
	return filepath.Join(root, "versions", version), nil
}

func validVersion(version string) bool {
	if version == "" || len(version) > 64 || version == "." || version == ".." {
		return false
	}
	for _, r := range version {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '.', r == '-', r == '+':
		default:
			return false
		}
	}
	return true
}

// Current returns the managed binary for tool when a valid receipt and binary
// exist. A nil error with found=false means reet has no managed installation;
// an error means a receipt exists but its binary cannot be trusted.
func Current(home, tool string) (string, Receipt, bool, error) {
	root, err := Root(home, tool)
	if err != nil {
		return "", Receipt{}, false, err
	}
	receipt, found, err := readReceipt(filepath.Join(root, "receipt.json"))
	if err != nil || !found {
		return "", Receipt{}, false, err
	}
	dir, err := versionDir(root, receipt.Version)
	if err != nil {
		return "", receipt, false, err
	}
	if _, found, err := readReceipt(filepath.Join(dir, "receipt.json")); err != nil || !found {
		return "", receipt, false, ErrManagedBroken
	}
	path := filepath.Join(dir, "opencode")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != receipt.BinarySize {
		return "", receipt, false, ErrManagedBroken
	}
	return path, receipt, true, nil
}

// ManagedBinary is the launch-time view: no receipt means the caller may use
// PATH; a broken receipt is an error, so a different binary is never run in
// place of the one reet recorded.
func ManagedBinary(home, tool string) (string, bool, error) {
	path, _, found, err := Current(home, tool)
	if err != nil {
		return "", false, err
	}
	return path, found, nil
}

// InstallStaged puts a staged, verified binary under the managed root and makes it
// current. The previous managed version is kept for rollback; a pre-existing
// installation reet did not make is not touched.
func InstallStaged(home, tool, version string, staged Staged) (Receipt, error) {
	root, err := Root(home, tool)
	if err != nil {
		return Receipt{}, err
	}
	if !validVersion(version) || staged.Path == "" || staged.Size <= 0 || staged.BinarySHA256 == "" || staged.SHA512 == "" {
		return Receipt{}, fmt.Errorf("%w: invalid staged binary", ErrArtifactShape)
	}
	dir, err := versionDir(root, version)
	if err != nil {
		return Receipt{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Receipt{}, err
	}
	binary := filepath.Join(dir, "opencode")
	if err := copyVerified(staged.Path, binary, staged); err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{
		Tool: tool, Version: version, Tarball: staged.Artifact.URL, SHA512: staged.SHA512,
		BinarySHA256: staged.BinarySHA256, BinarySize: staged.Size, InstalledAt: time.Now().UnixMilli(),
	}
	if err := writeReceipt(filepath.Join(dir, "receipt.json"), receipt); err != nil {
		return Receipt{}, err
	}
	previous := ""
	if existing, found, err := readReceipt(filepath.Join(root, "receipt.json")); err != nil {
		return Receipt{}, err
	} else if found && existing.Version != version {
		previous = existing.Version
	}
	receipt.Previous = previous
	if err := writeReceipt(filepath.Join(root, "receipt.json"), receipt); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

// Rollback makes the previous managed version current again. It verifies the
// kept binary's full hash before recording it, and says so when there is no
// previous reet-managed version.
func Rollback(home, tool string) (Receipt, error) {
	root, err := Root(home, tool)
	if err != nil {
		return Receipt{}, err
	}
	current, found, err := readReceipt(filepath.Join(root, "receipt.json"))
	if err != nil {
		return Receipt{}, err
	}
	if !found {
		return Receipt{}, ErrNotManaged
	}
	if current.Previous == "" {
		return Receipt{}, ErrNoRollback
	}
	dir, err := versionDir(root, current.Previous)
	if err != nil {
		return Receipt{}, ErrNoRollback
	}
	previous, found, err := readReceipt(filepath.Join(dir, "receipt.json"))
	if err != nil || !found || previous.Version != current.Previous {
		return Receipt{}, ErrNoRollback
	}
	if err := verifyFile(filepath.Join(dir, "opencode"), previous); err != nil {
		return Receipt{}, err
	}
	previous.Previous = current.Version
	if err := writeReceipt(filepath.Join(root, "receipt.json"), previous); err != nil {
		return Receipt{}, err
	}
	return previous, nil
}

// RemoveManaged deletes the whole reet-managed tree for one tool. Configuration,
// history and credentials live outside it; a system installation is refused.
func RemoveManaged(home, tool string) error {
	root, err := Root(home, tool)
	if err != nil {
		return err
	}
	if _, found, err := readReceipt(filepath.Join(root, "receipt.json")); err != nil {
		return err
	} else if !found {
		return ErrNotManaged
	}
	return os.RemoveAll(root)
}

func readReceipt(path string) (Receipt, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Receipt{}, false, nil
	}
	if err != nil {
		return Receipt{}, false, err
	}
	var receipt Receipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return Receipt{}, false, err
	}
	if receipt.Tool == "" || !validVersion(receipt.Version) || receipt.BinarySize <= 0 || receipt.BinarySHA256 == "" {
		return Receipt{}, false, fmt.Errorf("%w: invalid receipt", ErrManagedBroken)
	}
	return receipt, true, nil
}

func writeReceipt(path string, receipt Receipt) error {
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".receipt-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	cleanup := func() {
		file.Close()
		os.Remove(temp)
	}
	if _, err := file.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := file.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := file.Chmod(0o644); err != nil {
		cleanup()
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(temp)
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
		return err
	}
	return nil
}

func copyVerified(source, destination string, staged Staged) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	dir := filepath.Dir(destination)
	out, err := os.CreateTemp(dir, ".opencode-*")
	if err != nil {
		return err
	}
	temp := out.Name()
	cleanup := func() {
		out.Close()
		os.Remove(temp)
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(out, hash), io.LimitReader(in, staged.Size+1))
	if err != nil {
		cleanup()
		return err
	}
	if written != staged.Size || hex.EncodeToString(hash.Sum(nil)) != staged.BinarySHA256 {
		cleanup()
		return ErrArtifactChecksum
	}
	if err := out.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := out.Chmod(0o755); err != nil {
		cleanup()
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(temp)
		return err
	}
	if err := os.Rename(temp, destination); err != nil {
		os.Remove(temp)
		return err
	}
	return nil
}

func verifyFile(path string, receipt Receipt) error {
	file, err := os.Open(path)
	if err != nil {
		return ErrManagedBroken
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != receipt.BinarySize {
		return ErrManagedBroken
	}
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, receipt.BinarySize+1))
	if err != nil || written != receipt.BinarySize || hex.EncodeToString(hash.Sum(nil)) != receipt.BinarySHA256 {
		return ErrManagedBroken
	}
	return nil
}
