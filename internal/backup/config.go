// Package backup takes sealed, verifiable copies of reet's database and of
// one person's shelf, keeps them beside the database, and ships them to any
// S3-compatible destination. The instance's copies belong to the owner of the
// instance; a person's own copies belong to that person and to nobody else.
package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

const (
	EnvDir         = "REET_BACKUP_DIR"
	EnvKey         = "REET_BACKUP_KEY"
	EnvEvery       = "REET_BACKUP_EVERY"
	EnvKeep        = "REET_BACKUP_KEEP"
	EnvUserEvery   = "REET_BACKUP_USER_EVERY"
	EnvUserKeep    = "REET_BACKUP_USER_KEEP"
	EnvDestination = "REET_BACKUP_DESTINATION"
	EnvCompression = "REET_BACKUP_COMPRESSION"
	EnvS3Endpoint  = "REET_BACKUP_S3_ENDPOINT"
	EnvS3Region    = "REET_BACKUP_S3_REGION"
	EnvS3Bucket    = "REET_BACKUP_S3_BUCKET"
	EnvS3Prefix    = "REET_BACKUP_S3_PREFIX"
	EnvS3AccessKey = "REET_BACKUP_S3_ACCESS_KEY_ID"
	EnvS3SecretKey = "REET_BACKUP_S3_SECRET_ACCESS_KEY"
	EnvS3PathStyle = "REET_BACKUP_S3_PATH_STYLE"
	EnvS3Name      = "REET_BACKUP_S3_NAME"
	EnvAllowLocal  = "REET_BACKUP_ALLOW_LOCAL"
)

const (
	DefaultEvery       = 24 * time.Hour
	DefaultKeep        = 48
	DefaultUserEvery   = 0
	DefaultUserKeep    = 14
	DefaultPrefix      = "reet/"
	DefaultDestination = "both"
	DefaultCompression = "gzip"
	DefaultName        = "offsite"
	MaxKeep            = 100000
)

// S3 is one destination.
type S3 struct {
	Endpoint  string
	Region    string
	Bucket    string
	Prefix    string
	AccessKey string
	SecretKey string
	PathStyle *bool
}

func (s S3) Configured() bool { return strings.TrimSpace(s.Endpoint) != "" }

func (s S3) Where() string {
	if !s.Configured() {
		return ""
	}
	return strings.TrimRight(s.Endpoint, "/") + "/" + s.Bucket + "/" + s.Prefix
}

// Config is what the environment says. Everything the owner saves on the
// Backups page overrides the schedule and retention here.
type Config struct {
	Dir         string
	Key         string
	KeySource   string
	Every       time.Duration
	Keep        int
	UserEvery   time.Duration
	UserKeep    int
	Destination string
	Compression string
	Version     string
	AllowLocal  bool
	RemoteName  string
	S3          S3
	Findings    []Finding
	Usable      bool
	KeyPath     string
	Sealer      *Sealer
}

// Finding is a configuration problem the Backups page shows in words.
type Finding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// Lookup reads a value from the environment or from the file its _FILE
// variant names, refusing both at once.
func Lookup(getenv func(string) string, name string) (string, error) {
	v, file := getenv(name), getenv(name+"_FILE")
	switch {
	case v != "" && file != "":
		return "", fmt.Errorf("both %s and %s_FILE are set; set one", name, name)
	case file != "":
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("reading %s_FILE: %w", name, err)
		}
		return strings.TrimSpace(string(raw)), nil
	}
	return strings.TrimSpace(v), nil
}

// FromEnv reads the configuration and prepares the key. A key the environment
// does not give is generated beside the database once, with a warning that it
// has to be kept somewhere else too if the off-site copies are to be read after
// this machine is lost.
func FromEnv(getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	data := getenv("REET_DATA_DIR")
	if data == "" {
		data = "./data"
	}
	var errs []error
	str := func(name, def string) string {
		v, err := Lookup(getenv, name)
		if err != nil {
			errs = append(errs, err)
		}
		if v == "" {
			return def
		}
		return v
	}
	dur := func(name string, def time.Duration) time.Duration {
		v := str(name, "")
		if v == "" {
			return def
		}
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			errs = append(errs, fmt.Errorf("%s is %q, which is not a duration like 1h or 24h (0 turns it off)", name, v))
			return def
		}
		if d > 0 && d < time.Minute {
			errs = append(errs, fmt.Errorf("%s is %s; the shortest schedule is a minute", name, d))
			return def
		}
		return d
	}
	compression := func(name, def string) string {
		v := strings.ToLower(str(name, def))
		if v == "" {
			v = def
		}
		if v != CompressionGzip && v != CompressionNone {
			errs = append(errs, fmt.Errorf("%s is %q; write gzip or none", name, v))
			return def
		}
		return v
	}
	destination := func(name, def string) string {
		v := strings.ToLower(str(name, def))
		if v == "" {
			v = def
		}
		if !store.ValidBackupDestination(v) {
			errs = append(errs, fmt.Errorf("%s is %q; write local, remote or both", name, v))
			return def
		}
		return v
	}
	num := func(name string, def int) int {
		v := str(name, "")
		if v == "" {
			return def
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > MaxKeep {
			errs = append(errs, fmt.Errorf("%s is %q, which is not a number of copies to keep (0 keeps every one)", name, v))
			return def
		}
		return n
	}
	c := Config{
		Dir:         str(EnvDir, filepath.Join(data, "backups")),
		Key:         str(EnvKey, ""),
		Every:       dur(EnvEvery, DefaultEvery),
		Keep:        num(EnvKeep, DefaultKeep),
		UserEvery:   dur(EnvUserEvery, DefaultUserEvery),
		UserKeep:    num(EnvUserKeep, DefaultUserKeep),
		Destination: destination(EnvDestination, DefaultDestination),
		Compression: compression(EnvCompression, DefaultCompression),
		AllowLocal:  strings.EqualFold(str(EnvAllowLocal, ""), "1") || strings.EqualFold(str(EnvAllowLocal, ""), "true"),
		RemoteName:  str(EnvS3Name, DefaultName),
	}
	c.KeyPath = filepath.Join(filepath.Dir(c.Dir), "backup.key")
	c.S3 = S3{
		Endpoint:  str(EnvS3Endpoint, ""),
		Region:    str(EnvS3Region, ""),
		Bucket:    str(EnvS3Bucket, ""),
		Prefix:    str(EnvS3Prefix, DefaultPrefix),
		AccessKey: str(EnvS3AccessKey, ""),
		SecretKey: str(EnvS3SecretKey, ""),
	}
	if c.S3.Prefix != "" && !strings.HasSuffix(c.S3.Prefix, "/") {
		c.S3.Prefix += "/"
	}
	if v := str(EnvS3PathStyle, ""); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s is %q; write true or false", EnvS3PathStyle, v))
		} else {
			c.S3.PathStyle = &b
		}
	}
	if c.Key == "" {
		if _, statErr := os.Stat(c.KeyPath); os.IsNotExist(statErr) && hasCopies(c.Dir) {
			errs = append(errs, fmt.Errorf("there are copies in %s but %s is missing; restore that key before starting, or they cannot be opened", c.Dir, c.KeyPath))
		} else if key, path, created, err := ensureKeyFile(c.KeyPath); err != nil {
			errs = append(errs, err)
		} else {
			c.Key, c.KeyPath, c.KeySource = key, path, "file"
			if created {
				c.Findings = append(c.Findings, Finding{Severity: "warning", Code: "backup.key_generated",
					Message: "A backup key was generated at " + path + ". Every copy is sealed with it; keep a copy of that file somewhere other than this machine, or the off-site copies cannot be opened after a disk loss."})
			} else {
				c.Findings = append(c.Findings, Finding{Severity: "info", Code: "backup.key_beside_data",
					Message: "Copies are sealed with the key at " + path + ", beside the database. Keep a copy of it somewhere else."})
			}
		}
	} else {
		c.KeySource = "environment"
		if len(c.Key) < 32 {
			errs = append(errs, fmt.Errorf("%s is shorter than 32 characters; use a value from `openssl rand -hex 32`", EnvKey))
		}
	}
	sealer, err := NewSealer(c.Key, c.KeySource)
	if err != nil {
		errs = append(errs, err)
	} else {
		c.Sealer = sealer
		c.Usable = true
	}
	if c.S3.Configured() && (c.S3.Bucket == "" || c.S3.AccessKey == "" || c.S3.SecretKey == "") {
		c.Findings = append(c.Findings, Finding{Severity: "error", Code: "backup.offsite_incomplete",
			Message: "REET_BACKUP_S3_ENDPOINT names an endpoint but the bucket, access key id and secret access key are not all set, so nothing can be shipped to it."})
	}
	if c.Destination == store.DestinationBoth && !c.S3.Configured() {
		c.Findings = append(c.Findings, Finding{Severity: "warning", Code: "backup.no_offsite",
			Message: "No off-site destination is configured. Copies are on this machine only and are lost with it; add an S3-compatible destination on Admin, Backups."})
	}
	if c.Every == 0 {
		c.Findings = append(c.Findings, Finding{Severity: "info", Code: "backup.instance_off",
			Message: "The instance's scheduled backup is off. Set an interval on Admin, Backups, or set " + EnvEvery + "."})
	}
	if !c.Usable {
		c.Findings = append(c.Findings, Finding{Severity: "error", Code: "backup.no_key",
			Message: "No usable backup key is set, so nothing is backed up. Set " + EnvKey + " (or " + EnvKey + "_FILE) to a value from `reet backup keygen`, or let reet generate " + c.KeyPath + "."})
	}
	return c, errors.Join(errs...)
}

// hasCopies reports whether a directory already holds a sealed copy.
func hasCopies(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".reetbackup") {
			return true
		}
	}
	return false
}

// GenerateKey makes a key fit for REET_BACKUP_KEY.
func GenerateKey() (string, error) { return randomHex(32) }

// ensureKeyFile reads the key file, or makes one with 32 random bytes.
func ensureKeyFile(path string) (key, actual string, created bool, err error) {
	if b, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(b)), path, false, nil
	} else if !os.IsNotExist(err) {
		return "", path, false, err
	}
	k, err := randomHex(32)
	if err != nil {
		return "", path, false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", path, false, err
	}
	if err := os.WriteFile(path, []byte(k+"\n"), 0o600); err != nil {
		return "", path, false, err
	}
	return k, path, true, nil
}
