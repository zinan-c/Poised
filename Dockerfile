FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
RUN go build -o /out/poised ./cmd/poised

FROM alpine:3.22

WORKDIR /app
COPY --from=build /out/poised /usr/local/bin/poised
COPY configs ./configs
COPY web ./web

EXPOSE 8080
ENTRYPOINT ["poised"]
CMD ["-config", "configs/poised.example.json"]
