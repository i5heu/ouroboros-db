# TODO implementation tasks

- [x] Check if Out and Ingoing connections are verified - Bilateral verification - So if a node initiates a connection, it should verify the peer's certificate, and if a node receives an incoming connection, it should also verify the peer's certificate. This ensures that both parties in the communication are authenticated and trusted
- [x] Verify that the handshake is not leaking any private key in and outgoing
- [x] Verify that the TLS cert is bind to the ML-DSA-87 identity
- [x] Rename `JoinCluster` to `OpenPeerChannel`
- [x] Verify Authentication with QUIC Streams and Datagrams - Ensure that Auth is always performed over reliable QUIC streams and Datagrams just pigigyback on the existing TLS connection
- [ ] Heartbeat
- [ ] NodeStats

## Transport Layer

- [x] Implement `Carrier` - QUIC-based cluster transport with reliable/unreliable
      delivery
- [x] Implement `QuicTransport` - QUIC implementation with streams + datagrams
- [x] Implement `Connection` - Single QUIC connection to a peer
- [x] Implement `Stream` - Reliable QUIC stream
- [x] Implement `NodeRegistry` - Tracks all known nodes with certificates
- [ ] Implement `NodeSync` - Periodic full sync of node registry

## Auth Layer

- [x] Implement `CarrierAuth` - Done: `pkg/auth/carrier_auth.go`
- [x] Implement `AdminCA` - Done: `pkg/auth/admin_ca.go`
- [x] Implement `UserCA` - Done: `pkg/auth/user_ca.go`
- [x] Implement `NodeCert` - Done: `pkg/auth/node_cert.go`
- [x] Implement `DelegationProof` - Done: `pkg/auth/delegation_proof.go`

## Control Layer

- [x] Implement `ClusterController` - Done:
      `internal/cluster/cluster_controller.go`
- [ ] Implement `ClusterMonitor` - Node health, logs, data state, stats
- [ ] Implement `DataState` - Maps data to nodes with status
- [ ] Implement `NodeAvailabilityTracker` - Track node availability
- [ ] Complete `Vertex.GetContent()` - Currently stub returning nil
- [ ] Evaluate if `Cluster` needs methods beyond data container

## Data Layer

- [ ] Implement `CAS` - Content Addressable Storage main management layer
- [ ] Implement `DataRouter` - Routes data operations across cluster
- [ ] Implement `BlockStore` - Low-level Block and BlockSlice persistence
- [ ] Implement `EncryptionService` - Chunk <-> SealedChunk transformation
- [ ] Implement `DistributedWAL` - Intake buffer aggregating items until Block
      size
- [ ] Implement `BlockDistributionTracker` - Tracks block distribution and
      confirmations
- [ ] Implement `DeletionWAL` - Logs and processes deletions

## Index Layer

- [ ] Implement `Index` - Local index with parent-child, version, key-to-hash
- [ ] Implement `parentChildIndex` - Parent-child DAG relations
- [ ] Implement `VersionIndex` - Version vector heads tracking
- [ ] Implement `KeyToHashIndex` - String key to hash mapping
- [ ] Implement `DistributedIndex` - Cluster-wide index lookup
- [ ] Implement `HashToNode` - Maps hash to node
- [ ] Implement `KeyToHashAndNode` - Maps key to hash and node

## Rebalancing / Backup

- [ ] Implement `DataReBalancer` - Data rebalancing across nodes
- [ ] Implement `ReplicationMonitoring` - Monitor replication status
- [ ] Implement `SyncIndexTree` - Sync index trees
- [ ] Implement `BackupManager` - Backup management

## Logging

- [x] Implement `ClusterLog` - Done: `pkg/clusterlog/cluster_log.go`
- [x] Implement `LogEntry` - Done: `pkg/clusterlog/log_entry.go`
- [x] Implement `LogLevel` - Done: `pkg/clusterlog/log_level.go`


# TODO before release tasks

- [ ] Security audit of data encryption and key management
- [ ] Security audit of auth and transport layers
- [ ] Hardening of auth and transport against logical attacks
- [ ] Hardening of transport against DoS attacks
- [ ] Security review of user or public data handling code
- [ ] Identify and document security weaknesses