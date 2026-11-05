# Eli provider core and HTTP adapters

`provider.go`, `contract_test.go` and `fake_test.go` are adapted from
[eyupio/zoomies](https://github.com/eyupio/zoomies/tree/b4c67a1413c8274334d18225df4053959861fbc0/internal/assistant),
revision `b4c67a1413c8274334d18225df4053959861fbc0`, originally package
`internal/assistant`. Zoomies and reet distribute under GNU AGPL version 3;
see the repository [licence](../../LICENSE). Original author attribution remains
in the linked source history; the upstream files contain no individual notices.

Changes in the original probe: renamed the package to `elicore`, restricted the conformance runner and
fake to tests, and removed the fake's Zoomies fleet demo branches. No provider
adapter, credential store, controller toolbox or browser widget was copied.

## HTTP provider foundation, 5 November 2026

The files under `provider/` adapt the OpenAI chat-completions and Anthropic
Messages adapters, shared HTTP/SSE/model-list code and their protocol tests from
[eyupio/zoomies at 3c4d09627f78afab72a6575e94624fb0e7710f3f](https://github.com/eyupio/zoomies/tree/3c4d09627f78afab72a6575e94624fb0e7710f3f/internal/assistant/provider).
`provider/testserver_test.go` adapts that revision's
`internal/assistant/assistanttest/servers.go`; the local conformance test adapts
the existing reet probe, originally from the pinned source above. All fake
servers and conformance helpers remain test-only. Individual adapted files carry
source notices; author attribution remains in the linked upstream history.

Modifications: reet `elicore` imports, Gaffer OpenCode session identifiers,
test-only fixture wiring, no subscription command configuration, redirects
refused at the adapter boundary, configured-key masking in provider errors,
stream/tool assembly bounds and cancellation before buffered tool delivery.
Additional regressions cover those behaviours. No SDK or other dependency is
added, and the AGPL licence remains unchanged.

The adapters have no production caller, provider credential store, route or UI
yet. They do not grant session access or execution authority. The remaining
provider-configuration, permitted-network/data-access and coordinator lifecycle
contracts must be implemented before enabling a real Gaffer chat path.
