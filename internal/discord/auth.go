package discord

import (
	"fmt"
	"os"
	"strings"
)

const (
	TokenTypeBot  = "bot"
	TokenTypeUser = "user"
)

type StoredToken struct {
	Token     string
	TokenType string // "bot" or "user"
}

// SaveToken writes token to disk with 0600 permissions.
func SaveToken(path, token, tokenType string) error {
	content := fmt.Sprintf("%s\n%s\n", tokenType, token)
	return os.WriteFile(path, []byte(content), 0600)
}

// LoadToken reads token from disk.
func LoadToken(path string) (*StoredToken, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("not authenticated — run `discocli auth` first")
	}
	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(lines) != 2 {
		return nil, fmt.Errorf("corrupted token file — run `discocli auth` again")
	}
	return &StoredToken{TokenType: lines[0], Token: lines[1]}, nil
}

// FormatToken returns the token in the format discordgo expects.
func (t *StoredToken) FormatToken() string {
	if t.TokenType == TokenTypeBot {
		return "Bot " + t.Token
	}
	return t.Token // user tokens don't have a prefix
}
