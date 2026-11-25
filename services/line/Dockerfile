FROM golang:1.26.0-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY services/line ./services/line
RUN CGO_ENABLED=0 go build -o /line ./services/line/cmd/server
FROM alpine:3.23
COPY --from=build /line /line
USER 65532:65532
EXPOSE 8081
ENTRYPOINT ["/line"]
