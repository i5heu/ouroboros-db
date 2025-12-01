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

type lookupResponse struct {
	ComputedID string   `json:"computed_id"`
	Keys       []string `json:"keys"`
}

type computedIDResponse struct {
	Key        string `json:"key"`
	ComputedID string `json:"computed_id"`
}

// lookupWithDataRecord is used in the combined lookup+data endpoint response
type lookupWithDataRecord struct {
	Key            string `json:"key"`
	ResolvedKey    string `json:"resolvedKey,omitempty"`
	SuggestedEdit  string `json:"suggestedEdit,omitempty"`
	EditOf         string `json:"editOf,omitempty"`
	Found          bool   `json:"found"`
	MimeType       string `json:"mimeType,omitempty"`
	IsText         bool   `json:"isText,omitempty"`
	SizeBytes      int    `json:"sizeBytes,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
	Content        string `json:"content,omitempty"`
	EncodedContent string `json:"encodedContent,omitempty"`
	Error          string `json:"error,omitempty"`
	Title          string `json:"title,omitempty"`
	ComputedID     string `json:"computedId,omitempty"`
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
