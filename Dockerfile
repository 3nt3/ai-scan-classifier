FROM golang:1.22.3

WORKDIR /usr/src/ai-scan-classifier

COPY . .

RUN apt update && \
    apt install -y --no-install-recommends \
    ocrmypdf \
    tesseract-ocr-deu

RUN go build

CMD ["./ai-scan-classifier", "-d"]

