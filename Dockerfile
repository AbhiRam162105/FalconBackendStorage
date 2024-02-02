FROM golang:1.21
WORKDIR /app

COPY . .

RUN go build -o go-app .

EXPOSE 8080

CMD ["./go-app"]
