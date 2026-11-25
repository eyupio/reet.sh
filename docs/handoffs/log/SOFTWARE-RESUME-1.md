## SOFTWARE-RESUME-1: a half-finished agent software download is continued (25 November 2026)

The first half of the roadmap's "Mirror resilience" item (the second half, letting an account prefer the controller's mirror over
the registry, is separate).

**What changed.** `StageTarballFrom` (`internal/agentsoftware/artifact.go`) used to delete a download that stopped part-way and
start from nothing, so a ~79 MB archive on a poor connection could never finish, and the registry-then-mirror order began the
mirror's download again from byte zero. It now downloads into a file named for the archive's checksum
(`.opencode-<hash>.part` in the staging directory), keeps it when the download fails, and the next attempt, from any source,
sends `Range: bytes=<have>-`. The whole archive is then checked against its recorded sha512 exactly as before.

**Safe by construction.** The end checksum decides everything, so a half made of the wrong bytes, or pieces from two sources that
do not belong together, fails the check and the file is deleted (`ErrArtifactChecksum`), never continued. A server that ignores
the range and sends a whole 200, a 206 that does not start where asked, and a 416 each start the file again. A half larger
than the archive or the 300 MiB cap, or older than a day (`partialMaxAge`), is started again. A finished or rejected archive
leaves no `.part` file. The controller's mirror already served through `http.ServeContent`, which answers ranges, so it needed no
change.

**Across applies (found in review).** The first version kept the half only within one apply: `ApplySoftware` deleted its whole staging
directory on the way out, so the next apply began again from zero. It now removes every temporary file except a `.opencode-*.part`
(`clearStaging`), and `PrunePartials` deletes halves older than a day at the start of an apply so one that is never tried again
does not sit on disk. `TestApplySoftwareContinuesADownloadAcrossApplies` drives it through `ApplySoftware`: the first apply fails
as a download, leaves the half, and the second continues it with a Range request and leaves nothing behind; it fails with the old
`RemoveAll`.

**Tests** (`artifact_resume_test.go`, `-race -count=5`): an interrupted download continued with a Range request, the rest taken
from a different server, a server that ignores ranges, a wrong `Content-Range` and a refused range, a bad file deleted, a stale
file not trusted. The first two fail on the old code (it kept nothing).

shortcut: one download of an artifact at a time, which the software lock already makes true; no `If-Range`, because the checksum
is the guarantee.

Not checked: a real registry or a real flaky network (the cut-off is a server that drops the connection); that the registry
honours ranges for this package (npm's CDN does for tarballs, but I did not test it).
