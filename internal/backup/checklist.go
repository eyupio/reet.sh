package backup

import "strings"

// ChecklistItem is something a controller depends on that lives outside its
// database, so a copy cannot carry it and the operator sets it by hand on the
// machine the controller moves to.
type ChecklistItem struct {
	// Names are the settings the item covers.
	Names []string
	// Say is what to do about it.
	Say string
}

// Checklist is derived from the code, not from memory: every setting the
// server reads at start (`serve` in cmd/reet, the OIDC reader, the mail outbox
// setting, the backup reader and the list `reet install` saves) is named in an
// item, and a test fails when one is added to the code and not to this list.
// Settings only a client reads (REET_URL for a command run elsewhere) are not
// part of a controller and are not here.
func Checklist() []ChecklistItem {
	return []ChecklistItem{
		{[]string{"REET_DATA_DIR", "REET_ADDR", "REET_PUBLIC_URL"},
			"Where it keeps data, what it listens on, and its public address. Set REET_PUBLIC_URL to the new address; --public-url only rewrites the copies saved inside the database."},
		{[]string{"REET_AUTH", "REET_TOKEN", "REET_SIGNUP"},
			"How people sign in, the bootstrap and scan token, and whether sign-up is open."},
		{[]string{"REET_OIDC", "REET_OIDC_ISSUER", "REET_OIDC_CLIENT_ID", "REET_OIDC_CLIENT_SECRET", "REET_OIDC_CLIENT_SECRET_FILE", "REET_OIDC_REDIRECT_URL",
			"REET_OIDC_SCOPES", "REET_OIDC_PASSWORD_LOGIN", "REET_OIDC_AUTO_CREATE", "REET_OIDC_LINK_BY_EMAIL", "REET_OIDC_DEFAULT_ROLE", "REET_OIDC_BUTTON_LABEL",
			"REET_OIDC_EMAIL_CLAIM", "REET_OIDC_NAME_CLAIM", "REET_OIDC_GROUPS_CLAIM", "REET_OIDC_ADMIN_GROUPS", "REET_OIDC_MEMBER_GROUPS", "REET_OIDC_VIEWER_GROUPS"},
			"Single sign-on settings. The callback address changes with the controller, so register the new REET_OIDC_REDIRECT_URL with the identity provider."},
		{[]string{"REET_BACKUP_KEY"},
			"The backup key. The same key opens the copy's stored secrets; with another key they are cleared and listed above."},
		{[]string{"REET_BACKUP_DIR", "REET_BACKUP_EVERY", "REET_BACKUP_KEEP", "REET_BACKUP_USER_EVERY", "REET_BACKUP_USER_KEEP", "REET_BACKUP_DESTINATION",
			"REET_BACKUP_COMPRESSION", "REET_BACKUP_ALLOW_LOCAL", "REET_BACKUP_S3_NAME", "REET_BACKUP_S3_ENDPOINT", "REET_BACKUP_S3_REGION", "REET_BACKUP_S3_BUCKET",
			"REET_BACKUP_S3_PREFIX", "REET_BACKUP_S3_ACCESS_KEY_ID", "REET_BACKUP_S3_SECRET_ACCESS_KEY", "REET_BACKUP_S3_PASSPHRASE", "REET_BACKUP_S3_PATH_STYLE"},
			"Any backup schedule and destination set in the environment rather than saved in reet. `reet backup unescrow` prints a destination from an escrow."},
		{[]string{"REET_MAIL_OUTBOX_DIR"},
			"The development mail outbox folder, if you use one. Saved mail settings are in the database; a mail password sealed under another key was cleared."},
		{[]string{"REET_REMOTE_TERMINALS", "terminal-control/state.json"},
			"Terminal state. Whether remote terminals are on, and the public address and sign-in mode they were saved with, live in terminal-control/state.json in the data folder, which a copy does not carry. Set it on the new machine."},
		{[]string{"REET_SUPPORT_DESK", "REET_CONTACT_EMAIL", "REET_TOPICS", "REET_REFRESH", "REET_MAINTENANCE", "REET_LOG", "REET_PPROF_ADDR", "REET_RELAY_MAP_URL", "SKILLS_SH_TOKEN",
			"REET_LIBRARY_MANAGED_REMOVALS"},
			"Feature switches (including the library's managed-removal rollout switch, off unless it is 1), logging, the private-connection relay map and the skills.sh token."},
		{[]string{"REET_CONTROLLER_URL", "REET_CONTROLLER_TOKEN", "REET_ENV"},
			"If this machine reports itself to another controller, and the name it reports under."},
		{[]string{"the service definition"},
			"If reet runs as a service, `reet install` saves the settings above privately on the old machine; install the service on the new one and set them again."},
	}
}

// ChecklistLines is the checklist as a person reads it, ending a report.
func ChecklistLines() []string {
	lines := []string{"Not in the copy; set these by hand on the new machine:"}
	for _, it := range Checklist() {
		lines = append(lines, "  - "+strings.Join(it.Names, ", ")+": "+it.Say)
	}
	return lines
}
