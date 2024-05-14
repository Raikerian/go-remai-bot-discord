FROM golang:1.22.3-alpine

WORKDIR /app

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o /go-remai-bot-discord

CMD [ "/go-remai-bot-discord" ]
