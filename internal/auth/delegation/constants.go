package delegation

// Domain-separation context for node signing
// DelegationProofs.
const CTXNodeDelegationV1 = "OUROBOROS_NODE_DELEGATION_V1" // A

// ExporterLabel is the TLS exporter derivation label
// used for delegation binding.
const ExporterLabel = "EXPORTER_OUROBOROS_NODE_DELEGATION_V1" // A

// TranscriptBindingLabel is the TLS exporter label
// used as a transcript hash substitute. TLS 1.3 does
// not expose the raw handshake transcript hash; EKM
// with a fixed label provides the same binding
// properties (RFC 9266).
const TranscriptBindingLabel = "EXPORTER_OUROBOROS_TRANSCRIPT_BINDING_V1" // A

// MaxDelegationTTL is the maximum allowed lifetime
// of a DelegationProof in seconds (5 minutes).
const MaxDelegationTTL int64 = 300 // A

// Binding field sizes for hash- and exporter-based
// delegation inputs.
const ( // A
	TLSExporterBindingSize = 32
	TLSTranscriptHashSize  = 32
)
