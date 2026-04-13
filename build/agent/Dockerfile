FROM golang:1.21 AS builder
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -o agent ./cmd/agent/

FROM openeuler/openeuler:22.03-lts
# Agent needs access to host process info; etmem binary is mounted from host.
RUN yum install -y procps-ng && yum clean all
COPY --from=builder /workspace/agent /usr/local/bin/agent
ENTRYPOINT ["/usr/local/bin/agent"]
