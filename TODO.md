# TODO

## Renaming / Cleanup

- [ ] Remove duplicate `LogLevel` from `pkg/interfaces/monitor.go` - use
      `pkg/clusterlog.LogLevel`
- [ ] Remove duplicate `LogEntry` from `pkg/interfaces/monitor.go` - use
      `pkg/clusterlog.LogEntry`
- [ ] Implement `BootstrapConfig.LoadFromFile()` and `SaveToFile()` (currently
      stubs)

## Transport Layer

- [ ] Implement `Carrier` - QUIC-based cluster transport with reliable/unreliable
      delivery
- [ ] Implement `QuicTransport` - QUIC implementation with streams + datagrams
- [ ] Implement `Connection` - Single QUIC connection to a peer
- [ ] Implement `Stream` - Reliable QUIC stream
- [ ] Implement `NodeRegistry` - Tracks all known nodes with certificates
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
