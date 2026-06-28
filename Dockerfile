FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mcqq-bridge ./cmd/mcqq-bridge

FROM alpine:3.20

RUN adduser -D -h /app -u 10001 mcqq
WORKDIR /app
COPY --from=builder /out/mcqq-bridge /app/mcqq-bridge
RUN mkdir -p /app/data /app/logs /app/packs /app/napcat/config && chown -R mcqq:mcqq /app

EXPOSE 8080
ENTRYPOINT ["/app/mcqq-bridge"]
CMD ["start"]
