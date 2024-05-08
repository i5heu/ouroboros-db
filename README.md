 [![Go Reference](https://pkg.go.dev/badge/github.com/i5heu/ouroboros-db/pkg/api.svg)](https://pkg.go.dev/github.com/i5heu/ouroboros-db/pkg/api) [![Tests](https://github.com/i5heu/OuroborosDB/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/i5heu/OuroborosDB/actions/workflows/go.yml) ![](https://github.com/i5heu/OuroborosDB/blob/gh-pages/.badges/main/coverage.svg) [![Go Report Card](https://goreportcard.com/badge/github.com/i5heu/ouroboros-db)](https://goreportcard.com/report/github.com/i5heu/ouroboros-db) [![wakatime](https://wakatime.com/badge/github/i5heu/ouroboros-db.svg)](https://wakatime.com/badge/github/i5heu/ouroboros-db)
 
<p align="center">
  <img src=".media/logo.jpeg"  width="300">
</p>

# OuroborosDB
A embedded database built around the concept of event trees, emphasizing data deduplication and data integrity checks. By structuring data into event trees, OuroborosDB ensures efficient and intuitive data management. Key features include:

- Data Deduplication: Eliminates redundant data through efficient chunking and hashing mechanisms.
- Data Integrity Checks: Uses SHA-512 hashes to verify the integrity of stored data.
- Event-Based Architecture: Organizes data hierarchically for easy retrieval and management.
- Scalable Concurrent Processing: Optimized for concurrent processing to handle large-scale data.
- Log Management and Indexing: Provides efficient logging and indexing for performance monitoring.
- Non-Deletable Events: Once stored, events cannot be deleted or altered, ensuring the immutability and auditability of the data.
- (To be implemented) Temporary Events: Allows the creation of temporary events that can be marked as temporary and safely cleaned up later for short-term data storage needs.


## Table of Contents

- [OuroborosDB](#ouroborosdb)
  - [Table of Contents](#table-of-contents)
  - [Installation](#installation)
  - [Usage](#usage)
    - [Initialization](#initialization)
    - [Storing Files](#storing-files)
    - [Retrieving Files](#retrieving-files)
    - [Event Tree Management](#event-tree-management)
      - [Creating Root Event](#creating-root-event)
      - [Fetching Root Events by Title](#fetching-root-events-by-title)
      - [Creating Child Events](#creating-child-events)
      - [Fetching Child Events](#fetching-child-events)
  - [Testing](#testing)
  - [Benchmarking](#benchmarking)
    - [Benchmark current state of the codebase](#benchmark-current-state-of-the-codebase)
    - [Benchmark Versions](#benchmark-versions)
  - [OuroborosDB Performance Version Differences](#ouroborosdb-performance-version-differences)
  - [OuroborosDB Performance Changelog](#ouroborosdb-performance-changelog)
  - [Name and Logo](#name-and-logo)
  - [License](#license)

## Installation

OuroborosDB requires Go 1.21.5+

```bash
go get -u github.com/i5heu/OuroborosDB
```

## Usage

### Initialization

**OuroborosDB** can be initialized with a configuration struct that includes paths for storage and other settings.

```go
import "OuroborosDB"

func initializeDB() *OuroborosDB.OuroborosDB {
    db, err := OuroborosDB.NewOuroborosDB(OuroborosDB.Config{
        Paths:                     []string{"./data/storage"},
        MinimumFreeGB:             1,
        GarbageCollectionInterval: 10, // Minutes
    })
    if err != nil {
        log.Fatalf("Failed to initialize OuroborosDB: %v", err)
    }
    return db
}
```

### Storing Files

Files can be stored within events using the `StoreFile` method.

```go
import (
    "OuroborosDB/internal/storage"
    "OuroborosDB"
)

func storeFile(db *OuroborosDB.OuroborosDB, parentEvent storage.Event) storage.Event {
    fileContent := []byte("This is a sample file content")
    metadata := []byte("sample.txt")

    event, err := db.DB.StoreFile(storage.StoreFileOptions{
        EventToAppendTo: parentEvent,
        Metadata:        metadata,
        File:            fileContent,
    })
    if err != nil {
        log.Fatalf("Failed to store file: %v", err)
    }
    return event
}
```

### Retrieving Files

Files can be retrieved by providing the event from which they were stored.

```go
func retrieveFile(db *OuroborosDB.OuroborosDB, event storage.Event) []byte {
    content, err := db.DB.GetFile(event)
    if err != nil {
        log.Fatalf("Failed to retrieve file: %v", err)
    }
    return content
}
```

### Event Tree Management

#### Creating Root Event

Create a root event to represent the top level of an event tree.

```go
func createRootEvent(db *OuroborosDB.OuroborosDB, title string) storage.Event {
    rootEvent, err := db.DB.CreateRootEvent(title)
    if err != nil {
        log.Fatalf("Failed to create root event: %v", err)
    }
    return rootEvent
}
```

#### Fetching Root Events by Title

```go
func getRootEventsByTitle(db *OuroborosDB.OuroborosDB, title string) []storage.Event {
    events, err := db.DB.GetRootEventsWithTitle(title)
    if err != nil {
        log.Fatalf("Failed to fetch root events by title: %v", err)
    }
    return events
}
```

#### Creating Child Events

```go
func createChildEvent(db *OuroborosDB.OuroborosDB, parentEvent storage.Event) storage.Event {
    childEvent, err := db.DB.CreateNewEvent(storage.EventOptions{
        HashOfParentEvent: parentEvent.EventHash,
    })
    if err != nil {
        log.Fatalf("Failed to create child event: %v", err)
    }
    return childEvent
}
```

#### Fetching Child Events

```go
func getChildEvents(db *OuroborosDB.OuroborosDB, parentEvent storage.Event) []storage.Event {
    children, err := db.Index.GetDirectChildrenOfEvent(parentEvent.EventHash)
    if err != nil {
        log.Fatalf("Failed to fetch child events: %v", err)
    }
    return children
}
```

## Testing
  
```bash
  go test ./...
```

## Benchmarking
### Benchmark current state of the codebase
```bash
  go test -run='^$' -bench=.
```
### Benchmark Versions 
Works with uncommitted changes and version/commits that are reachable by `git checkout`.  
You also need to have installed `benchstat` to compare the benchmarks, install it with `go get golang.org/x/perf/cmd/benchstat@latest`

```bash
  # add versions to bench.sh
  bash bench.sh
  # Now look in benchmarks/combined_benchmarks_comparison to see the results
```
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

## License
OuroborosDB (c) 2024 Mia Heidenstedt and contributors  
   
SPDX-License-Identifier: AGPL-3.0
