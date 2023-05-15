FROM golang:1.20 as builder

WORKDIR /workspace
# install grpc health probe
ENV GRPC_HEALTH_PROBE_VERSION=v0.3.2
RUN wget -qO/bin/grpc_health_probe https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-amd64 && chmod +x /bin/grpc_health_probe
COPY go.mod .
COPY go.sum .
# configure git with secrets
RUN --mount=type=secret,id=GIT_PAT GIT_PAT=$(cat /run/secrets/GIT_PAT) && git config --global url."https://$GIT_PAT@github.com".insteadOf "https://github.com"
# download dependencies
RUN go mod download
# copy source code
COPY . .
# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -race

# Use distroless as minimal base image
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/semver-tags .
COPY --from=builder /bin/grpc_health_probe ./grpc_health_probe
USER 65532:65532

ENTRYPOINT ["/semver-tags", "run"]
