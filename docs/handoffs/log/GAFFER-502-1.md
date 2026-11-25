## GAFFER-502-1: a Gaffer provider failure keeps its sentence behind a proxy (25 November 2026)

Asking Gaffer for a plan on a controller behind a proxy showed a bare "502" and nothing else. Every provider failure
in Gaffer (`gaffer.ProbeError`: the provider refused, the model returned no plan, the answer could not be read) was
answered as 502 with a sentence saying what happened. A proxy in front of reet replaces the body of a 502 from the
origin with its own page, so the browser got no JSON and `req` in `web/src/lib/api.ts` fell back to the status.

What changed. `gafferError` in `internal/api/gaffer_settings.go` answers a provider failure as 500 with the same
sentence. It is still a 5xx, so `mayHaveArrived` in `web/src/lib/factory/delivery.ts` treats it as before. This covers
every Gaffer route that goes through `gafferError` (plan, reply, report, chat, model list).

Test. `TestAProviderFailureIsNotAnsweredAs502SoAProxyKeepsItsSentence` in `internal/api/gaffer_plan_test.go`.

Not done, and not checked.

- The failure was not reproduced on the proxied controller: it needs a sign-in. That the proxy rewrites a 502 is its
  documented behaviour, not something measured here. If the dialog still shows a bare "502" after this is deployed,
  the answer is coming from the proxy itself (the controller did not answer), which is a different fault.
- Why the provider failed for that plan is still unknown. After this change the dialog says.
- The machine routes that answer 502 with a code (`machine_error`, `upload_failed`, `software_download_failed` and
  others in `internal/api`) lose their body behind such a proxy in the same way. They are documented as 502 in several
  docs and were left alone.
