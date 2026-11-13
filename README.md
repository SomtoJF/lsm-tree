# LSM

Database currently features $O(1)$ indexed reads, append-only writes, segmentation and compaction, crash recovery, and concurrency control.

This is a research/descovery project based on the key-value store implementation described in the book "Designing Data-Intensive Applications" by Martin Kleppmann and Michael Greim.

## Goal

The goal is to implement a very basic key-value store with the following features:

- [x] Segmentation and Compaction
- [x] Crash Recovery
- [x] Startup recovery and index rebuilding
- [x] Checksummed, versioned records
- [x] WAL-like durable group commits
- [x] Concurrency Control

## Durability and Recovery

### Crash recovery

Crash recovery detects incomplete or corrupted records after an unexpected shutdown. Valid records remain available, while a damaged final record is discarded or truncated.

### Startup recovery and index rebuilding

On startup, the active file and binary segment hints are scanned to rebuild the in-memory key indexes. This restores lookup state without requiring a separate persisted index.

### Checksummed, versioned records

Each record stores a format version and CRC32 checksum alongside its key and value. The version identifies the frame layout, while the checksum detects partial or corrupted writes.

### WAL-like durable group commits

Writes first enter an in-memory queue and are grouped for up to 5 ms. The batch is written to the append-only active file and synced before acknowledgements are returned. This limits the number of expensive `fsync` calls.

### Manifest-based compaction recovery

Compaction writes segment data and hints to temporary files, syncs them, and then publishes their names through a manifest. On startup, the manifest identifies the complete segment set to use.

# Benchmark Data

View Benchmark Data [here](https://github.com/SomtoJF/lsm-tree/blob/main/BENCHMARKS.md)

## Usage

To run the program, simply:

- Clone the repo
- Build the program using `go build`
- Run the program using `./lsm-tree`
- Use the `help` command to see the available commands
