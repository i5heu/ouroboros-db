# OuroborosDB

[![Go Reference](https://pkg.go.dev/badge/github.com/i5heu/ouroboros-db.svg)](https://pkg.go.dev/github.com/i5heu/ouroboros-db) [![Tests](https://github.com/i5heu/OuroborosDB/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/i5heu/OuroborosDB/actions/workflows/go.yml) ![Coverage badge](https://github.com/i5heu/OuroborosDB/blob/gh-pages/.badges/main/coverage.svg) [![Go Report Card](https://goreportcard.com/badge/github.com/i5heu/ouroboros-db)](https://goreportcard.com/report/github.com/i5heu/ouroboros-db) [![wakatime](https://wakatime.com/badge/github/i5heu/ouroboros-db.svg)](https://wakatime.com/badge/github/i5heu/ouroboros-db)

<p align="center" style="margin: 2em;">
  <img width="400" height="400" style="border-radius: 3%; max-width: 100%" alt="Logo of OuroborosDB" src=".media/logo.jpeg">
</p>

⚠️ This Project is still in development and not ready for production use. ⚠️

## Name and Logo

The name "OuroborosDB" is derived from the ancient symbol "Ouroboros," a representation of cyclical events, continuity, and endless return. Historically, it's been a potent symbol across various cultures, signifying the eternal cycle of life, death, and rebirth. In the context of this database, the Ouroboros symbolizes the perpetual preservation and renewal of data. While the traditional Ouroboros depicts a serpent consuming its tail, our version deviates, hinting at both reverence for historical cycles and the importance of continuous adaptation in the face of change.

## License

OuroborosDB (c) 2024 Mia Heidenstedt and contributors  
 
SPDX-License-Identifier: AGPL-3.0


## HTTP API

The reference server in `cmd/server` now exposes a lightweight HTTP API on port 8083. Start the binary and the REST endpoints below become available.

### `POST /data`

Create a new stored object. The body must be JSON with the payload content encoded as base64. Reed-Solomon parameters default to 4 data shards and 2 parity shards when omitted. Provide an optional `mime_type` (<=255 bytes) to mark binary data; absence of a MIME type treats the payload as UTF-8 text.

```json
{
  "content": "aGVsbG8gd29ybGQ=",
  "reed_solomon_shards": 4,
  "reed_solomon_parity_shards": 2,
  "mime_type": "application/octet-stream"
}
```

Successful requests respond with the SHA-512 key of the stored data:

```json
{
  "key": "<hex-encoded-hash>"
}
```

### `GET /data`

List all stored keys returned as hexadecimal-encoded hashes:

```json
{
  "keys": ["<hex-encoded-hash>" ]
}
```

### Try it

```bash
curl -X POST http://localhost:8083/data \
  -H 'Content-Type: application/json' \
  -d '{"content":"aGVsbG8=","reed_solomon_shards":4,"reed_solomon_parity_shards":2}'

curl http://localhost:8083/data
```

### `GET /data/{key}`

Retrieve a stored entry by its hexadecimal hash. The response includes a base64 encoded payload, the inferred text flag, and the MIME type (if supplied when writing).

```json
{
  "key": "<hex-encoded-hash>",
  "content": "aGVsbG8gd29ybGQ=",
  "is_text": true,
  "mime_type": ""
}
```

### Payload header format

Stored objects reserve the first 256 bytes for metadata:

- Byte `0`: the most significant bit (MSB) marks text payloads. When set (`1`), the payload is treated as UTF-8 text and no MIME is stored.
- Bytes `1-255`: when the MSB is `0`, these bytes contain an ASCII/UTF-8 MIME type string padded with zeros. Empty or whitespace-only MIME values fall back to text handling.
- Remaining bytes: the original payload content.

This layout allows fast retrieval of text/binary hints without additional metadata calls.


## Development Notes

### Legend

#### Annotation legend for function comments:
To indicate the correctness and safety of the logic of functions, the following annotations are used in comments directly after the function definitions (See examples below):

- `// A` - Function and was written by **AI** and was not reviewed by a **human**.
- `// AP` - Function was written by **AI** and was reviewed but the **human** has found a potential issue which the **human** marked with a `// TODO ` comment.
- `// AC` - Function was written by **AI** and was reviewed and approved by a **human** that has medium confidence in the correctness and safety of the logic.
- `// H` - Function was written by a **human**
- `// HP` - Function was written by a **human** but the **human** has found a potential issue which the **human** marked with a `// TODO ` comment.
- `// HC` - A **human** comprehended the logic of th function in all its dimensions and is confident about its correctness and safety.

If the function has a higher risk profile (e.g., involves complex algorithms, security-sensitive operations, or critical data handling), a `P` prefix is added for `Priority`:

**All `P` function must be brought to `PHC` status before a production release.**

We add the indicators directly after the function declaration, although it is normally not common practice in Go, because it makes it easier to see the status of the function for most editors as they show use sticky function declaration.

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
```
## Wikipedia content used by mockData

The `cmd/mockData` tool does fetch and store small excerpts from Wikipedia for testing/demos. Wikipedia content is licensed under CC BY-SA 3.0 and GFDL: please follow those licenses (provide attribution, links to original articles, and indicate changes) if you reuse or redistribute any fetched Wikipedia content. See the canonical license pages for details:

- https://en.wikipedia.org/wiki/Wikipedia:Copyrights


## License
ouroboros-db © 2025 Mia Heidenstedt and contributors   
SPDX-License-Identifier: AGPL-3.0  