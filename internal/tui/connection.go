package tui

import (
	"charm.land/huh/v2"

	"github.com/eyupio/reet.sh/internal/clientconfig"
)

// tokenField never makes anyone retype a token the machine already has: when one
// is found the field says where, and leaving it empty keeps it. Typing a value
// still overrides it, for example after switching to another controller.
func tokenField(value *string, conn clientconfig.Connection) huh.Field {
	if conn.HasToken() {
		return huh.NewInput().
			Title("Token").
			Description("Using the token from " + conn.TokenFrom + ". Leave empty to keep it, or enter another (hidden).").
			EchoMode(huh.EchoModePassword).
			Value(value)
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
