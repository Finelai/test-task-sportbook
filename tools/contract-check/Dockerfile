FROM golang:1.26.0-alpine AS build
WORKDIR /src
COPY tools/contract-check/main.go ./main.go
RUN CGO_ENABLED=0 go build -o /contract-check ./main.go

FROM alpine:3.23
COPY --from=build /contract-check /contract-check
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/contract-check"]
