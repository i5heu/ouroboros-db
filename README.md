# OuroborosDB

[![Go Reference](https://pkg.go.dev/badge/github.com/i5heu/ouroboros-db.svg)](https://pkg.go.dev/github.com/i5heu/ouroboros-db) [![Tests](https://github.com/i5heu/OuroborosDB/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/i5heu/OuroborosDB/actions/workflows/go.yml) ![Coverage badge](https://github.com/i5heu/OuroborosDB/blob/gh-pages/.badges/main/coverage.svg) [![Go Report Card](https://goreportcard.com/badge/github.com/i5heu/ouroboros-db)](https://goreportcard.com/report/github.com/i5heu/ouroboros-db) [![wakatime](https://wakatime.com/badge/github/i5heu/ouroboros-db.svg)](https://wakatime.com/badge/github/i5heu/ouroboros-db)

![OuroborosDB logo](.media/logo.jpeg)

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
