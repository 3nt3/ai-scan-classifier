package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"log/slog"

	"github.com/labstack/echo/v4"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v2"
	"google.golang.org/api/option"
)

type AppContext struct {
	echo.Context
	Oauth2Config *oauth2.Config
	DB           *sql.DB
}

func saveToken(db *sql.DB, userID string, token *oauth2.Token) error {
	insertSQL := `INSERT INTO oauth_tokens (user_id, refresh_token, token_type)
	              VALUES (?, ?, ?)
	              ON CONFLICT(user_id) DO UPDATE SET
	              refresh_token=excluded.refresh_token,
	              token_type=excluded.token_type`

	_, err := db.Exec(insertSQL, userID, token.RefreshToken, token.TokenType)
	return err
}

func getToken(db *sql.DB, userID string) (*oauth2.Token, error) {
    selectSQL := `SELECT refresh_token, token_type FROM oauth_tokens WHERE user_id = ?`

    row := db.QueryRow(selectSQL, userID)
    var refreshToken, tokenType string
    err := row.Scan(&refreshToken, &tokenType)
    if err != nil {
        return nil, err
    }

    return &oauth2.Token{
        RefreshToken: refreshToken,
        TokenType: tokenType,
    }, nil
}

func redirect(c echo.Context) error {
	ac := c.(*AppContext)
	url := ac.Oauth2Config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	return c.Redirect(302, url)
}

func callback(c echo.Context) error {
	ac := c.(*AppContext)

	code := c.QueryParam("code")
	token, err := ac.Oauth2Config.Exchange(context.Background(), code)
	if err != nil {
		slog.Warn("Unable to retrieve token from web: %v", "error", err)
		return c.String(500, "Unable to retrieve token from web")
	}

	slog.Info("Received token", "token", token)

	// get email address
	srv, err := drive.NewService(context.Background(), option.WithHTTPClient(ac.Oauth2Config.Client(context.Background(), token)))
	if err != nil {
		slog.Warn("Unable to create Drive service", "error", err)
		return c.String(500, "Unable to create Drive service")
	}

	user, err := srv.About.Get().Fields("user").Do()
	if err != nil {
		slog.Info("Unable to retrieve user info", "error", err)
		return c.String(500, "Unable to retrieve user info")
	}

	err = saveToken(ac.DB, user.User.EmailAddress, token)
	if err != nil {
		slog.Warn("Unable to save token", "error", err)
		return c.String(500, "Unable to save token")
	}

	return c.String(200, "Token saved successfully")
}

// getFolderID retrieves the ID of a folder with the given path and creates if it doesn't exist
func getFolderID(srv *drive.Service, name string) (string, error) {
    q := fmt.Sprintf("mimeType='application/vnd.google-apps.folder' and name='%s'", name)
    r, err := srv.Files.List().Q(q).Do()
    if err != nil {
        return "", fmt.Errorf("unable to list files: %w", err)
    }

    if len(r.Files) == 0 {
        f := &drive.File{
            Name: name,
            MimeType: "application/vnd.google-apps.folder",
        }
        f, err := srv.Files.Create(f).Do()
        if err != nil {
            return "", fmt.Errorf("unable to create folder: %w", err)
        }
        return f.Id, nil
    }

    return r.Files[0].Id, nil
}

func StoreFile(db *sql.DB, userID string, data []byte, classification Classification) (string, error) {
    token, err := getToken(db, userID)
    if err != nil {
        return "", fmt.Errorf("unable to retrieve token: %w", err)
    }

    srv, err := drive.NewService(context.Background(), option.WithHTTPClient(ac.Oauth2Config.Client(context.Background(), token)))
    if err != nil {
        return "", fmt.Errorf("unable to create Drive service: %w", err)
    }

    file := &drive.File{
        Name: classification.FileName,
        MimeType: "application/octet-stream",
        Parents: []string{classification.Category},
    }
}

func RunServer() {
	slog.Info("Creating table...")
	db, err := sql.Open("sqlite3", "./tokens.db")
	if err != nil {
        slog.Error("error opening db", "error", err)
        panic(err)
	}

	defer db.Close()

	createTableSQL := `CREATE TABLE IF NOT EXISTS oauth_tokens (
		"user_id" TEXT PRIMARY KEY,
		"refresh_token" TEXT NOT NULL,
		"token_type" TEXT NOT NULL
	);`

	_, err = db.Exec(createTableSQL)
	if err != nil {
        slog.Error("error creating table ", "error", err)
        panic(err)
	}

	slog.Info("Table created successfully.")

	// Load client secrets from a local file.
	b, err := os.ReadFile("creds.json")
	if err != nil {
		slog.Error("Unable to read client secret file", "error", err)
        panic(err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, drive.DriveFileScope)
	if err != nil {
		slog.Error("Unable to parse client secret file to config", "error", err)
        panic(err)
	}

	// FIXME
	config.RedirectURL = "http://localhost:8080/callback"

	slog.Info("Config", "config", config)

	e := echo.New()

	e.GET("/auth", redirect)
	e.GET("/callback", callback)
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc := &AppContext{c, config, db}
			return next(cc)
		}
	})

	e.Logger.Fatal(e.Start(":8080"))
}
