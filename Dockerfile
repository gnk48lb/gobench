FROM golang:1.22-alpine AS builder
ARG CMD_PATH=cmd/api-service
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./${CMD_PATH}/

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata bash
WORKDIR /app
COPY --from=builder /server /server
COPY config/ /app/config/
ENTRYPOINT ["/server"]
