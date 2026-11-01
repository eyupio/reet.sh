# Eli provider compatibility spike

`provider.go`, `contract_test.go` and `fake_test.go` are adapted from
[eyupio/zoomies](https://github.com/eyupio/zoomies/tree/b4c67a1413c8274334d18225df4053959861fbc0/internal/assistant),
revision `b4c67a1413c8274334d18225df4053959861fbc0`, originally package
`internal/assistant`. Zoomies and reet distribute under GNU AGPL version 3;
see the repository [licence](../../LICENSE). Original author attribution remains
in the linked source history; the upstream files contain no individual notices.

Changes: renamed the package to `elicore`, restricted the conformance runner and
fake to tests, and removed the fake's Zoomies fleet demo branches. No provider
adapter, credential store, controller toolbox or browser widget was copied.

This is the KZ5A probe, not an enabled coordinator. It has no runtime caller,
network adapter, provider credential, route or UI. Preserve the upstream streaming,
usage, cancellation and tool-call contract rather than inventing another one.
