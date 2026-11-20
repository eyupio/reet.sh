package gaffer

import (
	"strings"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// outcomeImagesSuffix is the text that follows a prompt to name every image it
// does not already carry: one `Image:` line or one `Images: a b c` line, with
// the blank line before it. It is empty when there is nothing to add, so a
// prompt with no images is left exactly as it was.
func outcomeImagesSuffix(prompt string, images []string) string {
	var add []string
	for _, image := range images {
		ref := terminalproto.UploadReference(image)
		if ref == "" || strings.Contains(prompt, ref) {
			continue
		}
		add = append(add, ref)
	}
	if len(add) == 0 {
		return ""
	}
	line := "Image: " + add[0]
	if len(add) > 1 {
		line = "Images: " + strings.Join(add, " ")
	}
	if strings.TrimRight(prompt, " \t\r\n") == "" {
		return line
	}
	return "\n\n" + line
}

// OutcomePromptImages gives the opening prompt every uploaded image the
// outcome holds, not only the first. An image the prompt already names is left
// alone: a prompt written at approval is not doubled, and an outcome with no
// images is sent exactly as it was.
func OutcomePromptImages(prompt string, images []string) string {
	suffix := outcomeImagesSuffix(prompt, images)
	if suffix == "" {
		return prompt
	}
	return strings.TrimRight(prompt, " \t\r\n") + suffix
}
