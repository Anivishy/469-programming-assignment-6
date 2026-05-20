FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY bank/ bank/
COPY client/ client/
RUN CGO_ENABLED=0 go build -o /bank-client ./client

FROM alpine:3.20
COPY --from=builder /bank-client /bank-client
ENTRYPOINT ["/bank-client"]
