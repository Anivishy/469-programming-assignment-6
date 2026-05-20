FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY bank/ bank/
COPY server/ server/
RUN CGO_ENABLED=0 go build -o /bank-server ./server

FROM alpine:3.20
COPY --from=builder /bank-server /bank-server
ENTRYPOINT ["/bank-server"]
