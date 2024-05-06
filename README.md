<p align="center">
  <img src=".media/logo.jpeg"  width="300">
</p>

# OuroborosDB

A versioned, distributed key-value store designed for data integrity against bit rot. Uses CRC for added data verification. Offers seamless merging, detailed history with a chain of hashes, and an easy-to-use private-public key security mechanism. Compatible in browsers through WebAssembly (WASM) and on Linux/Docker environments.


## OuroborosDB Development TODO List (has to change)


## OuroborosDB Performance Version Differences
```bash
goos: linux
goarch: arm64
pkg: OuroborosDB
                                                           │ ./benchmarks/v0.0.2 │         ./benchmarks/v0.0.3         │
                                                           │       sec/op        │    sec/op     vs base               │
_Index_RebuildingIndex/RebuildIndex-8                              397.79m ±  2%   19.40m ±  2%  -95.12% (p=0.002 n=6)
_Index_GetDirectChildrenOfEvent/GetChildrenOfEvent-8               35.559µ ±  3%   4.727µ ±  3%  -86.71% (p=0.002 n=6)
_Index_GetChildrenHashesOfEvent/GetChildrenHashesOfEvent-8          77.27n ±  2%   75.79n ±  1%   -1.91% (p=0.004 n=6)
_DB_StoreFile/StoreFile-8                                           311.7µ ±  4%   188.4µ ±  4%  -39.57% (p=0.002 n=6)
_DB_GetFile/GetFile-8                                               3.695µ ±  4%   3.422µ ±  4%   -7.40% (p=0.004 n=6)
_DB_GetEvent/GetEvent-8                                            45.636µ ±  2%   6.220µ ±  2%  -86.37% (p=0.002 n=6)
_DB_GetMetadata/GetMetadata-8                                       4.040µ ±  4%   3.972µ ±  9%        ~ (p=0.132 n=6)
_DB_GetAllRootEvents/GetAllRootEvents-8                            121.18m ±  5%   19.46m ±  2%  -83.94% (p=0.002 n=6)
_DB_GetRootIndex/GetRootIndex-8                                     2.465m ±  5%   2.477m ±  3%        ~ (p=1.000 n=6)
_DB_GetRootEventsWithTitle/GetRootEventsWithTitle-8                 53.55µ ±  2%   12.24µ ±  3%  -77.13% (p=0.002 n=6)
_DB_CreateRootEvent/CreateRootEvent-8                               158.1µ ± 11%   129.1µ ± 11%  -18.34% (p=0.002 n=6)
_DB_CreateNewEvent/CreateNewEvent-8                                135.91µ ±  6%   49.64µ ± 15%  -63.48% (p=0.002 n=6)
geomean                                                             144.0µ         52.30µ        -63.69%
```

## OuroborosDB Performance Changelog

- **v0.0.3** - Switch from `gob` to `protobuf` for serialization
- **v0.0.2** - Create tests and benchmarks


## Name and Logo

The name "OuroborosDB" is derived from the ancient symbol "Ouroboros," a representation of cyclical events, continuity, and endless return. Historically, it's been a potent symbol across various cultures, signifying the eternal cycle of life, death, and rebirth. In the context of this database, the Ouroboros symbolizes the perpetual preservation and renewal of data. While the traditional Ouroboros depicts a serpent consuming its tail, our version deviates, hinting at both reverence for historical cycles and the importance of continuous adaptation in the face of modern data challenges.

