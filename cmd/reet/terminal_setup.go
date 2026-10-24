package main

import (
	"errors"
	"fmt"
	"github.com/eyupio/reet.sh/internal/terminalcontrol"
	"io"
	"strings"
)

func prepareTerminalSetup(status terminalcontrol.Status, origin, auth string, ask func(string, string) (string, error), out io.Writer) (string, string, error) {
	if origin == "" {
		origin = status.PublicURL
	}
	for origin == "" || terminalcontrol.ValidateOrigin(origin) != nil {
		if ask == nil {
			if status.Available && origin == "" {
				break
			}
			return "", "", errors.New("public URL is missing or invalid; run reet terminal start interactively, or provide REET_PUBLIC_URL/REET_URL in .env")
		}
		if origin != "" {
			fmt.Fprintln(out, "Use an HTTPS public origin, or loopback HTTP for local development.")
		}
		value, err := ask("Public URL", origin)
		if err != nil {
			return "", "", err
		}
		origin = strings.TrimSpace(value)
	}
	effectiveAuth := status.AuthMode
	if effectiveAuth == "" {
		effectiveAuth = auth
	}
	if effectiveAuth == "" {
		effectiveAuth = "accounts"
	}
	if effectiveAuth != "accounts" && auth != "accounts" && !status.RestartRequired {
		if ask == nil {
			return "", "", errors.New("accounts authentication is required; run reet terminal start interactively to save setup")
		}
		for {
			answer, err := ask("Enable accounts authentication? A controller restart will be needed (yes/no)", "yes")
			if err != nil {
				return "", "", err
			}
			switch strings.ToLower(strings.TrimSpace(answer)) {
			case "yes", "y":
				return origin, "accounts", nil
			case "no", "n":
				return "", "", errors.New("setup cancelled; no changes saved")
			default:
				fmt.Fprintln(out, "Choose yes or no.")
			}
		}
	}
	return origin, "accounts", nil
}
