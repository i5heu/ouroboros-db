# OuroborosDB

[![Go Reference](https://pkg.go.dev/badge/github.com/i5heu/ouroboros-db.svg)](https://pkg.go.dev/github.com/i5heu/ouroboros-db) [![Tests](https://github.com/i5heu/OuroborosDB/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/i5heu/OuroborosDB/actions/workflows/go.yml) ![Coverage badge](https://github.com/i5heu/OuroborosDB/blob/gh-pages/.badges/main/coverage.svg) [![Go Report Card](https://goreportcard.com/badge/github.com/i5heu/ouroboros-db)](https://goreportcard.com/report/github.com/i5heu/ouroboros-db) [![wakatime](https://wakatime.com/badge/github/i5heu/ouroboros-db.svg)](https://wakatime.com/badge/github/i5heu/ouroboros-db)

<p align="center" style="margin: 2em;">
  <img width="400" height="400" style="border-radius: 3%; max-width: 100%" alt="Logo of OuroborosDB" src=".media/logo.jpeg">
</p>

⚠️ This Project is still in development and not ready for production use. ⚠️


## Name and Logo

The name "OuroborosDB" is derived from the ancient symbol "Ouroboros," a representation of cyclical events, continuity, and endless return. Historically, it's been a potent symbol across various cultures, signifying the eternal cycle of life, death, and rebirth. In the context of this database, the Ouroboros symbolizes the perpetual preservation and renewal of data. While the traditional Ouroboros depicts a serpent consuming its tail, our version deviates, hinting at both reverence for historical cycles and the importance of continuous adaptation in the face of change.


## Architecture Overview

### Data Model: Content Pipeline

The core data flow follows a transformation pipeline from cleartext to distributed shards:

```
Chunk (cleartext)
  ↓ [encryption via EncryptionService]
SealedChunk (encrypted with SealedHash for integrity)
  ↓ [buffering in DistributedWAL]
DistributedWAL (intake buffer, aggregates until ~16MB)
  ↓ [batching via SealBlock()]
Block (DataSection + VertexSection + KeyRegistry + Indexes)
  ↓ [Reed-Solomon erasure coding]
BlockSlice (physical shard distributed to cluster nodes)
```

**Key Data Structures**:

- **Vertex** (formerly "Blob"): DAG node with metadata, parent references, and ChunkHash pointers (stored UNENCRYPTED in Block's VertexSection)
- **Chunk**: Temporary cleartext content with hash and size
- **SealedChunk**: Encrypted content with Nonce, OriginalSize, and SealedHash (integrity check without decryption)
- **Block**: Central archive unit (~16MB) containing:
  - BlockHeader (metadata, Reed-Solomon params)
  - DataSection (encrypted SealedChunks)
  - VertexSection (unencrypted Vertices for DAG structure)
  - ChunkRegion Index (ChunkHash → byte offset/length)
  - VertexRegion Index (VertexHash → byte offset/length)
  - KeyRegistry (map[Hash][]KeyEntry for access control)
- **KeyEntry**: Per-user encryption key wrapper linking pubKey to chunkHash
- **BlockSlice**: RS-encoded shard with reconstruction params, distributed across nodes


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

## License
ouroboros-db © 2026 Mia Heidenstedt and contributors   
SPDX-License-Identifier: AGPL-3.0  