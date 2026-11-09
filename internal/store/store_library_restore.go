package store

import (
	"database/sql"
	"encoding/hex"
	"fmt"
)

// LibraryRestoreReceipt is a machine's report that it restored a managed item
// after a stable local deletion and verified the result. Generation is not
// used: the receipt describes something already done, so it is judged against
// current intent rather than fenced to a snapshot.
type LibraryRestoreReceipt struct {
	LibraryDeploymentCheck
	Episode string `json:"episode"`
	Whole   bool   `json:"whole,omitempty"`
	Files   int    `json:"files,omitempty"`
}

func validEpisode(episode string) bool {
	raw, err := hex.DecodeString(episode)
	return err == nil && len(raw) == 16
}

// QueueLibraryRestoreSyncFor asks for an automatic sync because the machine
// has seen a deletion persist that the controller could not see for itself (a
// file missing inside an installed folder). It queues only for an item that is
// still wanted on a machine that is syncing, and at most one job at a time.
func (s *Store) QueueLibraryRestoreSyncFor(uid int64, enrollment string, in LibraryDeploymentCheck) (queued bool, err error) {
	in.Generation = ""
	err = s.withTx(func(tx *sql.Tx) error {
		intent, e := libraryDeploymentIntentTx(tx, uid, enrollment, in)
		if e != nil {
			return e
		}
		if intent.Action != "maintain" {
			return nil
		}
		var paused, keepSynced bool
		if e = tx.QueryRow(`SELECT paused,keep_synced FROM sync_agents WHERE id=? AND user_id=?`, enrollment, uid).Scan(&paused, &keepSynced); e != nil {
			return e
		}
		if paused || !keepSynced {
			return nil
		}
		var pending int
		if e = tx.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE user_id=? AND agent_id=? AND kind<>'update' AND status IN ('queued','running')`, uid, enrollment).Scan(&pending); e != nil || pending > 0 {
			return e
		}
		if _, e = insertSyncJobTx(tx, uid, enrollment, in.TargetID, "auto"); e != nil {
			return e
		}
		queued = true
		return nil
	})
	return queued, err
}

// RecordLibraryRestoreFor turns a verified restore into one informational
// notification-centre event. The key is the deletion episode, so retries,
// reconnects and replays of the same report tell nobody twice. If the item is
// no longer wanted (deactivated, deselected or deleted while the machine
// worked) the receipt is accepted but nothing is announced: the restore is not
// a fact the person needs, and the machine must not retry it forever.
func (s *Store) RecordLibraryRestoreFor(uid int64, enrollment string, in LibraryRestoreReceipt) (Notification, bool, error) {
	if !validEpisode(in.Episode) {
		return Notification{}, false, ErrPrivateSkillInvalid
	}
	in.Generation = ""
	var name, host string
	var wanted bool
	err := s.withTx(func(tx *sql.Tx) error {
		intent, e := libraryDeploymentIntentTx(tx, uid, enrollment, in.LibraryDeploymentCheck)
		if e != nil {
			return e
		}
		if e = tx.QueryRow(`SELECT host FROM sync_agents WHERE id=? AND user_id=?`, enrollment, uid).Scan(&host); e != nil {
			return e
		}
		name, wanted = intent.Name, intent.Action == "maintain"
		return nil
	})
	if err != nil {
		return Notification{}, false, err
	}
	if !wanted {
		return Notification{}, false, nil
	}
	body := "A file in it was missing, so reet.sh restored the saved version."
	switch {
	case in.Whole:
		body = "The folder was missing, so reet.sh restored the saved version."
	case in.Files > 1:
		body = fmt.Sprintf("%d files in it were missing, so reet.sh restored the saved version.", in.Files)
	}
	return s.RecordNotification(Notification{
		UserID: uid, Kind: "library_restore", Tone: "info", Link: "/machines",
		Title: fmt.Sprintf("Restored %s on %s after it was deleted locally.", name, host),
		Body:  body,
		Key:   "library-restore:" + enrollment + ":" + in.Episode,
	})
}
