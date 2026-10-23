package profilehistory

import "testing"

func TestOnlyAnIdenticalPrefixCanAdvance(t *testing.T) {
	for _, tc := range []struct {
		old, next string
		ok        bool
	}{
		{`{"revisions":[{"revision":1,"snapshot":"a"}]}`, `{"revisions":[{"snapshot":"a","revision":1}]}`, true},
		{`{"revisions":[{"revision":1}]}`, `{"revisions":[{"revision":1},{"revision":2}]}`, true},
		{`{"revisions":[{"revision":1},{"revision":2}]}`, `{"revisions":[{"revision":1}]}`, false},
		{`{"revisions":[{"revision":1,"snapshot":"a"}]}`, `{"revisions":[{"revision":1,"snapshot":"b"}]}`, false},
	} {
		if got := CanAdvance([]byte(tc.old), []byte(tc.next)); (got == nil) != tc.ok {
			t.Fatalf("%s -> %s: %v", tc.old, tc.next, got)
		}
	}
}
