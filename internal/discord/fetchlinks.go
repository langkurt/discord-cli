package discord

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/virat-mankali/discord-cli/internal/storage"
)

// FetchLinksOptions configures a fetch-links run.
type FetchLinksOptions struct {
	ChannelID   string
	GuildID     string
	ChannelName string
	GuildName   string
	OutDir      string
	Limit       int
}

// FetchLinksResult summarises a completed run.
type FetchLinksResult struct {
	Downloaded int
	Failed     int
	Skipped    int
}

// LinkProgressFunc is called before processing each link.
type LinkProgressFunc func(index, total int, url string)

// LinkID generates a stable ID for a (messageID, url) pair.
func LinkID(messageID, url string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(messageID+"|"+url)))
}

// ExtractAndStoreLinks scans existing stored messages and upserts link rows for
// text URLs found in content.  This only covers URLs typed inline — embed
// images captured via ProxyURL require a re-sync.
func ExtractAndStoreLinks(db *storage.DB, channelID string) (int, error) {
	rows, err := db.MessagesWithContent(channelID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, msg := range rows {
		urls := ExtractURLs(msg.Content)
		for _, u := range urls {
			id := LinkID(msg.ID, u)
			if err := db.UpsertLink(id, msg.ID, channelID, u, ""); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

// FetchAndDownloadLinks downloads pending link images.
//
// Strategy per link:
//  1. If proxy_url is set (captured from Discord embed during sync), download
//     from Discord's CDN directly — same as the Discord "Download" button.
//  2. Otherwise fall back to fetching og:image from the page (slower, less
//     reliable, requires the proxy site to still be live).
func FetchAndDownloadLinks(db *storage.DB, opts FetchLinksOptions, progress LinkProgressFunc) (FetchLinksResult, error) {
	pending, err := db.ListPendingLinks(opts.ChannelID, opts.GuildID, opts.Limit)
	if err != nil {
		return FetchLinksResult{}, fmt.Errorf("listing links: %w", err)
	}

	destDir := opts.OutDir
	if opts.GuildName != "" {
		destDir = filepath.Join(destDir, sanitiseName(opts.GuildName))
	}
	if opts.ChannelName != "" {
		destDir = filepath.Join(destDir, sanitiseName(opts.ChannelName))
	}
	destDir = filepath.Join(destDir, "links")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return FetchLinksResult{}, fmt.Errorf("creating output dir: %w", err)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	var res FetchLinksResult

	for i, link := range pending {
		if progress != nil {
			progress(i+1, len(pending), link.URL)
		}

		var imgURL string

		if link.ProxyURL != "" {
			// Fast path: Discord already proxied this image — download directly.
			imgURL = link.ProxyURL
		} else {
			// Slow path: scrape og:image from the external page.
			var scrapeErr error
			imgURL, scrapeErr = FetchOGImage(client, link.URL)
			if scrapeErr != nil {
				_ = db.MarkLinkFailed(link.ID, scrapeErr.Error())
				res.Failed++
				time.Sleep(DelayForURL(link.URL))
				continue
			}
		}

		localPath, err := downloadLinkImage(client, imgURL, link.ID, destDir)
		if err != nil {
			_ = db.MarkLinkFailed(link.ID, fmt.Sprintf("download: %s", err))
			res.Failed++
			time.Sleep(DelayForURL(link.URL))
			continue
		}

		_ = db.MarkLinkDownloaded(link.ID, localPath)
		res.Downloaded++

		// Only delay when we hit external sites; Discord CDN is fine with fast requests.
		if link.ProxyURL == "" {
			time.Sleep(DelayForURL(link.URL))
		}
	}

	return res, nil
}

// downloadLinkImage downloads an image URL and saves it to destDir.
func downloadLinkImage(client *http.Client, imgURL, linkID, destDir string) (string, error) {
	resp, err := client.Get(imgURL)
	if err != nil {
		return "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	// Derive filename from URL path
	filename := filepath.Base(strings.Split(imgURL, "?")[0])
	if filename == "" || filename == "." || filename == "/" {
		filename = "image.jpg"
	}
	// Prefix with short link ID to avoid collisions
	suffix := linkID
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	filename = suffix + "_" + filename
	localPath := filepath.Join(destDir, filename)

	// Skip if already on disk
	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}

	f, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(localPath)
		return "", fmt.Errorf("write: %w", err)
	}
	return localPath, nil
}
