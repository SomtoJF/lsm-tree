# LSM

Database currently features $O(1)$ read and write performance, segmentation and compaction to minimize disk usage, and concurrency control to ensure thread safety.

This is a research/descovery project based on the key-value store implementation described in the book "Designing Data-Intensive Applications" by Martin Kleppmann and Michael Greim.

## Goal

The goal is to implement a very basic key-value store with the following features:

- [x] Segmentation and Compaction
- [ ] Crash Recovery
- [x] Concurrency Control

# Benchmark Data

View Benchmark Data [here](https://github.com/SomtoJF/lsm-tree/blob/main/BENCHMARKS.md)

## Usage

To run the program, simply:

- Clone the repo
- Build the program using `go build`
- Run the program using `./lsm-tree`
- Use the `help` command to see the available commands
