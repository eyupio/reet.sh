package store

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
)

// Settings keys that say which controller this is and whether it has been told
// to stop acting. They live in the settings table, so a copy carries them; the
// restore removes the fence from what it brings (see AdaptDatabaseFile) because
// a controller that has just been restored is the one meant to be live.
const (
	settingControllerID = "controller_id"
	// SettingFenceSince and SettingFenceMovedTo are the persisted moved state.
	SettingFenceSince   = "fence_since"
	SettingFenceMovedTo = "fence_moved_to"
)

// ControllerID is this controller's own identifier, made the first time it is
// asked for and then kept. It names the controller, not its address, so a copy
// can say where it came from even after the controller changes address. A
// copy restored elsewhere carries the identifier with it: the id says which
// controller the data came from.
func (s *Store) ControllerID() (string, error) {
	if id, err := s.GetSetting(settingControllerID); err != nil || id != "" {
		return id, err
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// INSERT OR IGNORE so two first callers agree on whichever wrote first.
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES(?,?)`, settingControllerID, "ctl_"+hex.EncodeToString(b)); err != nil {
		return "", err
	}
	return s.GetSetting(settingControllerID)
}

// FenceSince is when this controller was fenced, in milliseconds since the
// Unix epoch, or 0 when it is not.
func (s *Store) FenceSince() (int64, error) {
	v, err := s.GetSetting(SettingFenceSince)
	if err != nil || v == "" {
		return 0, err
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0, nil
	}
	return n, nil
}
