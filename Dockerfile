FROM golang:1.27-alpine

WORKDIR /app

COPY . .

RUN apk add --no-cache git build-base && cd /app && (go mod download || true) && if [ -d /app/cmd/server ]; then go mod tidy && go build -o /app/bin/server ./cmd/server; else echo 'WARNING: /app/cmd/server not found; only go.mod present, skipping server build'; fi

EXPOSE 8080

CMD ["sh", "-c", "/app/bin/server"]
