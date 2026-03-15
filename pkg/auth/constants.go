package auth

// Domain-separation context for CA signing NodeCerts.
const CTXNodeAdmissionV1 = "OUROBOROS_NODE_ADMISSION_V1" // A

// Domain-separation context for node signing
// DelegationProofs.
const CTXNodeDelegationV1 = "OUROBOROS_NODE_DELEGATION_V1" // A

// Domain-separation context for AdminCA anchoring
// UserCAs.
const CTXUserCAAnchorV1 = "OUROBOROS_USER_CA_ANCHOR_V1" // A

// ExporterLabel is the TLS exporter derivation label
// used for delegation binding.
const ExporterLabel = "EXPORTER_OUROBOROS_NODE_DELEGATION_V1" // A

// MaxDelegationTTL is the maximum allowed lifetime
// of a DelegationProof in seconds (5 minutes).
const MaxDelegationTTL int64 = 300 // A

// MaxPeerCertBundleSize is the maximum number of
// certificates allowed in a peer cert bundle.
const MaxPeerCertBundleSize int = 1024 // A

// DefaultCertVersion is the current NodeCert payload
// version.
const DefaultCertVersion uint16 = 1 // A

// Slog key constants for structured logging.
const ( // A
	LogKeyStep      = "step"
	LogKeyNodeID    = "nodeID"
	LogKeyCAHash    = "caHash"
	LogKeyScope     = "scope"
	LogKeyReason    = "reason"
	LogKeyCertIndex = "certIndex"
)
