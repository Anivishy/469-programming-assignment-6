FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY bank/ bank/
COPY tester/ tester/
RUN CGO_ENABLED=0 go build -o /tester ./tester

FROM alpine:3.20
COPY --from=builder /tester /tester
ENTRYPOINT ["/tester"]
