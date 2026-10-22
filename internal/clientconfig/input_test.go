package clientconfig

import (
	"bytes"
	"strings"
	"testing"
)

func TestRequireExistingAndNoninteractive(t *testing.T) {
	for _, secret := range []bool{false, true} {
		var out bytes.Buffer
		value, err := Require("existing", "setting", secret, strings.NewReader(""), &out)
		if err != nil || value != "existing" || out.Len() != 0 {
			t.Fatal("existing input prompted")
		}
		_, err = Require("", "--target", secret, strings.NewReader("unconsumed\n"), &out)
		if err == nil || !strings.Contains(err.Error(), "--target") || out.Len() != 0 {
			t.Fatal("noninteractive input should fail without prompting")
		}
	}
}
