package main

import (
	"3nt3/ai-scan-classifier/storage"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"time"

	"bytes"

	"github.com/jlaffaye/ftp"
	"github.com/lmittmann/tint"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/spf13/viper"
	"github.com/urfave/cli/v2"

	dotenv "github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
)

func main() {
	var programLevel = new(slog.LevelVar) // Info by default

	app := &cli.App{
		Name:    "ai-scan-classifier",
		Usage:   "Classify the content of a scanned document using OpenAI's ChatGPT",
		Version: "0.1.0",
		ExitErrHandler: func(context *cli.Context, err error) {
			if err != nil {
				slog.Error("Error running app", "error", err)
				os.Exit(1)
			}
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "Enable verbose logging",
			},
			&cli.StringFlag{
				Name:  "log-style",
				Usage: "The log style to use",
				Value: "plain",
			},
			&cli.StringFlag{
				Name:  "log-level",
				Usage: "The log level to use",
				Value: "info",
			},
			&cli.BoolFlag{
				Name:    "daemon",
				Usage:   "Run the app as a daemon",
				Aliases: []string{"d"},
			},
		},
		Commands: []*cli.Command{
			{
				Name:    "classify",
				Aliases: []string{"c"},
				Usage:   "Classify the content of a scanned document",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "lang",
						Aliases: []string{"l"},
						Usage:   "The language of the document",
						Value:   "deu",
					},
				},
			},
		},
		Action: func(c *cli.Context) error {
			// slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{TimeFormat: time.DateTime})))

			// adjust log level
			mappings := map[string]slog.Level{
				"debug": slog.LevelDebug,
				"info":  slog.LevelInfo,
				"warn":  slog.LevelWarn,
				"error": slog.LevelError,
			}
			level, ok := mappings[c.String("log-level")]
			if !ok {
				level = slog.LevelInfo
			}
			programLevel.Set(level)

			// set log log-style
			switch c.String("log-style") {
			case "json":
				slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: programLevel})))
			case "plain":
				slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{TimeFormat: time.DateTime, Level: programLevel})))
			default:
				slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{TimeFormat: time.DateTime, Level: programLevel})))
			}

			slog.Debug("Log level", "level", programLevel.String())

			err := dotenv.Load()
			if err != nil && !os.IsNotExist(err) {
				slog.Warn("Error loading .env file", "error", err)
				return nil
			}

			// check for daemon flag
			if c.Bool("daemon") {
				slog.Info("Running as daemon")

				go storage.RunServer()

				return daemon()
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

func classifyFile(file string) (storage.Classification, error) {
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
		return storage.Classification{}, err
	}

	// read the sidecar file
	ocr, err := os.ReadFile("/tmp/output.pdf.txt")
	if err != nil {
		slog.Error("Error reading sidecar file", "error", err)
		return storage.Classification{}, err
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
	task is to classify its content as one of the following categories. Give an explanation, a title, a filename, and a category in JSON format.

    An example response would be:
    {"category": "tk", "explanation": "This is a scan of a letter by TK (Techniker Krankenkasse), issuing an SMS-Tan reset code", title: "SMS-TAN Wiederherstellungscode", "filename": "sms_tan_reset_code.pdf"}

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
    - hildebrandtstraße: A document that is related to the apartment at Hildebrandtstraße 8
    - check24: A document that is related to my work at Check24
    - insurance: A document that is related to insurance
	- misc: A document that does not fit into any of the above categories
    - rheinbahn: A document that is related to Rheinbahn
    - hs-bochum: A document that is related to my studies at Hochschule Bochum

    If you feel that the document does not fit any of the above categories but fits well in a broader category, you may suggest one (only in one word). Only do so as a last resort.
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
		return storage.Classification{}, err
	}

	var classification storage.Classification
	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &classification)
	if err != nil {
		slog.Error("Error parsing OpenAI response", "error", err)
		return storage.Classification{}, err
	}

	slog.Info("Classification", "title", classification.Title, "category", classification.Category, "explanation", classification.Explanation)
	return classification, nil
}

func daemon() error {
	viper.SetConfigName("daemon")
	viper.SetConfigType("yml")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()
	if err != nil {
		slog.Error("Error reading config file", "error", err)
		return err
	}

	if !viper.IsSet("ftp.host") {
		slog.Error("FTP host not set")
		return errors.New("FTP host not set")
	}

	if !viper.IsSet("ftp.username") {
		slog.Error("FTP username not set")
		return errors.New("FTP username not set")
	}

	if !viper.IsSet("ftp.password") {
		slog.Error("FTP password not set")
		return errors.New("FTP password not set")
	}

	if !viper.IsSet("ftp.path") {
		slog.Error("FTP path not set")
		return errors.New("FTP path not set")
	}

	host := viper.GetString("ftp.host")
	user := viper.GetString("ftp.username")
	password := viper.GetString("ftp.password")
	path := viper.GetString("ftp.path")

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

	// Store already known files and their size
	knownFiles := make(map[string]map[string]bool)
	firstRun := true

	_, err = c.List(path)
	if err != nil {
		slog.Error("Error listing FTP directory", "error", err)
		return err
	}

	slog.Info("Known files", "files", knownFiles)

	for {
		entries, err := c.List(path)
		if err != nil {
			slog.Error("Error listing FTP directory", "error", err)
			return err
		}

		slog.Debug("Known files", "files", knownFiles)

		for _, entry := range entries {
			if entry.Type != ftp.EntryTypeFolder {
				slog.Warn("Your FTP directory should only contain folders, check your printer configuration", "file", entry.Name)
				continue
			}

			go processUserFolder(c, path, entry.Name, knownFiles[entry.Name])
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

func sendTelegramMessage(user string, message string) error {
	if !viper.IsSet("telegram_token") {
		return errors.New("Telegram token not set")
	}

	token := viper.GetString("telegram_token")

	// get username from config
	if !viper.IsSet(fmt.Sprintf("users.%s.telegram", user)) {
		return errors.New("Telegram user not set")
	}

	telegramUser := viper.GetString(fmt.Sprintf("users.%s.telegram", user))

	bot, err := telego.NewBot(token, telego.WithDefaultDebugLogger())
	if err != nil {
		slog.Error("Error creating Telegram bot", "error", err)
		return err
	}

	msg, err := bot.SendMessage(tu.Message(tu.Username(telegramUser), message).WithParseMode(telego.ModeHTML))
	if err != nil {
		slog.Error("Error sending Telegram message", "error", err)
		return err
	}

	slog.Info("Sent Telegram message", "message", msg)

	return nil
}

func uploadFileToNextcloud(user string, classification storage.Classification, localFilePath string) (string, error) {
	// Open local file
	file, err := os.Open(localFilePath)
	if err != nil {
		slog.Error("Error opening local file", "error", err)
		return "", err
	}
	defer file.Close()

	// Read the file contents into a byte slice
	fileContents, err := io.ReadAll(file)
	if err != nil {
		slog.Error("Error reading local file", "error", err)
		return "", err
	}

	// Get the Nextcloud credentials from the environment
	nextcloudURL := viper.GetString(fmt.Sprintf("%s.nextcloud.url", user))
	username := viper.GetString(fmt.Sprintf("%s.nextcloud.username", user))
	password := viper.GetString(fmt.Sprintf("%s.nextcloud.password", user))

	if !viper.IsSet(fmt.Sprintf("%s.nextcloud.url", user)) {
		slog.Error("Nextcloud URL not set", "user", user)
		return "", errors.New("Nextcloud URL not set")
	}

	if !viper.IsSet(fmt.Sprintf("%s.nextcloud.username", user)) {
		slog.Error("Nextcloud username not set", "user", user)
		return "", errors.New("Nextcloud username not set")
	}

	if !viper.IsSet(fmt.Sprintf("%s.nextcloud.password", user)) {
		slog.Error("Nextcloud password not set", "user", user)
		return "", errors.New("Nextcloud password not set")
	}

	now := time.Now()
	newFileName := fmt.Sprintf("%s_%s", now.Format("2006-01-02"), classification.FileName)

	remotePath := fmt.Sprintf("Documents/scans/%s/%s", classification.Category, newFileName)
	slog.Debug("Uploading file to Nextcloud", "remotePath", remotePath)

	// Create a PUT request to upload the file to Nextcloud
	requestURL := fmt.Sprintf("%s/remote.php/dav/files/%s/%s", nextcloudURL, username, remotePath)
	req, err := http.NewRequest("PUT", requestURL, bytes.NewReader(fileContents))
	if err != nil {
		slog.Error("Error creating PUT request", "error", err)
		return "", err
	}

	// Set the request headers
	req.Header.Set("Content-Type", "application/octet-stream")
	req.SetBasicAuth(username, password)

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Error sending PUT request", "error", err)
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	slog.Debug("Response body", "body", string(body))

	// Check if the request was successful
	if resp.StatusCode != http.StatusCreated {
		slog.Error("Error uploading file to Nextcloud", "status", resp.Status)

		return "", errors.New(fmt.Sprintf("Error uploading file to Nextcloud: %s, %v", resp.Status, string(body)))
	}

	slog.Info("Uploaded file to Nextcloud", "remotePath", remotePath)

	// get the oc-fileid from response headers
	ocFileId := resp.Header.Get("oc-fileid")
	ocEtag := resp.Header.Get("oc-etag")

	slog.Debug("oc-fileid", "oc-fileid", ocFileId)
	slog.Debug("oc-etag", "oc-etag", ocEtag)

	return fmt.Sprintf("%s/f/%s", nextcloudURL, ocFileId), nil
}

func processUserFolder(c *ftp.ServerConn, path string, user string, knownFiles map[string]bool) {
	entries, err := c.List(fmt.Sprintf("%s/%s", path, user))
	if err != nil {
		slog.Error("Error listing FTP directory", "error", err)
		return
	}

	for _, entry := range entries {
		if entry.Type != ftp.EntryTypeFolder {
			slog.Warn("Your FTP user directories should only contain files, go fuck yourself", "folder", entry.Name)
		}

		if _, ok := knownFiles[entry.Name]; ok {
			continue
		}

		slog.Info("New file", "file", entry.Name)
		sendTelegramMessage(user, fmt.Sprintf("<b>New file: <code>%s</code></b>", entry.Name))

		const maxTries = 5
		const delay = 5 * time.Second
		for i := 0; i < maxTries; i++ {
			fileName, err := downloadFile(c, fmt.Sprintf("%s/%s", path, entry.Name))
			if err != nil {
				slog.Error("Error downloading file", "error", err)
				sendTelegramMessage(user, fmt.Sprintf("Error downloading file: <pre>%s</pre>", err))
				sendTelegramMessage(user, fmt.Sprintf("%d tries left", maxTries-i))
				time.Sleep(delay)
				continue
			}
			classification, err := classifyFile(fileName)
			if err != nil {
				slog.Error("Error classifying file", "error", err)
				sendTelegramMessage(user, fmt.Sprintf("Error classifying file: %s", err))
				sendTelegramMessage(user, fmt.Sprintf("%d tries left", maxTries-i))
				time.Sleep(delay)
				continue
			}

			var providerName string
			var downloadURL string
			if viper.IsSet(fmt.Sprintf("%s.nextcloud", user)) {
				providerName = "Nextcloud"
				downloadURL, err = uploadFileToNextcloud(user, classification, fileName)
			} else if viper.IsSet(fmt.Sprintf("%s.google_drive", user)) {
				providerName = "Google Drive"
				downloadURL, err = uploadFileToGoogleDrive(user, classification, fileName)
			} else {
				slog.Error("No cloud storage provider set", "user", user)
				sendTelegramMessage(user, "No cloud storage provider set")
				break
			}

			if err != nil {
				slog.Error("Error uploading file", "provider", providerName, "error", err)
				sendTelegramMessage(user, fmt.Sprintf("Error uploading file: <pre>%s</pre>", err))
				sendTelegramMessage(user, fmt.Sprintf("%d tries left", maxTries-i))
				time.Sleep(delay)
				continue
			}

			err = sendTelegramMessage(user, fmt.Sprintf(`Classified file: %s

<b>%s</b>

<blockquote><b>Category: %s</b></blockquote>

You can download it from <a href="%s">%s</a>`, entry.Name, classification.Title, classification.Category, downloadURL, providerName))
			if err != nil {
				slog.Error("Error sending Telegram message", "error", err)
			}

			break
		}
	}

	// clear knownFiles
	knownFiles = make(map[string]bool)
	for _, entry := range entries {
		if entry.Type == ftp.EntryTypeFile {
			knownFiles[entry.Name] = true
		}
	}
}

func uploadFileToGoogleDrive(user string, classification storage.Classification, localFilePath string) (string, error) {
    // get email from config
    if !viper.IsSet(fmt.Sprintf("users.%s.google_drive.email", user)) {
        return "", errors.New("Google Drive email not set for user")
    }

    email := viper.GetString(fmt.Sprintf("users.%s.google_drive.email", user))

    storage.StoreFile(email, localFilePath, classification)
}
