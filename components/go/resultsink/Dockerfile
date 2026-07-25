FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY pkg ./pkg
COPY components/go ./components/go
RUN CGO_ENABLED=0 go build -o /out/resultsink ./components/go/resultsink

FROM gcr.io/distroless/static-debian12
COPY --from=builder /out/resultsink /resultsink
ENTRYPOINT ["/resultsink"]
