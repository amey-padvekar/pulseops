# Multi-stage build for the PulseOps backend (Cloud Run).
# Build context is the backend/ directory (go.mod at its root).
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Static binary so it runs on a distroless base.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
# Cloud Run sends traffic to $PORT (default 8080); the server honors PORT.
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
