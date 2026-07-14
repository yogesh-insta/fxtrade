# syntax=docker/dockerfile:1

FROM golang:1.25 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=builder /out/server /server
ENV PORT=8080
ENV DB_PATH=/tmp/btc-sentiment.db
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
