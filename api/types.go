package api

import "net/http"

type createResponse struct {
	Key string `json:"key"`
}

type listResponse struct {
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
}

type AuthFunc func(*http.Request) error

type Option func(*Server)
