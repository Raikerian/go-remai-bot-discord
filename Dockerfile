FROM golang:1.20-alpine

ENV DISCORD_TOKEN=""
ENV OPENAI_TOKEN=""

WORKDIR /app

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o /go-remai-bot-discord

COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
