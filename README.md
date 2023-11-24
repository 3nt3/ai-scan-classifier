# ai-scan-classifier

![the image](/image.jpg)

## Overview

ai-scan-classifier is a Go project for classifying PDF documents using OCR. It integrates with `ocrmypdf` for text extraction and leverages ChatGPT for document classification.

## Features

- **PDF Classification**: Automatically classify PDF documents.
- **OCR Text Extraction**: Use `ocrmypdf` for accurate text extraction.
- **ChatGPT Integration**: Analyze and classify text content.

## Getting Started

1. Install Go, Tesseract OCR, and ocrmypdf.
2. Clone the repo and build:

   ```bash
   git clone https://github.com/3nt3/ai-scan-classifier.git
   cd ai-scan-classifier
   go build
   ```

3. Run the classifier:

   ```bash
   OPENAI_KEY=your_chatgpt_api_key ./ai-scan-classifier input.pdf
   ```

   Replace `input.pdf` with the path to your input PDF.

## Configuration

Configure ai-scan-classifier using environment variables or a `.env` file.

```env
OPENAI_KEY=your_chatgpt_api_key
```

## Contribution

Contributions are welcome! Open issues or submit pull requests.
