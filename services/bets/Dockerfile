FROM golang:1.26.0-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY services/bets ./services/bets
COPY contracts/generated ./contracts/generated
RUN CGO_ENABLED=0 go build -o /bets ./services/bets/cmd/server
FROM alpine:3.23
COPY --from=build /bets /bets
USER 65532:65532
EXPOSE 8083
ENTRYPOINT ["/bets"]
