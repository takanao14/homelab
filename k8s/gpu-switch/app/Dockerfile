FROM --platform=$BUILDPLATFORM golang:1.26.5 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY web ./web

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/gpu-switch .

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /out/gpu-switch /gpu-switch

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/gpu-switch"]
