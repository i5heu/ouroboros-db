package auth

// MaxPeerCertBundleSize is the maximum number of
// certificates allowed in a peer cert bundle.
const MaxPeerCertBundleSize int = 1024 // A

// MaxRevocationEntries is the maximum number of
// entries allowed in each revocation map.
const MaxRevocationEntries int = 100_000 // A

// Binding field sizes for hash- and exporter-based
// delegation inputs.
const ( // A
	TLSCertPubKeyHashSize  = 32
	X509FingerprintSize    = 32
	NodeCertBundleHashSize = 32
)

// Slog key constants for structured logging.
const ( // A
	LogKeyStep      = "step"
	LogKeyNodeID    = "nodeID"
	LogKeyCAHash    = "caHash"
	LogKeyScope     = "scope"
	LogKeyReason    = "reason"
	LogKeyCertIndex = "certIndex"
)
