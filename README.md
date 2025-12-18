# OuroborosDB

[![Go Reference](https://pkg.go.dev/badge/github.com/i5heu/ouroboros-db.svg)](https://pkg.go.dev/github.com/i5heu/ouroboros-db) [![Tests](https://github.com/i5heu/OuroborosDB/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/i5heu/OuroborosDB/actions/workflows/go.yml) ![Coverage badge](https://github.com/i5heu/OuroborosDB/blob/gh-pages/.badges/main/coverage.svg) [![Go Report Card](https://goreportcard.com/badge/github.com/i5heu/ouroboros-db)](https://goreportcard.com/report/github.com/i5heu/ouroboros-db) [![wakatime](https://wakatime.com/badge/github/i5heu/ouroboros-db.svg)](https://wakatime.com/badge/github/i5heu/ouroboros-db)

<p align="center" style="margin: 2em;">
  <img width="400" height="400" style="border-radius: 3%; max-width: 100%" alt="Logo of OuroborosDB" src=".media/logo.jpeg">
</p>

⚠️ This Project is still in development and not ready for production use. ⚠️

- **OuroborosDB** is an in-progress **distributed, content-addressable database library** for Go. Data is addressed by hashes and organized in a **Git-like DAG of Blobs**, where each Blob can point to a `Parent` and form branches and merge nodes. Concurrent writes simply become new heads in the DAG that the caller can inspect and resolve.
- Data is stored as **Blobs → Chunks → SealedSlices**: blobs are versioned objects (Git-like DAG with parents/heads), chunks are deduplicated content blocks, and sealed slices are compressed, erasure-coded, and encrypted fragments distributed across nodes.
- The cluster is designed to be **stateless-ish and self-healing**: nodes host local KV stores, and a combination of a **SyncIndexTree** (Merkle-style sync) and a **DeletionWAL** (tombstone propagation) keeps replicas convergent without a global consensus log.
- OuroborosDB is exposed as a **single root Go package** (`import "github.com/i5heu/ouroborosdb"`) that can be embedded into other Go projects.

## Name and Logo

The name "OuroborosDB" is derived from the ancient symbol "Ouroboros," a representation of cyclical events, continuity, and endless return. Historically, it's been a potent symbol across various cultures, signifying the eternal cycle of life, death, and rebirth. In the context of this database, the Ouroboros symbolizes the perpetual preservation and renewal of data. While the traditional Ouroboros depicts a serpent consuming its tail, our version deviates, hinting at both reverence for historical cycles and the importance of continuous adaptation in the face of change.

## Development Notes

### Legend

#### Annotation legend for function comments:
To indicate the correctness and safety of the logic of functions, the following annotations are used in comments directly after the function definitions, at the same line as func (See examples below):

- `// A` - Function and was written by **AI** and was not reviewed by a **human**.
- `// AP` - Function was written by **AI** and was reviewed but the **human** has found a potential issue which the **human** marked with a `// TODO ` comment.
- `// AC` - Function was written by **AI** and was reviewed and approved by a **human** that has medium confidence in the correctness and safety of the logic.
- `// H` - Function was written by a **human**
- `// HP` - Function was written by a **human** but the **human** has found a potential issue which the **human** marked with a `// TODO ` comment.
- `// HC` - A **human** comprehended the logic of th function in all its dimensions and is confident about its correctness and safety.

If the function has a higher risk profile (e.g., involves complex algorithms, security-sensitive operations, or critical data handling), a `P` prefix is added for `Priority`:

**All `P` function must be brought to `PHC` status before a production release.**

We add the indicators directly after the function declaration, although it is normally not common practice in Go, because it makes it easier to see the status of the function for most editors as they show use sticky function declaration.

It is negotiable that AI generated functions must be generated with an `// A` or `// AP` annotation after the function declaration `func exampleFunction() { // A`.

Examples:  
```go

// This function does X, Y, and Z.
func exampleFunction() { // A
    // Function is low risk and was written by AI and not reviewed by a human.
}

// This function does X, Y, and Z.
func exampleFunction() { // HC
    // Function is low risk and was comprehended by a human who is confident about its correctness and safety.
}

// This function performs critical operations X, Y, and Z has some funky stuff going on.
func criticalFunction() { // PAP
    // Function is high risk and was comprehended by a human who is confident about its correctness and safety.
}

// This function performs critical operations X, Y, and Z.
func criticalFunction() { // PHC
    // Function is high risk and was comprehended by a human who is confident about its correctness and safety.
}

// If a function has multiline parameters, the annotation goes at the same line as func
func manyParametersFunction( // AC
    param1 string, 
    param2 int, 
    param3 []byte
) error { 
    // function has many parameters, was written by AI and reviewed and approved by a human.
    return nil
}
```

### Architecture

This UML uses pseudo classes to illustrate the main components and their relationships in OuroborosDB.

```mermaid

classDiagram
    class Cluster {
        -[]Node Nodes
    }

    namespace DBNode {
  

        class Node {
            +string NodeID
            +[]string Addresses
            + index IndexModel
        }

        class ClusterController {
        }

        class ClusterMonitor {
            +MonitorNodeHealth()
        }

        class NodeAvailabilityTracker {
            +TrackAvailability()
        }

        class BootStrapper {
            +BootstrapNode(node Node) error
        }

        class MessageTunnel {
            +Broadcast(message Message)
            +SendMessageToNode(nodeID NodeID, message Message)
        }

        class MessageAuthenticator {
            +AuthenticateMessage(message Message) bool
        }

        class DataRouter {
            +StoreBlob(blob Blob) (hash.Hash, error)
            +RetrieveBlob(hash.Hash) (Blob, error)
            +DeleteBlob(hash.Hash) error
            +SetSealedSlice(SealedSliceWithPayload) error
        }

        class CAS {
            +StoreBlob(blob Blob) (hash.Hash, error)
            +GetBlob(hash.Hash) (Blob, error)
            +DeleteBlob(hash.Hash) error
        }

        class LocalSealedSliceStore {
            +Store(sealedSlice SealedSliceWithPayload, HashOfPubKey hash.Hash) error
            +Get(hash.Hash) (SealedSlice, error)
            +Delete(hash.Hash) error
            +ChunkListSealedSlices(chunkHash.Hash) (sealedSliceHashes []hash.Hash, error)
            +ChunkListSealedSlicesForPubKey(hash.Hash, HashOfPubKey hash.Hash) (sealedSliceHashes []hash.Hash, error)
        }

        class DeletionWAL {
            +LogDeletion(hash.Hash) error
            +ProcessDeletions() error
        }

        class DistributedIndex 

        class HashToNode {
            +GetNodeForHash(hash.Hash) Node
        }

        class KeyToHashAndNode {
            +GetHashAndNodeForKey(string) (hash.Hash, Node, error)
        }

        class DataReBalancer {
            +BalanceData()
        }

        class ReplicationMonitoring {
            +MonitorReplications()
        }

        class SyncIndexTree {
            +Sync()
        }

        class BackupManager {
            +BackupData()
        }
    }

    Cluster "1" *-- "*" Node : contains
    Node "1" o-- "*" ClusterController : listensOn
    ClusterController "1" *-- "1" MessageTunnel : communicatesVia
    Node "1" o-- "1" CAS : persists blobs
    CAS "1" o-- "1" DataRouter : delegates persistence to
    DataRouter "1" *-- "1" LocalSealedSliceStore : uses
    ClusterController "1" *-- "1" DistributedIndex : LooksUps
    DistributedIndex "1" *-- "1" HashToNode : used for mapping
    DistributedIndex "1" *-- "1" KeyToHashAndNode : lookups for keys
    ClusterController "1" *-- "1" DataReBalancer : manages
    DataReBalancer "1" *-- "1" ReplicationMonitoring : utilizes
    MessageTunnel "1" *-- "1" MessageAuthenticator : secures
    DataRouter "1" *-- "1" ClusterController : interacts with
    ClusterController "1" *-- "1" BootStrapper : initializes
    ClusterController "1" *-- "1" ClusterMonitor : monitors
    Node "1" *-- "1" BackupManager : manages backups
    ClusterMonitor "1" *-- "1" NodeAvailabilityTracker : utilizes
    DataReBalancer "1" *-- "1" SyncIndexTree : utilizes
    Node "1" *-- "1" DeletionWAL : logs deletions


    namespace IndexModel {
        class Index {
            -LocalIndexStore store
        }
        class parentChildIndex {
            - map<hash.Hash, []hash.Hash> ParentToChildren
            - map<hash.Hash, hash.Hash> ChildToParent
        }
        class VersionIndex {
            - map<hash.Hash, []hash.Hash> VersionVectorHeads
        }
        class KeyToHashIndex {
            - map<string, hash.Hash> KeyToHash
        }
    }

    Node "1" o-- "1" Index : indexes relations and metadata
    Index "1" o-- "1" parentChildIndex : manages
    Index "1" o-- "1" VersionIndex : manages

    namespace DataModel {
        class Blob {
            +string Key
            +hash.Hash Hash
            +hash.Hash Parent
            +int64 Created
            +enum Type "Child, VersionVector, Merger"
            <<virtual>> Content
            -[]hash.Hash ChunkHashes
            +Get() []byte
        }


        class Chunk {
            +hash.Hash ChunkHash
            <<virtual>> ChunkData
            -[]hash.Hash SealedSliceHashes
            +Get() []byte
        }

        class SealedSlice {
            +hash.Hash SliceHash
            -[]byte Nonce
            +Get() []byte
        }

        class SealedSliceWithPayload {
            +SealedSlice SealedSlice
            +[]byte SealedPayload
        }

        class KeyIndex {
            -[SliceHash][HashOfPubKey] []EncapsulatedKey
        }
    }

    LocalSealedSliceStore "1" o-- "*" Blob : stores
    Blob "1" *-- "*" Chunk : materializes Content
    Chunk "1" *-- "*" SealedSlice : materializes ChunkData
    SealedSliceWithPayload "1" *-- "1" SealedSlice : wraps
    SealedSlice "1" *-- "*" KeyIndex : provides Key
    note for SealedSlice "
Blob, Chunk and SealedSlice are all persisted
as key/value entries in LocalSealedSliceStores on one
or more Nodes in the Cluster.
If a SealedSlice is stored on a Node it does not
imply that Node has the entire Blob or Chunk."

```

## License
ouroboros-db © 2025 Mia Heidenstedt and contributors   
SPDX-License-Identifier: AGPL-3.0  