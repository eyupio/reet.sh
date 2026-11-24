- **Revoking a personal token does not end a machine's private connection**: a machine enrolled with a personal API token
  (every machine enrolled before enrolment codes) stops working on the controller's public address when that token is
  revoked, but its private control and terminal connection checks only the machine's own id and secret. To end it,
  remove the machine on the Machines page, or run `reet agent enroll --again` on it with a code. `SECURITY.md` says so.
