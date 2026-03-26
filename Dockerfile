FROM golang:1.24 AS builder
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X github.com/ppiankov/mongopulse/internal/cli.version=${VERSION}" -o /mongopulse ./cmd/mongopulse

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /mongopulse /mongopulse
ENTRYPOINT ["/mongopulse"]
CMD ["serve"]
