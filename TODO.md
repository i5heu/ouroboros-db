## Open Questions

### Handling Small Data
Small data is often smaller then the hashes of the reed-solomon slices.

#### Solution:

- Encrypt Chunks instead of RS-Slices
- Combine Chunks to a Block of 16MB
- Apply Reed-Solomon on the Block Level
- Block must be chunk readable (byte range readable) so we can extract chunks without decoding the whole block.
- We need to collect chunks in a WAL until we have enough to form a block.
- The ChunkMetadata (which chunks belong to which original data), must be also stored in the WAL and block in a extra section since one chunk can belong to multiple original data pieces.
- The Events also must be stored in the WAL and block.
- The chunks must be stored with the encryption pub key group.
- The encrypted AES-keys must be stored in the block.

### Inter Node Connection
Something QUIC

## Answered Questions

### Node Permission Levels:
0. Unauthenticated Node: Can do nothing except AuthenticationRequest
1. Authenticated Node: Can request SealedSlices and Metas
2. Trusted Node: Can Request Everything