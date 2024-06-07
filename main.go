package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/lmittmann/tint"
	"github.com/urfave/cli/v2"

	dotenv "github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
)

func main() {
    app := &cli.App{
        Name: "ai-scan-classifier",
        Usage: "Classify the content of a scanned document using OpenAI's ChatGPT",
        Version: "0.1.0",
        ExitErrHandler: func(context *cli.Context, err error) {
            if err != nil {
                slog.Error("Error running app", "error", err)
                os.Exit(1)
            }
        },
        Flags: []cli.Flag{
            &cli.BoolFlag{
                Name: "verbose",
                Usage: "Enable verbose logging",
            },
            &cli.StringFlag{
                Name: "log-style",
                Usage: "The log style to use",
                Value: "plain",
                Action: func(c *cli.Context, s string) error {
                    switch c.String("log-style") {
                        case "json":
                            slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
                        case "plain":
                            slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{TimeFormat: time.DateTime})))
                        default:
                            slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{TimeFormat: time.DateTime})))
                    }
                    return nil
                },
            },
            &cli.BoolFlag{
                Name: "daemon",
                Usage: "Run the app as a daemon",
                Aliases: []string{"d"},
            },
        },
        Commands: []*cli.Command{
            {
                Name: "classify",
                Aliases: []string{"c"},
                Usage: "Classify the content of a scanned document",
                Flags: []cli.Flag{
                    &cli.StringFlag{
                        Name: "lang",
                        Aliases: []string{"l"},
                        Usage: "The language of the document",
                        Value: "deu",
                    },
                },
            },
        },
        Action: func(c *cli.Context) error {
            slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{TimeFormat: time.DateTime})))

            err := dotenv.Load()
            if err != nil && !os.IsNotExist(err) {
                slog.Warn("Error loading .env file", "error", err)
                return nil
            }

            // check for daemon flag
            if c.Bool("daemon") {
                slog.Info("Running as daemon")

                return watchFTP()
            }

            // if no arguments are provided and it's not the help command, return an errors
            if c.Args().Len() == 0 && !c.Bool("help") {
                return errors.New("No file provided")
            }

            classifyFile(c.Args().First())
            return nil
        },
        EnableBashCompletion: true,
    }


    app.Run(os.Args)
}

func classifyFile(file string) {
	// remove files from previous runs
	toRemove := []string{"/tmp/output.pdf", "/tmp/output.pdf.txt"}
	for _, file := range toRemove {
        err := os.Remove(file)
		if err != nil {
            slog.Warn(fmt.Sprintf("Error removing file: %s", file), "error", err)
		}
	}

    slog.Info("Processing file", "file", file)

	output, err := exec.Command("ocrmypdf", file, "--redo-ocr", "-l", "deu", "/tmp/output.pdf", "--sidecar").CombinedOutput()
	if err != nil {
        slog.Error("Error running ocrmypdf", "error", err, "output", string(output))
		return
	}

	// read the sidecar file
	ocr, err := os.ReadFile("/tmp/output.pdf.txt")
	if err != nil {
        slog.Error("Error reading sidecar file", "error", err)
		return
	}

    // only include the first 2000 characters
    ocr = ocr[:min(2000, len(ocr))]

    // chop up ocr into chunks of 2000 characters and convert to ChatCompletionMessage:
    var messages []openai.ChatCompletionMessage
    for i := 0; i < len(ocr); i += 2000 {
        messages = append(messages, openai.ChatCompletionMessage{
            Role:    openai.ChatMessageRoleUser,
            Content: string(ocr[i:min(i+2000, len(ocr))]),
        })
    }


	openaiKey := os.Getenv("OPENAI_KEY")

	prompt := `
	You will be provided with a the OCR version of a scanned document, and your
	task is to classify its content as one of the following categories. Give an explanation, a title and a category in JSON format.

    An example response would be:
    {"category": "ids", "explanation": "This is a scan of a German ID card (Personalausweis) for Max Mustermann.", title: "Perso Max"}

	- bizfactory: A document that is related to my work at Biz Factory GmbH
	- ids: A scan of an ID card, passport, or similar card
	- klausuren: A scan of an exam or similar
	- schule: A document that is related to my school education
	- sparkasse: A document that is related to my bank account at Sparkasse
    - deka: A document that is related to my investment at Deka
    - db: A document that is related to Deutsche Bahn
    - taxes: A document that is related to taxes
	- comdirect: A document that is related to my bank account at Comdirect
	- th-koeln: A document that is related to my studies at Technische Hochschule Köln
	- tk: A document that is related to my health insurance at TK (Techniker Krankenkasse)
    - gov: A document that is issued by a government or other official institution
    - hilde8: A document that is related to the apartment at Hildebrandtstraße 8
    - check24: A document that is related to my work at Check24
    - insurance: A document that is related to insurance
	- misc: A document that does not fit into any of the above categories

    If you feel that the document does not fit any of the above categories but fits well in a broader category, you may suggest one (only in one word).
	`

	client := openai.NewClient(openaiKey)
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT4,
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
        slog.Error("Error running OpenAI API", "error", err)
		return
	}

    var classification map[string]interface{}
    err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &classification)
    if err != nil {
        slog.Error("Error parsing OpenAI response", "error", err)
        return
    }

    slog.Info("Classification", "title", classification["title"], "category", classification["category"], "explanation", classification["explanation"])
    fmt.Println(classification["category"])
}

func watchFTP() error {
    host := os.Getenv("FTP_HOST")
    user := os.Getenv("FTP_USER")
    password := os.Getenv("FTP_PASSWORD")
    path := os.Getenv("FTP_PATH")

    if host == "" {
        return errors.New("FTP_HOST not set")
    }
    if user == "" {
        return errors.New("FTP_USER not set")
    }
    if password == "" {
        return errors.New("FTP_PASSWORD not set")
    }
    if path == "" {
        return errors.New("FTP_PATH not set")
    }

    slog.Info("Watching FTP", "host", host, "user", user, "path", path)

    c, err := ftp.Dial(fmt.Sprintf("%s:%d", host, 21), ftp.DialWithTimeout(5*time.Second))
    if err != nil {
        slog.Error("Error connecting to FTP", "error", err)
        return err
    }

    err = c.Login(user, password)
    if err != nil {
        slog.Error("Error logging in to FTP", "error", err)
        return err
    }

    // Store already known files
    knownFiles := make(map[string]bool)
    firstRun := true

    entries, err := c.List(path)
    if err != nil {
        slog.Error("Error listing FTP directory", "error", err)
        return err
    }

    for _, entry := range entries {
        if entry.Type == ftp.EntryTypeFile {
            knownFiles[entry.Name] = true
        }
    }

    slog.Info("Known files", "files", knownFiles)

    for {
        entries, err := c.List(path)
        if err != nil {
            slog.Error("Error listing FTP directory", "error", err)
            return err
        }

        for _, entry := range entries {
            if entry.Type == ftp.EntryTypeFile && !knownFiles[entry.Name] {
                slog.Info("New file", "file", entry.Name)
                fileName, err := downloadFile(c, fmt.Sprintf("%s/%s", path, entry.Name))
                if err != nil {
                    slog.Error("Error downloading file", "error", err)
                    continue
                }
                classifyFile(fileName)
            }
            knownFiles[entry.Name] = true
        }

        time.Sleep(5 * time.Second)

        if firstRun {
            firstRun = false
        }
    }
}

func downloadFile(c *ftp.ServerConn, path string) (string, error) {
    file, err := os.CreateTemp("", "ai-scan-classifier")
    if err != nil {
        slog.Error("Error creating file", "error", err)
        return "", err
    }
    defer file.Close()

    resp, err := c.Retr(path)
    if err != nil {
        slog.Error("Error downloading file", "error", err)
        return "", err
    }
    defer resp.Close()

    buf, err := io.ReadAll(resp)
    if err != nil {
        slog.Error("Error reading file", "error", err)
        return "", err
    }

    _, err = file.Write(buf)
    if err != nil {
        slog.Error("Error writing file", "error", err)
        return "", err
    }

    return file.Name(), nil
}

