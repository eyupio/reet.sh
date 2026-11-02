package tui

import (
	"charm.land/huh/v2"

	"github.com/eyupio/reet.sh/internal/clientconfig"
)

// tokenField asks for a token only when none can be found already. A token in
// REET_TOKEN, an env file or the saved token is used silently; the form says
// where it came from so nobody retypes a secret the machine already has.
func tokenField(value *string, conn clientconfig.Connection) huh.Field {
	if conn.HasToken() {
		return huh.NewNote().
			Title("Token").
			Description("Using the token from " + conn.TokenFrom + ". Nothing to enter.")
	}
	return huh.NewInput().
		Title("Token").
		Description("Personal API token from Account on the controller. It is saved for next time.").
		EchoMode(huh.EchoModePassword).
		Value(value)
}

// serverDescription says where a prefilled controller URL came from.
func serverDescription(conn clientconfig.Connection, fallback string) string {
	if conn.Server != "" && conn.ServerFrom != "" && conn.ServerFrom != "option" {
		return "From " + conn.ServerFrom + "; edit to use another controller."
	}
	return fallback
}

// tokenFileOr prefers an explicit token file, then the env file a token was found in.
func tokenFileOr(explicit string, conn clientconfig.Connection) string {
	if explicit != "" {
		return explicit
	}
	return conn.TokenFile
}
