// Package api provides the API layer for OuroborosDB.
// This package will contain REST/gRPC handlers for external access.
package api

// API defines the interface for the external API.
type API interface {
	// Start starts the API server.
	Start() error

	// Stop stops the API server.
	Stop() error
}

// Config holds configuration for the API server.
type Config struct {
	// Port is the port to listen on.
	Port int

	// EnableTLS enables TLS for the API server.
	EnableTLS bool

	// CertFile is the path to the TLS certificate file.
	CertFile string

	// KeyFile is the path to the TLS key file.
	KeyFile string
}
