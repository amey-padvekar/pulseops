FROM golang:1.23 AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12

WORKDIR /
COPY --from=builder /out/server /server

EXPOSE 8080
ENTRYPOINT ["/server"]