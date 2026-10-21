package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Escrow is the instance's backup key and every destination, secrets
// included, so a machine rebuilt from nothing can be given both without the
// database they were saved in. It is sealed under a passphrase a person
// chooses and kept away from this machine and the backup destination.
type Escrow struct {
	CreatedAt    int64          `json:"createdAt"`
	Version      string         `json:"version,omitempty"`
	Key          string         `json:"key"`
	Destinations []EscrowRemote `json:"destinations"`
}

// EscrowRemote is one destination in an escrow, with its secret readable
// only after the escrow is opened.
type EscrowRemote struct {
	Name      string `json:"name"`
	Source    string `json:"source"`
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
	PathStyle *bool  `json:"pathStyle,omitempty"`
	Keep      int    `json:"keep"`
	Disabled  bool   `json:"disabled"`
}

// Escrow returns the key and every destination for one owner, secrets
// opened. A destination whose secret cannot be opened is an error rather
// than an escrow that quietly leaves it out.
func (s *Service) Escrow(ownerID int64) (Escrow, error) {
	if s.cfg.Sealer == nil || s.cfg.Key == "" {
		return Escrow{}, errors.New("backups are not enabled: no usable backup key")
	}
	out := Escrow{CreatedAt: time.Now().UnixMilli(), Version: s.cfg.Version, Key: s.cfg.Key}
	remotes, err := s.loadRemotes(ownerID)
	if err != nil {
		return out, err
	}
	for _, r := range remotes {
		if r.Problem != "" {
			return out, fmt.Errorf("%s: %s", r.Name, r.Problem)
		}
		out.Destinations = append(out.Destinations, EscrowRemote{
			Name: r.Name, Source: r.Source, Endpoint: r.Endpoint, Region: r.Region,
			Bucket: r.Bucket, Prefix: r.Prefix, AccessKey: r.AccessKey,
			SecretKey: r.S3.SecretKey, PathStyle: r.S3.PathStyle, Keep: r.Keep, Disabled: r.Disabled,
		})
	}
	return out, nil
}

// SealEscrow seals an escrow under a passphrase, compressed and with the
// copy format's password protection.
func SealEscrow(e Escrow, passphrase string) ([]byte, error) {
	if len(passphrase) < MinPassword {
		return nil, fmt.Errorf("the escrow passphrase needs at least %d characters", MinPassword)
	}
	payload, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return nil, err
	}
	sealer, _ := NewSealer(strings.Repeat("0", 32), "escrow")
	m := Manifest{ID: "escrow-" + time.Now().UTC().Format("20060102T150405Z"), Kind: "escrow", Target: "keys"}
	return sealer.SealArchive(m, payload, SealOptions{Compress: true, Password: passphrase})
}

// OpenEscrow opens an escrow written by SealEscrow.
func OpenEscrow(data []byte, passphrase string) (Escrow, error) {
	if passphrase == "" {
		return Escrow{}, errors.New("the escrow needs its passphrase")
	}
	_, payload, err := OpenArchive(data, passphrase, "")
	if err != nil {
		return Escrow{}, err
	}
	var e Escrow
	if err := json.Unmarshal(payload, &e); err != nil {
		return Escrow{}, errors.New("this file is not a reet escrow")
	}
	if e.Key == "" {
		return Escrow{}, errors.New("this escrow holds no backup key")
	}
	return e, nil
}
