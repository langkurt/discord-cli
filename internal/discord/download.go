package discord

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/virat-mankali/discord-cli/internal/storage"
)

// DownloadOptions configures a batch download run.
type DownloadOptions struct {
	ChannelID string // filter by channel ID (optional)
	GuildID   string // filter by guild ID (optional)
	MediaType string // "image", "gif", "video", "all"
	OutDir    string // destination root directory
	Limit     int    // 0 = unlimited
	// GuildName / ChannelName for folder organisation — fetched by caller
	GuildName   string
	ChannelName string
}

// DownloadResult summarises a completed batch.
type DownloadResult struct {
	Downloaded int
	Skipped    int
	Failed     int
}

// ProgressFunc is called before each file is downloaded.
// Returning false cancels the remaining downloads.
type ProgressFunc func(index, total int, filename string)

// DownloadAttachments fetches pending attachments and saves them to disk.
func DownloadAttachments(db *storage.DB, opts DownloadOptions, progress ProgressFunc) (DownloadResult, error) {
	pending, err := db.ListPendingAttachments(opts.ChannelID, opts.GuildID, opts.MediaType, opts.Limit)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("listing attachments: %w", err)
	}

	// Build destination folder: <out>/<guild>/<channel>/
	destDir := opts.OutDir
	if opts.GuildName != "" {
		destDir = filepath.Join(destDir, sanitiseName(opts.GuildName))
	}
	if opts.ChannelName != "" {
		destDir = filepath.Join(destDir, sanitiseName(opts.ChannelName))
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return DownloadResult{}, fmt.Errorf("creating output dir: %w", err)
	}

	var res DownloadResult
	client := &http.Client{Timeout: 30 * time.Second}

	for i, a := range pending {
		if progress != nil {
			progress(i+1, len(pending), a.Filename)
		}

		localPath, err := downloadOne(client, a, destDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: failed to download %s: %v\n", a.Filename, err)
			res.Failed++
			continue
		}

		if err := db.MarkDownloaded(a.ID, localPath); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: failed to record %s: %v\n", a.Filename, err)
		}
		res.Downloaded++
		time.Sleep(200 * time.Millisecond)
	}

	return res, nil
}

// downloadOne downloads a single attachment and returns the local file path.
func downloadOne(client *http.Client, a storage.Attachment, destDir string) (string, error) {
	resp, err := client.Get(a.URL)
	if err != nil {
		return "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	// Avoid collisions by prefixing with the message ID's last 8 chars
	suffix := a.MessageID
	if len(suffix) > 8 {
		suffix = suffix[len(suffix)-8:]
	}
	filename := suffix + "_" + a.Filename
	localPath := filepath.Join(destDir, filename)

	// If file already exists on disk, skip writing but still mark as downloaded
	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}

	f, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(localPath) // clean up partial file
		return "", fmt.Errorf("write file: %w", err)
	}

	return localPath, nil
}

// sanitiseName strips characters that are problematic in file paths.
func sanitiseName(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '/' || c == '\\' || c == ':' || c == '*' || c == '?' || c == '"' || c == '<' || c == '>' || c == '|' {
			out = append(out, '_')
		} else {
			out = append(out, c)
		}
	}
	return string(out)
}
