package discord

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/bwmarrin/discordgo"
)


// NewSession creates an authenticated discordgo session.
func NewSession(token *StoredToken) (*discordgo.Session, error) {
	s, err := discordgo.New(token.FormatToken())
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}
	// Validate token
	_, err = s.User("@me")
	if err != nil {
		return nil, fmt.Errorf("invalid token — run `discocli auth` again: %w", err)
	}
	return s, nil
}

// FetchMessages paginates through messages in a channel oldest-first.
// Pass beforeID to start from a specific point (for resumable sync).
// Pass stopBefore (a snowflake ID string) to stop when messages older than
// the cutoff are reached — use TimeToSnowflake to convert a time.Time.
// Calls onBatch for each batch of up to 100 messages.
func FetchMessages(s *discordgo.Session, channelID string, onBatch func([]*discordgo.Message) error, beforeID, stopBefore string) error {
	const (
		baseDelay  = 400 * time.Millisecond // minimum wait between batches
		jitter     = 300 * time.Millisecond // random extra 0–300ms on top
		backoffMax = 10 * time.Second       // cap for 429 back-off
	)
	backoff := 2 * time.Second // starting back-off on rate limit hit

	for {
		msgs, err := s.ChannelMessages(channelID, 100, beforeID, "", "")
		if err != nil {
			// discordgo surfaces 429s as errors — back off and retry once
			if isRateLimit(err) {
				time.Sleep(backoff)
				backoff *= 2
				if backoff > backoffMax {
					backoff = backoffMax
				}
				continue
			}
			return fmt.Errorf("failed to fetch messages: %w", err)
		}
		// Successful fetch — reset back-off
		backoff = 2 * time.Second

		if len(msgs) == 0 {
			break
		}

		// Apply cutoff: msgs are newest-first; once we cross stopBefore,
		// all subsequent messages are older, so trim and stop.
		reachedCutoff := false
		if stopBefore != "" {
			for i, m := range msgs {
				if m.ID < stopBefore {
					msgs = msgs[:i]
					reachedCutoff = true
					break
				}
			}
		}

		if len(msgs) > 0 {
			if err := onBatch(msgs); err != nil {
				return err
			}
			beforeID = msgs[len(msgs)-1].ID
		}

		if reachedCutoff {
			break
		}

		// Jittered delay: 400ms base + random 0–300ms
		sleep := baseDelay + time.Duration(rand.Int63n(int64(jitter)))
		time.Sleep(sleep)
	}
	return nil
}

// isRateLimit checks if a discordgo error is an HTTP 429.
func isRateLimit(err error) bool {
	if err == nil {
		return false
	}
	restErr, ok := err.(*discordgo.RESTError)
	return ok && restErr.Response != nil && restErr.Response.StatusCode == 429
}

// isUserTokenForbidden checks for HTTP 403 with Discord code 20002
// ("Only bots can use this endpoint") — hit when a user token calls a bot-only API.
func isUserTokenForbidden(err error) bool {
	if err == nil {
		return false
	}
	restErr, ok := err.(*discordgo.RESTError)
	return ok && restErr.Response != nil && restErr.Response.StatusCode == 403
}

// threadSearchResponse is the shape of GET /channels/{id}/threads/search.
// This endpoint works with user tokens and is what the Discord app itself uses.
type threadSearchResponse struct {
	Threads      []*discordgo.Channel `json:"threads"`
	HasMore      bool                 `json:"has_more"`
	TotalResults int                  `json:"total_results"`
}

// fetchActiveThreadsUserToken uses GET /channels/{id}/threads/search to list
// active threads — the only active-threads endpoint available to user tokens.
// It paginates via offset until has_more is false.
func fetchActiveThreadsUserToken(s *discordgo.Session, channelID string) ([]*discordgo.Channel, error) {
	var all []*discordgo.Channel
	offset := 0
	const pageSize = 25

	for {
		url := fmt.Sprintf(
			"https://discord.com/api/v10/channels/%s/threads/search?limit=%d&sort_by=last_message_time&sort_order=desc&archived=false&offset=%d",
			channelID, pageSize, offset,
		)
		body, err := s.RequestWithBucketID("GET", url, nil, "threads/search")
		if err != nil {
			return nil, err
		}
		var result threadSearchResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decode thread search: %w", err)
		}
		all = append(all, result.Threads...)
		if !result.HasMore || len(result.Threads) == 0 {
			break
		}
		offset += len(result.Threads)
		time.Sleep(300 * time.Millisecond)
	}
	return all, nil
}

// FetchThreads returns all active and archived public threads for a forum/media channel.
// For bot tokens it uses the standard ThreadsActive endpoint; for user tokens it falls
// back to the threads/search endpoint that Discord's own app uses.
func FetchThreads(s *discordgo.Session, channelID, guildID string) ([]*discordgo.Channel, error) {
	var activeThreads []*discordgo.Channel

	active, err := s.ThreadsActive(channelID)
	if err != nil {
		if isUserTokenForbidden(err) {
			// Bot-only endpoint. Use the search endpoint instead — works with user tokens.
			activeThreads, err = fetchActiveThreadsUserToken(s, channelID)
			if err != nil {
				return nil, fmt.Errorf("fetch active threads (user token) for %s: %w", channelID, err)
			}
		} else {
			return nil, fmt.Errorf("fetch active threads for %s: %w", channelID, err)
		}
	} else {
		activeThreads = active.Threads
	}

	threads := make([]*discordgo.Channel, 0, len(activeThreads))
	threads = append(threads, activeThreads...)

	// Paginate archived threads until exhausted; before=nil starts from most recent
	var before *time.Time
	for {
		archived, err := s.ThreadsArchived(channelID, before, 100)
		if err != nil {
			return nil, fmt.Errorf("fetch archived threads for %s: %w", channelID, err)
		}
		threads = append(threads, archived.Threads...)
		if !archived.HasMore || len(archived.Threads) == 0 {
			break
		}
		last := archived.Threads[len(archived.Threads)-1]
		if last.ThreadMetadata != nil && !last.ThreadMetadata.ArchiveTimestamp.IsZero() {
			t := last.ThreadMetadata.ArchiveTimestamp
			before = &t
		} else {
			break
		}
	}

	return threads, nil
}

// IsSyncableChannel reports whether a channel type carries messages directly.
func IsSyncableChannel(t discordgo.ChannelType) bool {
	switch t {
	case discordgo.ChannelTypeGuildText,
		discordgo.ChannelTypeGuildVoice,
		discordgo.ChannelTypeGuildNews,
		discordgo.ChannelTypeGuildNewsThread,
		discordgo.ChannelTypeGuildPublicThread,
		discordgo.ChannelTypeGuildPrivateThread:
		return true
	}
	return false
}

// IsThreadContainer reports whether a channel type holds threads (forum/media).
func IsThreadContainer(t discordgo.ChannelType) bool {
	return t == discordgo.ChannelTypeGuildForum || t == discordgo.ChannelTypeGuildMedia
}

// ChannelTypeLabel returns a short display label for a channel type.
func ChannelTypeLabel(t discordgo.ChannelType) string {
	switch t {
	case discordgo.ChannelTypeGuildText:
		return "text"
	case discordgo.ChannelTypeGuildVoice:
		return "voice"
	case discordgo.ChannelTypeGuildNews:
		return "announcement"
	case discordgo.ChannelTypeGuildForum:
		return "forum"
	case discordgo.ChannelTypeGuildMedia:
		return "media"
	case discordgo.ChannelTypeGuildNewsThread,
		discordgo.ChannelTypeGuildPublicThread,
		discordgo.ChannelTypeGuildPrivateThread:
		return "thread"
	default:
		return "unknown"
	}
}

// ResolveChannelByName finds a text channel by name within a guild.
func ResolveChannelByName(s *discordgo.Session, guildID, name string) (*discordgo.Channel, error) {
	channels, err := s.GuildChannels(guildID)
	if err != nil {
		return nil, fmt.Errorf("failed to list channels: %w", err)
	}
	for _, ch := range channels {
		if ch.Name == name && ch.Type == discordgo.ChannelTypeGuildText {
			return ch, nil
		}
	}
	return nil, fmt.Errorf("channel %q not found in guild %s", name, guildID)
}

// ResolveGuildByName finds a guild by name from the user's guild list.
func ResolveGuildByName(s *discordgo.Session, name string) (*discordgo.UserGuild, error) {
	guilds, err := s.UserGuilds(100, "", "", false)
	if err != nil {
		return nil, fmt.Errorf("failed to list guilds: %w", err)
	}
	for _, g := range guilds {
		if g.Name == name {
			return g, nil
		}
	}
	return nil, fmt.Errorf("guild %q not found", name)
}
