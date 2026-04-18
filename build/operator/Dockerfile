## Pre-built binary Dockerfile (no Go required, no network required for build stage)
## Usage: cp dist/operator bin/operator && nerdctl build -t etmem-operator:v0.4.0-pid -f build/operator/Dockerfile .
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY bin/operator .
USER 65532:65532
ENTRYPOINT ["/operator"]
