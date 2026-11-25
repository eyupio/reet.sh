- **`reet agent set-server` can no longer be fooled by a server that relays**: a machine used to accept any server that
  could get a proof from the real controller, including one that just passed the machine's request on. The proof is now
  signed over the controller's own public address, and the machine accepts only that. The controller you point a machine
  at must be this release or newer and have its public URL set (`REET_PUBLIC_URL`, or the admin settings), and you must
  use that address; otherwise the machine says why and sends nothing.
