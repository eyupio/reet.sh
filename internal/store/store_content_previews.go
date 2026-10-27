package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/eyupio/reet.sh/internal/contentobs"
	"github.com/eyupio/reet.sh/internal/skillplan"
	"github.com/eyupio/reet.sh/internal/skillpreview"
)

const (
	SkillContentPreviewVersion = 1
	MaxSkillContentPreviews    = 64
	skillContentPreviewTTL     = 10 * 60 * 1000
	skillContentPreviewReady   = 90 * 1000
)

var (
	ErrSkillContentPreview         = errors.New("content preview changed, expired, busy or unavailable")
	ErrSkillContentPreviewMissing  = errors.New("content preview unavailable")
	ErrSkillContentPreviewInvalid  = errors.New("invalid content preview")
	ErrSkillContentPreviewCapacity = errors.New("content preview history full")
)

// SkillContentPreview is a metadata-only look at one machine for one pin. It is a
// proposal to read, never an installed receipt, an ownership claim or permission
// to write; Authority says so on the wire.
//
// Stored states are queued, capturing, captured, failed, cancelled, expired and
// superseded. Reads also report stale, for a capture older than the planner's
// freshness window. A fence that no longer holds is reported as superseded with
// reason codes, so an old review can never look current. ReceiptVersion is the
// version of the slot's ownership receipt as the plan was built, empty when the
// slot had none; a captured review is superseded once it differs.
type SkillContentPreview struct {
	ID             string `json:"id"`
	RequestKey     string `json:"requestKey"`
	TargetID       string `json:"targetId"`
	TargetVersion  string `json:"targetVersion"`
	EnvironmentID  string `json:"environmentId"`
	AgentID        string `json:"agentId"`
	Agent          string `json:"agent"`
	Project        string `json:"project"`
	Name           string `json:"name"`
	BundleID       string `json:"bundleId"`
	RevisionID     string `json:"revisionId"`
	ManifestDigest string `json:"manifestDigest"`
	Mode           string `json:"mode"`
	// Instance is the connector process this request was made for. The inventory
	// revision is bound when capture begins and is zero before then; it is a
	// 63-bit random value, so it travels as a string.
	Instance          string                `json:"instanceId"`
	InventoryRevision int64                 `json:"inventoryRevision,string"`
	State             string                `json:"state"`
	Reasons           []string              `json:"reasons"`
	CreatedAt         int64                 `json:"createdAt"`
	UpdatedAt         int64                 `json:"updatedAt"`
	ExpiresAt         int64                 `json:"expiresAt"`
	CapturedAt        int64                 `json:"capturedAt,omitempty"`
	Capture           *skillpreview.Capture `json:"capture,omitempty"`
	CaptureHash       string                `json:"captureHash,omitempty"`
	ReceiptVersion    string                `json:"receiptVersion,omitempty"`
	Plan              *skillplan.Plan       `json:"plan,omitempty"`
	ReviewHash        string                `json:"reviewHash,omitempty"`
	Authority         string                `json:"authority"`
	Error             string                `json:"error,omitempty"`
	// Adoption is computed on item reads only, never stored or listed.
	Adoption *SkillContentAdoptionOffer `json:"adoption,omitempty"`
}

type SkillContentPreviewCreate struct {
	RequestKey            string `json:"requestKey"`
	TargetID              string `json:"targetId"`
	ExpectedTargetVersion string `json:"expectedTargetVersion"`
}

// SkillContentPreviewWork is everything a connector learns: which local scope
// and skill folder to look at. It never names the private bundle, revision or
// pin, and the connector resolves the scope against its own configured roots.
type SkillContentPreviewWork struct {
	ID        string `json:"id"`
	Instance  string `json:"instanceId"`
	Agent     string `json:"agent"`
	Project   string `json:"project"`
	Name      string `json:"name"`
	ExpiresAt int64  `json:"expiresAt"`
}

type SkillContentPreviewCapability struct {
	AgentID    string `json:"agentId"`
	Ready      bool   `json:"ready"`
	ObservedAt int64  `json:"observedAt"`
}

// skillContentReviewIdentity is what a future approval must bind. The plan hash
// alone is not enough: a pin can change away and back to identical content, and
// only the pin's independent version tells those two intents apart. The receipt
// version does the same for ownership: a receipt released and re-adopted with
// identical content is a different decision from the one a review saw.
type skillContentReviewIdentity struct {
	Version           int    `json:"version"`
	OwnerID           int64  `json:"ownerId"`
	RequestID         string `json:"requestId"`
	TargetID          string `json:"targetId"`
	TargetVersion     string `json:"targetVersion"`
	BundleID          string `json:"bundleId"`
	RevisionID        string `json:"revisionId"`
	ManifestDigest    string `json:"manifestDigest"`
	Mode              string `json:"mode"`
	AgentID           string `json:"agentId"`
	Instance          string `json:"instanceId"`
	InventoryRevision int64  `json:"inventoryRevision"`
	CaptureHash       string `json:"captureHash"`
	PlanHash          string `json:"planHash"`
	ReceiptVersion    string `json:"receiptVersion"`
}

func (i skillContentReviewIdentity) Hash() string {
	i.Version = SkillContentPreviewVersion
	raw, _ := json.Marshal(i)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Store) migrateSkillContentPreviews() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS skill_content_preview_capabilities(agent_id TEXT PRIMARY KEY,user_id INTEGER NOT NULL,instance TEXT NOT NULL,observed_at INTEGER NOT NULL,FOREIGN KEY(agent_id,user_id) REFERENCES sync_agents(id,user_id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS skill_content_previews(id TEXT PRIMARY KEY,user_id INTEGER NOT NULL,agent_id TEXT NOT NULL,request_key TEXT NOT NULL,state TEXT NOT NULL,data TEXT NOT NULL,created_at INTEGER NOT NULL,UNIQUE(user_id,request_key),FOREIGN KEY(agent_id,user_id) REFERENCES sync_agents(id,user_id) ON DELETE CASCADE)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func activePreview(p SkillContentPreview) bool {
	return (p.State == "queued" || p.State == "capturing") && p.ExpiresAt >= now()
}

type previewQuerier interface {
	QueryRow(string, ...any) *sql.Row
}

func readPreview(q previewQuerier, uid int64, id string) (p SkillContentPreview, err error) {
	if !contentobs.ValidID(id) {
		return p, ErrSkillContentPreviewMissing
	}
	var raw string
	err = q.QueryRow(`SELECT data FROM skill_content_previews WHERE id=? AND user_id=? AND `+privateSkillAccount, id, uid, uid).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrSkillContentPreviewMissing
	}
	if err == nil {
		err = json.Unmarshal([]byte(raw), &p)
	}
	if p.Reasons == nil {
		p.Reasons = []string{}
	}
	return
}

func writePreview(tx *sql.Tx, uid int64, p SkillContentPreview) error {
	p.UpdatedAt = now()
	// An adoption offer is derived on item reads; a caller holding one must never
	// persist it.
	p.Adoption = nil
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE skill_content_previews SET state=?,data=? WHERE id=? AND user_id=?`, p.State, string(raw), p.ID, uid)
	return err
}

// currentInventoryRevision is the inventory identity the controller holds right
// now for the preview's scope on the preview's connector.
func currentInventoryRevision(q previewQuerier, uid int64, environmentID, agentID string) (int64, error) {
	var revision int64
	err := q.QueryRow(`SELECT sc.inventory_revision FROM environments e JOIN environment_scans sc ON sc.env_id=e.id JOIN sync_agents a ON a.user_id=e.user_id AND a.host=e.host COLLATE NOCASE WHERE e.user_id=? AND e.id=? AND a.id=?`, uid, environmentID, agentID).Scan(&revision)
	return revision, err
}

// previewFence lists every reason this request no longer describes the present.
// It reads only, so reads and writes share one definition of "still current".
// Callers hold the account guard (an active, non-viewer account) before asking:
// every read goes through privateSkillAccount and every write through
// privateSkillOwner, so a demoted account never reaches this point.
func previewFence(q previewQuerier, uid int64, p SkillContentPreview) ([]string, error) {
	reasons := []string{}
	var version string
	switch err := q.QueryRow(`SELECT version FROM skill_content_targets WHERE user_id=? AND id=?`, uid, p.TargetID).Scan(&version); {
	case errors.Is(err, sql.ErrNoRows):
		reasons = append(reasons, "pin_removed")
	case err != nil:
		return nil, err
	case version != p.TargetVersion:
		reasons = append(reasons, "pin_changed")
	}
	var paused bool
	switch err := q.QueryRow(`SELECT paused FROM sync_agents WHERE id=? AND user_id=?`, p.AgentID, uid).Scan(&paused); {
	case errors.Is(err, sql.ErrNoRows):
		reasons = append(reasons, "connector_removed")
	case err != nil:
		return nil, err
	case paused:
		reasons = append(reasons, "connector_paused")
	}
	var instance string
	switch err := q.QueryRow(`SELECT instance FROM skill_content_preview_capabilities WHERE agent_id=? AND user_id=?`, p.AgentID, uid).Scan(&instance); {
	case errors.Is(err, sql.ErrNoRows):
		reasons = append(reasons, "process_changed")
	case err != nil:
		return nil, err
	case instance != p.Instance:
		reasons = append(reasons, "process_changed")
	}
	if p.InventoryRevision != 0 {
		switch current, err := currentInventoryRevision(q, uid, p.EnvironmentID, p.AgentID); {
		case errors.Is(err, sql.ErrNoRows):
			reasons = append(reasons, "scope_removed")
		case err != nil:
			return nil, err
		case current != p.InventoryRevision:
			reasons = append(reasons, "inventory_changed")
		}
	}
	// Only a built plan binds a receipt version. A request still awaiting capture
	// reads whatever receipt exists in the transaction that builds its plan.
	if p.CapturedAt > 0 {
		switch current, err := currentReceiptVersion(q, uid, p.EnvironmentID, p.Name); {
		case err != nil:
			return nil, err
		case current != p.ReceiptVersion:
			reasons = append(reasons, "receipt_changed")
		}
	}
	return reasons, nil
}

func unionReasons(a, b []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, r := range append(append([]string{}, a...), b...) {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}

// effectivePreview applies the fences to a stored request without writing.
func effectivePreview(q previewQuerier, uid int64, p SkillContentPreview) (SkillContentPreview, error) {
	p.Authority = "preview_only"
	switch p.State {
	case "queued", "capturing":
		if p.ExpiresAt < now() {
			p.State = "expired"
			return p, nil
		}
	case "captured", "superseded":
	default:
		return p, nil
	}
	live, err := previewFence(q, uid, p)
	if err != nil {
		return p, err
	}
	p.Reasons = unionReasons(p.Reasons, live)
	switch {
	case p.State == "superseded":
	case len(live) > 0:
		p.State = "superseded"
	case p.State == "captured" && now()-p.CapturedAt > skillplan.MaxAgeMillis:
		p.State = "stale"
	}
	return p, nil
}

// currentPreview is a stored request with its fences applied, as every read sees it.
func currentPreview(q previewQuerier, uid int64, id string) (SkillContentPreview, error) {
	p, err := readPreview(q, uid, id)
	if err != nil {
		return p, err
	}
	return effectivePreview(q, uid, p)
}

// SkillContentPreviewFor is the item read. It alone carries the adoption offer,
// read in the same snapshot as the request's effective state.
func (s *Store) SkillContentPreviewFor(uid int64, id string) (out SkillContentPreview, err error) {
	err = s.withTx(func(tx *sql.Tx) error {
		p, e := currentPreview(tx, uid, id)
		if e != nil {
			return e
		}
		p.Adoption, e = adoptionOffer(tx, uid, p)
		out = p
		return e
	})
	return
}

// SkillContentPreviewsFor returns bounded summaries: no capture, no plan, no offer.
func (s *Store) SkillContentPreviewsFor(uid int64) ([]SkillContentPreview, []SkillContentPreviewCapability, error) {
	rows, err := s.db.Query(`SELECT id FROM skill_content_previews WHERE user_id=? AND `+privateSkillAccount+` ORDER BY created_at DESC,id`, uid, uid)
	if err != nil {
		return nil, nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	list := []SkillContentPreview{}
	for _, id := range ids {
		p, e := currentPreview(s.db, uid, id)
		if errors.Is(e, ErrSkillContentPreviewMissing) {
			continue
		}
		if e != nil {
			return nil, nil, e
		}
		p.Capture, p.Plan = nil, nil
		list = append(list, p)
	}
	caps := []SkillContentPreviewCapability{}
	rows, err = s.db.Query(`SELECT c.agent_id,c.observed_at,a.paused FROM skill_content_preview_capabilities c JOIN sync_agents a ON a.id=c.agent_id AND a.user_id=c.user_id WHERE c.user_id=? AND `+privateSkillAccount+` ORDER BY c.agent_id`, uid, uid)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c SkillContentPreviewCapability
		var paused bool
		if err = rows.Scan(&c.AgentID, &c.ObservedAt, &paused); err != nil {
			return nil, nil, err
		}
		c.Ready = !paused && c.ObservedAt > now()-skillContentPreviewReady
		caps = append(caps, c)
	}
	return list, caps, rows.Err()
}

// CreateSkillContentPreviewFor binds a new request to the owned pin the caller
// reviewed (ID and exact version), the pin's enrollment and the connector process
// that last negotiated. The revision, bundle, name, mode and scope come from
// storage, never from the caller. A repeated request key returns the original.
func (s *Store) CreateSkillContentPreviewFor(uid int64, in SkillContentPreviewCreate) (out SkillContentPreview, err error) {
	if !privateSkillRequestID.MatchString(in.RequestKey) || !contentobs.ValidID(in.TargetID) || !contentobs.ValidID(in.ExpectedTargetVersion) {
		return out, ErrSkillContentPreviewInvalid
	}
	err = s.withTx(func(tx *sql.Tx) error {
		if e := privateSkillOwner(tx, uid); e != nil {
			return e
		}
		var existing string
		switch e := tx.QueryRow(`SELECT id FROM skill_content_previews WHERE user_id=? AND request_key=?`, uid, in.RequestKey).Scan(&existing); {
		case e == nil:
			original, e := readPreview(tx, uid, existing)
			if e != nil {
				return e
			}
			if original.TargetID != in.TargetID || original.TargetVersion != in.ExpectedTargetVersion {
				return ErrSkillContentPreview
			}
			out, e = effectivePreview(tx, uid, original)
			return e
		case !errors.Is(e, sql.ErrNoRows):
			return e
		}
		pin, e := scanContentTarget(tx.QueryRow(`SELECT `+contentTargetCols+contentTargetJoin+` WHERE t.user_id=? AND t.id=?`, uid, in.TargetID))
		if e != nil {
			return e
		}
		if pin.Version != in.ExpectedTargetVersion {
			return ErrSkillContentTargetConflict
		}
		if e = contentAgent(tx, uid, pin.AgentID); e != nil {
			return e
		}
		agent, project, _, e := reportedPinScope(tx, uid, pin.EnvironmentID, pin.AgentID)
		if e != nil {
			return e
		}
		var instance string
		var observed int64
		switch e = tx.QueryRow(`SELECT instance,observed_at FROM skill_content_preview_capabilities WHERE agent_id=? AND user_id=?`, pin.AgentID, uid).Scan(&instance, &observed); {
		case errors.Is(e, sql.ErrNoRows):
			return ErrSkillContentPreview
		case e != nil:
			return e
		case observed < now()-skillContentPreviewReady:
			return ErrSkillContentPreview
		}
		var total int
		if e = tx.QueryRow(`SELECT count(*) FROM skill_content_previews WHERE user_id=?`, uid).Scan(&total); e != nil {
			return e
		}
		if total >= MaxSkillContentPreviews {
			return ErrSkillContentPreviewCapacity
		}
		pending, e := tx.Query(`SELECT data FROM skill_content_previews WHERE agent_id=? AND user_id=? AND state IN ('queued','capturing')`, pin.AgentID, uid)
		if e != nil {
			return e
		}
		busy := false
		for pending.Next() {
			var raw string
			var other SkillContentPreview
			if e = pending.Scan(&raw); e != nil {
				break
			}
			if e = json.Unmarshal([]byte(raw), &other); e != nil {
				break
			}
			busy = busy || activePreview(other)
		}
		if e == nil {
			e = pending.Err()
		}
		pending.Close()
		if e != nil {
			return e
		}
		if busy {
			return ErrSkillContentPreview
		}
		out = SkillContentPreview{
			ID: syncID(), RequestKey: in.RequestKey, TargetID: pin.ID, TargetVersion: pin.Version, EnvironmentID: pin.EnvironmentID, AgentID: pin.AgentID,
			Agent: agent, Project: project, Name: pin.Name, BundleID: pin.BundleID, RevisionID: pin.RevisionID, ManifestDigest: pin.ManifestDigest, Mode: pin.Mode,
			Instance: instance, State: "queued", Reasons: []string{}, CreatedAt: now(), UpdatedAt: now(), ExpiresAt: now() + skillContentPreviewTTL, Authority: "preview_only",
		}
		raw, e := json.Marshal(out)
		if e != nil {
			return e
		}
		_, e = tx.Exec(`INSERT INTO skill_content_previews(id,user_id,agent_id,request_key,state,data,created_at) VALUES(?,?,?,?,?,?,?)`, out.ID, uid, out.AgentID, out.RequestKey, out.State, string(raw), out.CreatedAt)
		return e
	})
	return
}

// ClaimSkillContentPreviewFor is the connector's negotiation and its poll in one:
// it records that this process can serve previews and hands out at most one
// request. It reads no files and answers with nothing at all to an idle process.
// Work whose fences have failed is retired here rather than handed out.
func (s *Store) ClaimSkillContentPreviewFor(uid int64, agent, instance string) (work *SkillContentPreviewWork, err error) {
	if !contentobs.ValidID(instance) {
		return nil, ErrSkillContentPreviewInvalid
	}
	err = s.withTx(func(tx *sql.Tx) error {
		if e := privateSkillOwner(tx, uid); e != nil {
			return e
		}
		if e := contentAgent(tx, uid, agent); e != nil {
			return e
		}
		if _, e := tx.Exec(`INSERT INTO skill_content_preview_capabilities(agent_id,user_id,instance,observed_at) VALUES(?,?,?,?) ON CONFLICT(agent_id) DO UPDATE SET instance=excluded.instance,observed_at=excluded.observed_at`, agent, uid, instance, now()); e != nil {
			return e
		}
		rows, e := tx.Query(`SELECT data FROM skill_content_previews WHERE agent_id=? AND user_id=? AND state IN ('queued','capturing') ORDER BY created_at,id`, agent, uid)
		if e != nil {
			return e
		}
		pending := []SkillContentPreview{}
		for rows.Next() {
			var raw string
			var p SkillContentPreview
			if e = rows.Scan(&raw); e != nil {
				break
			}
			if e = json.Unmarshal([]byte(raw), &p); e != nil {
				break
			}
			pending = append(pending, p)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return e
		}
		var running int
		if e = tx.QueryRow(`SELECT count(*) FROM sync_jobs WHERE agent_id=? AND user_id=? AND status='running'`, agent, uid).Scan(&running); e != nil {
			return e
		}
		for _, p := range pending {
			if p.ExpiresAt < now() {
				p.State, p.Error = "expired", "preview_expired"
				if e = writePreview(tx, uid, p); e != nil {
					return e
				}
				continue
			}
			reasons, e := previewFence(tx, uid, p)
			if e != nil {
				return e
			}
			if len(reasons) > 0 {
				p.State, p.Reasons = "superseded", unionReasons(p.Reasons, reasons)
				if e = writePreview(tx, uid, p); e != nil {
					return e
				}
				continue
			}
			if running > 0 {
				continue
			}
			if p.State == "queued" {
				// Bind the inventory identity at the moment capture begins: a legacy
				// scan can complete between creation and the connector's next poll, but
				// none can run inside the single cycle that captures and reports.
				revision, e := currentInventoryRevision(tx, uid, p.EnvironmentID, p.AgentID)
				if e != nil {
					return e
				}
				if revision <= 0 {
					continue
				}
				p.InventoryRevision, p.State = revision, "capturing"
				if e = writePreview(tx, uid, p); e != nil {
					return e
				}
			}
			work = &SkillContentPreviewWork{ID: p.ID, Instance: p.Instance, Agent: p.Agent, Project: p.Project, Name: p.Name, ExpiresAt: p.ExpiresAt}
			return nil
		}
		return nil
	})
	return
}

// RecordSkillContentCaptureFor accepts the connector's metadata report. Every
// fence is rechecked in the writing transaction, then the planner is run on
// trusted storage, this capture and the slot's ownership receipt, read in that
// same transaction. Without a receipt there is no baseline, so an existing tree is
// never treated as managed: it comes back as adoption_required.
func (s *Store) RecordSkillContentCaptureFor(uid int64, agent, id, instance string, capture skillpreview.Capture) error {
	hash, err := capture.Hash()
	if err != nil {
		return ErrSkillContentPreviewInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if e := privateSkillOwner(tx, uid); e != nil {
			return e
		}
		p, e := readPreview(tx, uid, id)
		if e != nil {
			return e
		}
		if p.AgentID != agent {
			return ErrSkillContentPreviewMissing
		}
		if e = contentAgent(tx, uid, agent); e != nil {
			return e
		}
		reasons, e := previewFence(tx, uid, p)
		if e != nil {
			return e
		}
		if len(reasons) > 0 || p.Instance != instance {
			return ErrSkillContentPreview
		}
		if p.State == "captured" {
			if p.CaptureHash == hash {
				return nil
			}
			return ErrSkillContentPreview
		}
		if p.State != "capturing" || p.ExpiresAt < now() || p.InventoryRevision <= 0 {
			return ErrSkillContentPreview
		}
		received := now()
		plan, receiptVersion, e := buildPreviewPlan(tx, uid, p, capture, received)
		if e != nil {
			// The inputs themselves cannot be planned (for example a name the planner
			// refuses, or a receipt that fails validation). That is this request's
			// terminal result, not something to retry.
			p.State, p.Error = "failed", "plan_unavailable"
			return writePreview(tx, uid, p)
		}
		p.Capture, p.CaptureHash, p.Plan, p.CapturedAt, p.State, p.ReceiptVersion = &capture, hash, &plan, received, "captured", receiptVersion
		p.ReviewHash = skillContentReviewIdentity{
			OwnerID: uid, RequestID: p.ID, TargetID: p.TargetID, TargetVersion: p.TargetVersion, BundleID: p.BundleID, RevisionID: p.RevisionID,
			ManifestDigest: p.ManifestDigest, Mode: p.Mode, AgentID: p.AgentID, Instance: p.Instance, InventoryRevision: p.InventoryRevision,
			CaptureHash: hash, PlanHash: plan.Hash, ReceiptVersion: receiptVersion,
		}.Hash()
		return writePreview(tx, uid, p)
	})
}

// buildPreviewPlan assembles the planner's snapshot only from storage and the
// validated capture. Enrollment generation is the connector's ID: enrollment rows
// are never reused, so re-enrolling a machine always yields a new one. The slot's
// receipt, when it has one, is the baseline and its version is returned for the
// preview to bind; having no receipt is what blocks adoption. A receipt that fails
// validation is an error, never an absent baseline, and whether a baseline still
// fits the target and bundle is the planner's decision. The caller holds the owner
// write lock, so the receipt cannot change before the plan is stored.
func buildPreviewPlan(tx *sql.Tx, uid int64, p SkillContentPreview, c skillpreview.Capture, received int64) (skillplan.Plan, string, error) {
	revision, err := privateSkillRevision(tx, uid, p.BundleID, p.RevisionID, false)
	if err != nil {
		return skillplan.Plan{}, "", err
	}
	if revision.Manifest == nil || revision.ManifestDigest != p.ManifestDigest || revision.Manifest.Digest != p.ManifestDigest {
		return skillplan.Plan{}, "", errors.New("pinned revision identity changed")
	}
	baseline, receiptVersion, err := skillContentReceiptBaseline(tx, uid, p.EnvironmentID, p.Name)
	if err != nil {
		return skillplan.Plan{}, "", err
	}
	plan, err := skillplan.Build(skillplan.Snapshot{
		InventoryRevision: p.InventoryRevision,
		Target:            skillplan.Target{OwnerID: uid, AgentID: p.AgentID, EnvironmentID: p.EnvironmentID, Generation: p.AgentID, RootID: c.RootID, SkillName: p.Name, Adapter: c.Adapter, Schema: skillplan.Version},
		Desired:           skillplan.Revision{OwnerID: uid, BundleID: p.BundleID, RevisionID: p.RevisionID, Name: p.Name, Manifest: *revision.Manifest},
		Observation:       skillplan.Observation{InventoryRevision: p.InventoryRevision, Generation: p.AgentID, Presence: c.Presence, Complete: c.Complete, ReceivedAt: received, Manifest: c.Manifest},
		Baseline:          baseline,
		RootOwnership:     c.RootOwnership,
		Mode:              p.Mode,
	}, received)
	if err != nil {
		return skillplan.Plan{}, "", err
	}
	return plan, receiptVersion, nil
}

// FailSkillContentPreviewFor records a bounded code, never raw local errors.
func (s *Store) FailSkillContentPreviewFor(uid int64, agent, id, instance, code string) error {
	switch code {
	case "capture_failed", "scope_unavailable":
	default:
		return ErrSkillContentPreviewInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if e := privateSkillOwner(tx, uid); e != nil {
			return e
		}
		p, e := readPreview(tx, uid, id)
		if e != nil {
			return e
		}
		if p.AgentID != agent || p.Instance != instance {
			return ErrSkillContentPreview
		}
		if !activePreview(p) {
			return nil
		}
		p.State, p.Error = "failed", code
		return writePreview(tx, uid, p)
	})
}

// StopSkillContentPreviewFor revokes outstanding work. A capture already recorded
// is a finished observation and is left as it is.
func (s *Store) StopSkillContentPreviewFor(uid int64, id string) error {
	return s.withTx(func(tx *sql.Tx) error {
		if e := privateSkillOwner(tx, uid); e != nil {
			return e
		}
		p, e := readPreview(tx, uid, id)
		if e != nil {
			return e
		}
		if p.State == "queued" || p.State == "capturing" {
			p.State = "cancelled"
			return writePreview(tx, uid, p)
		}
		return nil
	})
}

// DismissSkillContentPreviewFor cancels live work or removes finished history.
// It never touches the pin, the private revision or any machine file.
func (s *Store) DismissSkillContentPreviewFor(uid int64, id string) (out SkillContentPreview, err error) {
	err = s.withTx(func(tx *sql.Tx) error {
		if e := privateSkillOwner(tx, uid); e != nil {
			return e
		}
		p, e := readPreview(tx, uid, id)
		if e != nil {
			return e
		}
		if p.State == "queued" || p.State == "capturing" {
			p.State = "cancelled"
			out = p
			return writePreview(tx, uid, p)
		}
		out = p
		_, e = tx.Exec(`DELETE FROM skill_content_previews WHERE id=? AND user_id=?`, id, uid)
		return e
	})
	return
}
