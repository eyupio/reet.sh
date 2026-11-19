package scan

import (
	"strings"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// A skill whose install failed is tried again after installRetryFirst, then
// after twice as long each time, up to installRetryMost.
const (
	installRetryFirst = 5 * time.Second
	installRetryMost  = 5 * time.Minute
)

type installBackoff struct {
	digest, detail string
	failures       int
	next           time.Time
}

// installBackoffs remembers, per home and lower-case name, an install that
// failed, so a bundle that cannot be installed is not fetched every cycle.
// It lasts as long as the process; a restarted agent tries at once.
var installBackoffs = struct {
	sync.Mutex
	m map[string]installBackoff
}{m: map[string]installBackoff{}}

func (r *reconciler) waitKey(f *folderRun, a store.DesiredSkill) string {
	return r.home + "\x00" + f.rel + "\x00" + strings.ToLower(a.Name)
}

// waiting says whether a's install in this folder failed lately for this
// revision, and why.
func (r *reconciler) waiting(f *folderRun, a store.DesiredSkill) (string, bool) {
	installBackoffs.Lock()
	defer installBackoffs.Unlock()
	b, ok := installBackoffs.m[r.waitKey(f, a)]
	if !ok || b.digest != a.Digest || r.now.After(b.next) {
		return "", false
	}
	return b.detail, true
}

func (r *reconciler) failedInstall(f *folderRun, a store.DesiredSkill, detail string) {
	installBackoffs.Lock()
	defer installBackoffs.Unlock()
	key := r.waitKey(f, a)
	b := installBackoffs.m[key]
	if b.digest != a.Digest {
		b = installBackoff{digest: a.Digest}
	}
	b.failures++
	wait := installRetryMost
	if b.failures <= 6 {
		wait = min(installRetryFirst<<(b.failures-1), installRetryMost)
	}
	b.next, b.detail = r.now.Add(wait), detail
	installBackoffs.m[key] = b
}

func (r *reconciler) recovered(f *folderRun, a store.DesiredSkill) {
	installBackoffs.Lock()
	defer installBackoffs.Unlock()
	delete(installBackoffs.m, r.waitKey(f, a))
}

// forgetWaits drops the waits of skills that are no longer Active.
func (r *reconciler) forgetWaits(active []store.DesiredSkill) {
	still := map[string]bool{}
	for _, a := range active {
		still[strings.ToLower(a.Name)] = true
	}
	installBackoffs.Lock()
	defer installBackoffs.Unlock()
	for key := range installBackoffs.m {
		home, rest, _ := strings.Cut(key, "\x00")
		_, name, _ := strings.Cut(rest, "\x00")
		if home == r.home && !still[name] {
			delete(installBackoffs.m, key)
		}
	}
}
