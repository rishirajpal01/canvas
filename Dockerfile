FROM golang:alpine as build

WORKDIR /usr/src/rplace

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN go build -v -o /usr/local/bin/rplace


FROM ubuntu

EXPOSE 8080
COPY --from=build /usr/local/bin/rplace /usr/local/bin/rplace

ENTRYPOINT [ "rplace" ]