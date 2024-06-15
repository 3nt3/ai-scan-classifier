FROM golang:1.22.3

WORKDIR /usr/src/ai-scan-classifier

COPY . .

RUN go build

CMD ["./ai-scan-classifier", "-d"]

