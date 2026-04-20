FROM golang:1.22 as builder

WORKDIR /workspace

COPY go.mod go.sum ./
COPY api/ api/
COPY controllers/ controllers/
COPY cmd/ cmd/

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager ./cmd/main.go

FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/manager .

USER 65532:65532

ENTRYPOINT ["/manager"]
