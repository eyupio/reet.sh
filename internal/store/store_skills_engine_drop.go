package store

import "database/sql"

// engineDropTables held the plan, preview and approve sync engine: sync
// targets and their options, runs, receipts and installs, library selections,
// the content targets, previews and receipts, the connectors' bases and edit
// events, and the old ignore list, which skill-states-v1 turned into Archived
// items. Every machine now follows the library through the desired state and
// its reports. A table that references another comes before it.
var engineDropTables = []string{
	"sync_run_jobs", "sync_runs", "sync_receipts", "skill_installs",
	"sync_targets", "sync_options", "sync_policies", "sync_library",
	"canonical_library_selections", "library_selections", "library_selection_accounts",
	"library_bases", "library_connectors", "library_difference_choices", "library_edit_events",
	"skill_content_targets", "skill_content_previews", "skill_content_preview_capabilities", "skill_content_receipts",
	"ignored",
}

// engineDropShelfColumns were the shelf's In use and To test flags. A skill's
// status is its library item's, Active or Archived.
var engineDropShelfColumns = []string{"used", "bookmarked"}

// migrateSkillsEngineDrop is skills-engine-drop-v1. It runs after every
// migration that could still read what it removes, in one transaction, and
// is safe to run again: the jobs are dropped once, and each table, trigger and
// column only if it is still there.
//
// The jobs dropped are the engine's that never started: queued, or awaiting
// the person's approval. Binary updates are sync_jobs rows of kind update and
// stay, whatever their status.
func (s *Store) migrateSkillsEngineDrop() error {
	return s.withTx(func(tx *sql.Tx) error {
		var migrated int
		if err := tx.QueryRow(`SELECT count(*) FROM library_migrations WHERE name='skills-engine-drop-v1'`).Scan(&migrated); err != nil {
			return err
		}
		if migrated == 0 {
			if _, err := tx.Exec(`DELETE FROM sync_jobs WHERE kind<>'update' AND status IN ('queued','awaiting-approval')`); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO library_migrations VALUES('skills-engine-drop-v1',?)`, now()); err != nil {
				return err
			}
		}
		// library_intent_fence moved sync targets and cancelled their jobs
		// when a library item changed; it reads tables dropped below.
		if _, err := tx.Exec(`DROP TRIGGER IF EXISTS library_intent_fence`); err != nil {
			return err
		}
		for _, table := range engineDropTables {
			if _, err := tx.Exec(`DROP TABLE IF EXISTS ` + table); err != nil {
				return err
			}
		}
		for _, column := range engineDropShelfColumns {
			var n int
			if err := tx.QueryRow(`SELECT count(*) FROM pragma_table_info('shelf') WHERE name=?`, column).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				continue
			}
			if _, err := tx.Exec(`ALTER TABLE shelf DROP COLUMN ` + column); err != nil {
				return err
			}
		}
		return nil
	})
}
