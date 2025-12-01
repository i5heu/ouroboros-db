package meta

import "time"

// Metadata represents the stored metadata for a piece of data.
// CreatedAt is stored as an ISO-8601 timestamp in JSON and unmarshalled
// into a time.Time value.
type Metadata struct {
	CreatedAt time.Time `json:"created_at"`
	Title     string    `json:"title,omitempty"`
	MimeType  string    `json:"mime_type,omitempty"`
	// EditOf optionally links this entry to the hash it edits.
	// Stored as a hex-encoded hash string to keep metadata JSON self-contained.
	EditOf string `json:"edit_of,omitempty"`
}
