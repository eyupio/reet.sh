# Test binary only; this fixture is never published as the production image.
ARG REET_RUNTIME_BASE=gcr.io/distroless/static-debian12:nonroot
FROM ${REET_RUNTIME_BASE}
COPY --chown=65532:65532 gaffer.test /gaffer.test
COPY --chown=65532:65532 web/ /web/
ENV TMPDIR=/data REET_GAFFER_TEST_WEB=/web
USER 65532:65532
ENTRYPOINT ["/gaffer.test", "-test.run", "^TestGafferOfflineBrowserHarness$", "-test.timeout=10m", "-test.v"]
