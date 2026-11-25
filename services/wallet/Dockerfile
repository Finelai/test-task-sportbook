FROM golang:1.26.0-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY services/wallet ./services/wallet
COPY contracts/generated/wallet ./contracts/generated/wallet
RUN CGO_ENABLED=0 go build -o /wallet ./services/wallet/cmd/server
FROM alpine:3.23
COPY --from=build /wallet /wallet
USER 65532:65532
EXPOSE 8082 9093
ENTRYPOINT ["/wallet"]
