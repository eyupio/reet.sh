package skillimport

import (
	"errors"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"strings"
	"testing"
)

func TestSelectionExclusionsAndCredentialBlocking(t *testing.T) {
	base := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("instructions\r\n")}, {Path: "script.sh", Data: []byte("echo ok\n"), Executable: true}}
	for _, f := range []skillbundle.ContentFile{{Path: ".env", Data: []byte("TOKEN=private")}, {Path: "config/auth.json", Data: []byte("private")}, {Path: "support.txt", Data: []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nPRIVATE_VALUE")}, {Path: "key.txt", Data: []byte("ghp_" + strings.Repeat("a", 36))}} {
		files := append(append([]skillbundle.ContentFile{}, base...), f)
		_, _, err := Select(files, nil)
		var blocked *BlockedError
		if !errors.As(err, &blocked) || strings.Contains(err.Error(), "PRIVATE_VALUE") {
			t.Fatal("unsafe or leaked error", err)
		}
		m, selected, err := Select(files, []string{f.Path})
		if err != nil || len(selected) != 2 || !selected[1].Executable || !m.Complete {
			t.Fatal(selected, err)
		}
	}
	for _, exclude := range [][]string{{"SKILL.md"}, {"missing"}, {"script.sh", "script.sh"}, {"../script.sh"}} {
		if _, _, err := Select(base, exclude); err == nil {
			t.Fatal("invalid exclusion accepted", exclude)
		}
	}
}
