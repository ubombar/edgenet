FROM golang:1.18.3-alpine AS build

WORKDIR /edgenet

COPY go.mod .
RUN go mod download

COPY . ./
ENV CGO_ENABLED=0
RUN go build -o nodecontribution ./cmd/nodecontribution/

FROM alpine:3.16.2

RUN adduser -D -u 8118 edgenet
USER edgenet:edgenet

WORKDIR /edgenet/nodecontribution/
COPY --from=build --chown=edgenet:edgenet /edgenet/nodecontribution ./

CMD ["./nodecontribution"]
