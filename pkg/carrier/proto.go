package carrier

import (
	oldproto "github.com/golang/protobuf/proto"
)

type authNodeCert struct { // A
	CertVersion    uint32 `protobuf:"varint,1,opt,name=certVersion,proto3"`
	NodePubKeyKEM  []byte `protobuf:"bytes,2,opt,name=nodePubKeyKEM,proto3"`
	NodePubKeySign []byte `protobuf:"bytes,3,opt,name=nodePubKeySign,proto3"`
	IssuerCAHash   string `protobuf:"bytes,4,opt,name=issuerCAHash,proto3"`
	ValidFrom      int64  `protobuf:"varint,5,opt,name=validFrom,proto3"`
	ValidUntil     int64  `protobuf:"varint,6,opt,name=validUntil,proto3"`
	Serial         []byte `protobuf:"bytes,7,opt,name=serial,proto3"`
	CertNonce      []byte `protobuf:"bytes,8,opt,name=certNonce,proto3"`
}

func (m *authNodeCert) Reset() { // A
	*m = authNodeCert{}
}

func (m *authNodeCert) String() string { // A
	return oldproto.CompactTextString(m)
}

func (*authNodeCert) ProtoMessage() {} // A

type authHello struct { // A
	Certs        []*authNodeCert `protobuf:"bytes,1,rep,name=certs,proto3"`
	CASignatures [][]byte        `protobuf:"bytes,2,rep,name=caSignatures,proto3"`
}

func (m *authHello) Reset() { // A
	*m = authHello{}
}

func (m *authHello) String() string { // A
	return oldproto.CompactTextString(m)
}

func (*authHello) ProtoMessage() {} // A

type authProof struct { // A
	TLSCertPubKeyHash  []byte `protobuf:"bytes,1,opt,name=tlsCertPubKeyHash,proto3"`
	TLSExporterBinding []byte `protobuf:"bytes,2,opt,name=tlsExporterBinding,proto3"`
	TLSTranscriptHash  []byte `protobuf:"bytes,3,opt,name=tlsTranscriptHash,proto3"`
	X509Fingerprint    []byte `protobuf:"bytes,4,opt,name=x509Fingerprint,proto3"`
	NodeCertBundleHash []byte `protobuf:"bytes,5,opt,name=nodeCertBundleHash,proto3"`
	NotBefore          int64  `protobuf:"varint,6,opt,name=notBefore,proto3"`
	NotAfter           int64  `protobuf:"varint,7,opt,name=notAfter,proto3"`
	DelegationSig      []byte `protobuf:"bytes,8,opt,name=delegationSig,proto3"`
}

func (m *authProof) Reset() { // A
	*m = authProof{}
}

func (m *authProof) String() string { // A
	return oldproto.CompactTextString(m)
}

func (*authProof) ProtoMessage() {} // A
