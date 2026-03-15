package discord

import "time"

// Internal types — normalized before storing.

type Guild struct {
	ID   string
	Name string
	Icon string
}

type Channel struct {
	ID      string
	GuildID string
	Name    string
	Type    int // 0=text, 1=DM, 2=voice, 4=category, 5=announcement, 11=thread
	Topic   string
}

type Message struct {
	ID          string
	ChannelID   string
	GuildID     string
	AuthorID    string
	AuthorName  string
	Content     string
	Timestamp   time.Time
	Attachments []Attachment
	Edited      bool
}

type Attachment struct {
	ID       string
	Filename string
	URL      string
	Size     int
}
