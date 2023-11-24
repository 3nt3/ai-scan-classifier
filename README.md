# AI-Scan-Classifier

![AI-Scan-Classifier Logo](https://your-image-url.com/logo.png)

## Overview

AI-Scan-Classifier is a Go project for classifying PDF documents using OCR. It integrates with `ocrmypdf` for text extraction and leverages ChatGPT for document classification.

## Features

- **PDF Classification**: Automatically classify PDF documents.
- **OCR Text Extraction**: Use `ocrmypdf` for accurate text extraction.
- **ChatGPT Integration**: Analyze and classify text content.

## Getting Started

1. Install Go, Tesseract OCR, and ocrmypdf.
2. Clone the repo and build:

   ```bash
   git clone https://github.com/3nt3/AI-Scan-Classifier.git
   cd AI-Scan-Classifier
   go build
   ```

3. Run the classifier:

   ```bash
   OPENAI_KEY=your_chatgpt_api_key ./AI-Scan-Classifier input.pdf
   ```

   Replace `input.pdf` with the path to your input PDF.

## Configuration

Configure AI-Scan-Classifier using environment variables or a `.env` file.

```env
OPENAI_KEY=your_chatgpt_api_key
```

## Contribution

Contributions are welcome! Open issues or submit pull requests.
