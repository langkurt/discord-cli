package discord

import (
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
