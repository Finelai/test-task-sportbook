FROM golang:1.26.0-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY contracts/generated ./contracts/generated
COPY tools/contract-check-grpc ./tools/contract-check-grpc
RUN CGO_ENABLED=0 go build -o /contract-check-grpc ./tools/contract-check-grpc

FROM alpine:3.23
COPY --from=build /contract-check-grpc /contract-check-grpc
USER 65532:65532
EXPOSE 9091
ENTRYPOINT ["/contract-check-grpc"]
