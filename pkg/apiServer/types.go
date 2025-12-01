package apiServer

import (
	"net/http"

	ouroboros "github.com/i5heu/ouroboros-db"
)

type createResponse struct {
	Key string `json:"key"`
}

type listResponse struct {
	Keys []string `json:"keys"`
}

type searchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

type searchResponse struct {
	Keys []string `json:"keys"`
}

type createMetadata struct {
	ReedSolomonShards       uint8    `json:"reed_solomon_shards"`
	ReedSolomonParityShards uint8    `json:"reed_solomon_parity_shards"`
	Parent                  string   `json:"parent"`
	Children                []string `json:"children"`
	MimeType                string   `json:"mime_type"`
	IsText                  *bool    `json:"is_text"`
	Filename                string   `json:"filename"`
	Title                   string   `json:"title,omitempty"`
	EditOf                  string   `json:"edit_of,omitempty"`
}

type AuthFunc func(req *http.Request, db *ouroboros.OuroborosDB) error

type Option func(*Server)
