## Pre-built binary Dockerfile (no Go required, no network required for build stage)
## Usage: cp dist/agent bin/agent && nerdctl build -t etmem-agent:v0.4.0-pid -f build/agent/Dockerfile .
FROM openeuler/openeuler:22.03-lts
RUN yum install -y procps-ng && yum clean all
COPY bin/agent /usr/local/bin/agent
ENTRYPOINT ["/usr/local/bin/agent"]
