package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/softwaremirror"
)

func mirrorTestArtifact(t *testing.T) (agentsoftware.Artifact, []byte) {
	t.Helper()
	binary := []byte("reviewed binary")
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gzipWriter)
	if err := writer.WriteHeader(&tar.Header{Name: "package/bin/opencode", Mode: 0o755, Size: int64(len(binary)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	archive := buffer.Bytes()
	sum := sha512.Sum512(archive)
	archiveSum := sha256.Sum256(archive)
	binarySum := sha256.Sum256(binary)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	t.Cleanup(server.Close)
	return agentsoftware.Artifact{
		URL:           server.URL + "/opencode.tgz",
		ArchiveSHA256: hex.EncodeToString(archiveSum[:]),
		ArchiveSize:   int64(len(archive)),
		SHA512:        base64.StdEncoding.EncodeToString(sum[:]),
		BinarySHA256:  hex.EncodeToString(binarySum[:]),
		BinarySize:    int64(len(binary)),
		Member:        "package/bin/opencode",
	}, archive
}

func TestSoftwareMirrorSettingsAndCache(t *testing.T) {
	srv, st, _, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	artifact, archive := mirrorTestArtifact(t)
	srv.s.mirror = softwaremirror.New(t.TempDir(), artifact)

	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
		if method != http.MethodGet {
			r.Header.Set("Content-Type", "application/json")
		}
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}

	w := request(http.MethodGet, "/api/v2/settings/software-mirror", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	var view softwareMirrorView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Enabled || view.Cached || view.Version != agentsoftware.OpenCodeReviewedVersion {
		t.Fatalf("fresh mirror setting: %+v", view)
	}

	put := request(http.MethodPut, "/api/v2/settings/software-mirror", `{"enabled":true}`)
	if put.Code != http.StatusOK {
		t.Fatalf("put: %d %s", put.Code, put.Body.String())
	}
	if err := json.Unmarshal(put.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Enabled || view.ChangedBy == "" || view.ChangedAt == 0 {
		t.Fatalf("enabled setting: %+v body=%s", view, put.Body.String())
	}

	cache := request(http.MethodPost, "/api/v2/settings/software-mirror/cache", "")
	if cache.Code != http.StatusOK {
		t.Fatalf("cache: %d %s", cache.Code, cache.Body.String())
	}
	if err := json.Unmarshal(cache.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Cached || view.Size != int64(len(archive)) || view.Digest != artifact.ArchiveSHA256 {
		t.Fatalf("cached setting: %+v", view)
	}

	events, err := st.ListEventsFor(uid, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.Action] = true
	}
	if !seen["software-mirror-on"] || !seen["software-mirror-cache"] {
		t.Fatalf("audit: %v", seen)
	}
}

func TestPrivateSoftwareArtifactServesOnlyThePinnedDigest(t *testing.T) {
	srv, st, agent, secret, _ := privateFixture(t, AuthAccounts, "")
	artifact, archive := mirrorTestArtifact(t)
	srv.s.mirror = softwaremirror.New(t.TempDir(), artifact)
	if err := st.SetSetting(softwareMirrorKey, "1"); err != nil {
		t.Fatal(err)
	}
	handler := srv.s.privateHandler()
	path := "/private/v1/software/artifact/" + artifact.ArchiveSHA256

	rec := privateRequest(t, handler, http.MethodGet, path, "", agent.ID, secret, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("artifact: %d %s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), archive) {
		t.Fatalf("served %d bytes, want the archive", rec.Body.Len())
	}
	if rec.Header().Get("Content-Length") != "" && rec.Header().Get("Content-Length") != strconv.Itoa(len(archive)) {
		t.Fatalf("content length = %s", rec.Header().Get("Content-Length"))
	}

	if rec := privateRequest(t, handler, http.MethodGet, "/private/v1/software/artifact/not-the-digest", "", agent.ID, secret, true); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown digest: %d", rec.Code)
	}
	if rec := privateRequest(t, handler, http.MethodGet, path, "", agent.ID, "wrong-secret", true); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong credential: %d", rec.Code)
	}
	if err := st.SetSetting(softwareMirrorKey, "0"); err != nil {
		t.Fatal(err)
	}
	if rec := privateRequest(t, handler, http.MethodGet, path, "", agent.ID, secret, true); rec.Code != http.StatusNotFound {
		t.Fatalf("disabled mirror: %d", rec.Code)
	}
}
