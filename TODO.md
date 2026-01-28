# OuroborosDB - Project Completion TODO

This document tracks all tasks required to bring OuroborosDB from prototype to production-ready status.

**Current Status:** Alpha / Prototype
**Last Updated:** 2026-01-28

---

## 1. Core Infrastructure (Not Implemented)

These components exist in the architecture plan but have no working implementation.

### ClusterController
- [ ] Implement ClusterController to coordinate cluster operations
- [ ] Integrate with Carrier for node communication
- [ ] Connect to DistributedIndex for hash/key lookups
- [ ] Manage DataReBalancer lifecycle

### DistributedIndex
- [ ] Implement `HashToNode` - map content hashes to responsible nodes
- [ ] Implement `KeyToHashAndNode` - resolve keys to hash and node location
- [ ] Add consistent hashing for node assignment
- [ ] Handle node join/leave rebalancing of hash ranges

### DataRouter (Distributed Operations)
- [ ] Current implementation in `internal/dataRouting` is a stub
- [ ] Implement `DistributeBlockSlices()` - send slices to appropriate nodes
- [ ] Implement distributed `RetrieveBlock()` - collect slices from nodes
- [ ] Implement distributed `StoreVertex()` and `RetrieveVertex()`
- [ ] Add retry logic for failed node communications

---

## 2. Security (Critical Priority)

### TLS Certificate Validation
- [ ] Remove `InsecureSkipVerify: true` from `internal/carrier/quic_transport.go:83`
- [ ] Remove `InsecureSkipVerify: true` from `internal/carrier/stream/quic.go:107`
- [ ] Implement proper certificate chain validation
- [ ] Add certificate pinning for known cluster nodes

### Node Authentication
- [ ] Implement node authentication during QUIC handshake
- [ ] Add mutual TLS (mTLS) for node-to-node communication
- [ ] Implement node join approval workflow (`internal/carrier/carrier_handlers.go:97`)
- [ ] Add node identity verification using cluster certificates

### Key Wrapping (Cryptographic Weakness)
- [ ] Replace XOR key wrapping with HKDF + AES-KW in `internal/encryption/encryption.go`
  - Current: `aesKey[i] = wrappedAESKey[i] ^ sharedSecret[i]` (lines 218-222, 325-329)
  - Target: Use HKDF to derive wrapping key, then AES-KW for key encapsulation
- [ ] Add proper key derivation function for shared secrets

### Message Signing
- [ ] Integrate ML-DSA (Dilithium) for message signing
- [ ] Sign all inter-node messages
- [ ] Verify signatures on message receipt
- [ ] Add replay protection with nonces/timestamps

### Private Key Protection
- [ ] Encrypt private keys at rest
- [ ] Implement secure key loading with passphrase
- [ ] Add key rotation mechanism

### Access Control
- [ ] Enforce KeyEntry access control - verify requestor has valid KeyEntry before serving chunks
- [ ] Implement per-chunk access revocation
- [ ] Add audit logging for access attempts

---

## 3. Data Integrity (High Priority)

### WAL Crash Recovery
- [ ] Implement crash recovery mechanism for WAL
- [ ] Fix atomicity issue in `SealBlock` - WAL entries deleted before BlockStore confirmation
  - See `internal/wal/wal.go` - entries should not be deleted until block is safely persisted
- [ ] Add WAL replay on startup
- [ ] Implement checkpointing for faster recovery

### Post-Write Verification
- [ ] Add hash verification after writing to BlockStore
- [ ] Verify block integrity on retrieval
- [ ] Implement background integrity scanning

### Reed-Solomon Verification
- [ ] Verify reconstructed blocks match original hash after RS decoding
- [ ] Add parity slice validation
- [ ] Implement automatic repair of corrupted slices

### CAS Vertex Hash Calculation
- [ ] Fix vertex hash calculation to include all fields
- [ ] Ensure hash stability across serialization/deserialization
- [ ] Add hash verification on vertex retrieval

### Region Bounds Validation
- [ ] Validate ChunkRegion offset/length against block bounds
- [ ] Validate VertexRegion offset/length against block bounds
- [ ] Return proper errors for out-of-bounds requests

---

## 4. Cluster Operations (Stub Implementations)

### DeletionWAL
- [ ] Implement actual deletion logic in `ProcessDeletions()`
  - Current: empties list without performing deletions
- [ ] Coordinate deletions across cluster nodes
- [ ] Add deletion confirmation tracking
- [ ] Implement garbage collection for orphaned data

### DataReBalancer
- [ ] Implement `BalanceData()` - redistribute slices when nodes join/leave
- [ ] Add incremental rebalancing to avoid thundering herd
- [ ] Implement priority-based rebalancing (hot data first)
- [ ] Add progress tracking and resumption

### ReplicationMonitoring
- [ ] Implement `MonitorReplications()` - track replication factor per block
- [ ] Alert when replication falls below threshold
- [ ] Trigger automatic re-replication
- [ ] Add replication metrics and dashboard

### ClusterMonitor / NodeAvailabilityTracker
- [ ] Implement `MonitorNodeHealth()` - active health checks
- [ ] Implement `TrackAvailability()` - heartbeat-based availability
- [ ] Add node failure detection with configurable timeouts
- [ ] Implement node recovery detection and reintegration

### SyncIndexTree
- [ ] Implement `Sync()` - synchronize index state across nodes
- [ ] Add Merkle tree for efficient index comparison
- [ ] Implement incremental index sync

### BackupManager
- [ ] Implement `BackupData()` - full and incremental backups
- [ ] Add backup scheduling
- [ ] Implement backup verification
- [ ] Add restore functionality

### Carrier Message Serialization
- [ ] Implement payload serialization for `JoinCluster` (`internal/carrier/carrier_membership.go:31`)
- [ ] Implement payload serialization for `LeaveCluster` (`internal/carrier/carrier_membership.go:56`)
- [ ] Implement join request serialization (`internal/carrier/carrier_bootstrap.go:59`)

---

## 5. Performance Optimizations

### N+1 Query in ListBlockSlices
- [ ] Optimize `ListBlockSlices` in `internal/blockstore/blockstore.go:154`
- [ ] Use batch retrieval instead of individual lookups
- [ ] Add result caching for frequently accessed blocks

### WAL Buffer Size Tracking
- [ ] Fix O(N) buffer size recalculation in `internal/wal/wal.go:59-84`
- [ ] Track buffer size incrementally on append/delete
- [ ] Remove need for full iteration on `recalcBufferSize()`

### Partial Block Reads
- [ ] Implement efficient partial block reads using ChunkRegion
- [ ] Avoid loading entire block when only one chunk needed
- [ ] Add memory-mapped file access for large blocks

### Distribution Tracker Memory
- [ ] Fix potential memory leak in BlockDistributionTracker
- [ ] Add cleanup for completed/failed distributions
- [ ] Implement bounded history for distribution records

### Caching Layer
- [ ] Add in-memory LRU cache for hot blocks
- [ ] Implement cache invalidation on updates
- [ ] Add cache size configuration
- [ ] Consider distributed cache for cluster

### Index Optimizations
- [ ] Current indexes are in-memory maps - add persistence
- [ ] Implement B-tree or LSM-tree for large datasets
- [ ] Add index compaction
- [ ] Implement bloom filters for negative lookups

---

## 6. Persistence (Critical)

### BlockStore
- [ ] Replace in-memory map in `internal/blockstore` with disk storage
- [ ] Options: flat files, BadgerDB, or RocksDB
- [ ] Add write-ahead logging for BlockStore operations
- [ ] Implement storage compaction

### Index Persistence
- [ ] Port `internal/index` from in-memory maps to BadgerDB
- [ ] Implement ParentChild index persistence
- [ ] Implement Version index persistence
- [ ] Implement KeyToHash index persistence

### Cluster State
- [ ] Persist cluster node list in `internal/cluster`
- [ ] Add cluster configuration persistence
- [ ] Implement cluster state recovery on restart

---

## 7. Testing & Quality

### Integration Tests
- [ ] Add integration tests for distributed operations
- [ ] Test node join/leave scenarios
- [ ] Test network partition handling
- [ ] Test data recovery scenarios

### Security Tests
- [ ] Add security test suite
- [ ] Test TLS configuration
- [ ] Test access control enforcement
- [ ] Penetration testing for common vulnerabilities

### Performance Benchmarks
- [ ] Add benchmarks for block operations
- [ ] Add benchmarks for network operations
- [ ] Add benchmarks for encryption/decryption
- [ ] Establish performance baselines

### Code Review
- [ ] Review AI-generated code in `internal/carrier` (marked `// A`)
- [ ] Update annotations from `// A` to `// HC` after review
- [ ] Address all `// TODO` comments in codebase

---

## 8. Documentation

### API Documentation
- [ ] Document public interfaces in `pkg/`
- [ ] Add godoc comments to all exported types
- [ ] Create API reference documentation

### Operations Guide
- [ ] Document deployment procedures
- [ ] Create cluster setup guide
- [ ] Document backup/restore procedures
- [ ] Add troubleshooting guide

### Architecture Documentation
- [ ] Update architecture diagrams to reflect implementation
- [ ] Document data flow for key operations
- [ ] Create security architecture document

---

## Priority Order

1. **Security** - TLS validation, key wrapping, authentication
2. **Data Integrity** - WAL crash recovery, hash verification
3. **Persistence** - BlockStore and Index persistence
4. **Cluster Operations** - Complete stub implementations
5. **Performance** - Optimize identified bottlenecks
6. **Testing** - Comprehensive test coverage
7. **Documentation** - Complete documentation

---

## References

- Architecture Plan: `docs/diagrams/Architecture_plan.mmd`
- Current Implementation: `docs/diagrams/Architecture_current.mmd`
- Implementation Status: `ImplementationStatus.md`
