package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	dotenv "github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
)

func main() {
	err := dotenv.Load()
	if err != nil {
		fmt.Printf("Error loading .env file: %v\n", err)
		return
	}

	// remove files from previous runs
	toRemove := []string{"/tmp/output.pdf", "/tmp/output.pdf.txt"}
	for _, file := range toRemove {
		err = os.Remove(file)
		if err != nil {
			fmt.Printf("Error removing file %s: %v\n", file, err)
		}
	}


	// run ocrmypdf command on the first argument
	output, err := exec.Command("ocrmypdf", os.Args[1], "-l", "deu", "/tmp/output.pdf", "--sidecar", ).CombinedOutput()
	if err != nil {
		fmt.Printf("Error running ocrmypdf: %v\n%v\n", err, string(output))
		return
	}

	// read the sidecar file
	ocr, err := os.ReadFile("/tmp/output.pdf.txt")
	if err != nil {
		fmt.Printf("Error reading sidecar file: %v\n", err)
		return
	}


	openaiKey := os.Getenv("OPENAI_KEY")

	prompt := `
	You will be provided with a the OCR version of a scanned document, and your
	task is to classify its content as one of the following categories. You may
	only respond with one word.


	- bizfactory: A document that is related to my work at Biz Factory GmbH
	- ids: A scan of an ID card, passport, or similar
	- klausuren: A scan of an exam or similar
	- schule: A document that is related to my school education
	- sparkasse: A document that is related to my bank account at Sparkasse
	- comdirect: A document that is related to my bank account at Comdirect
	- th-koeln: A document that is related to my studies at Technische Hochschule Köln
	- tk: A document that is related to my health insurance at TK (Techniker Krankenkasse)
	- misc: A document that does not fit into any of the above categories
	`

	client := openai.NewClient(openaiKey)
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: string(ocr),
				},
			},
		},
	)

	if err != nil {
		fmt.Printf("ChatCompletion error: %v\n", err)
		return
	}

	fmt.Println(resp.Choices[0].Message.Content)
}
