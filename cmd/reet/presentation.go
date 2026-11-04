package main

import "strings"

func structuredArgs(args []string) bool {
	for i, arg := range args {
		flag := arg == "--json" || arg == "--json=true" || arg == "--non-interactive" ||
			arg == "--non-interactive=true" || arg == "--output=json"
		outputJSON := arg == "--output" && i+1 < len(args) && args[i+1] == "json"
		if flag || outputJSON {
			return true
		}
		if strings.HasPrefix(arg, "--json=") && arg != "--json=false" {
			return true
		}
	}
	return false
}
