FROM golang:1.20-alpine

ENV DISCORD_TOKEN=""
ENV OPENAI_TOKEN=""

WORKDIR /app

COPY . .

RUN go mod download

RUN go build -o /go-remai-bot-discord

COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
