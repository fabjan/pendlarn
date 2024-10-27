FROM golang:1.20 AS build
WORKDIR /tree
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo .

FROM alpine:3.17
RUN apk add --no-cache tzdata
WORKDIR /app
COPY --from=build /tree/pendlarn .

ENV TZ="Europe/Stockholm"
ENV PORT="3000"
CMD ["./pendlarn"]
