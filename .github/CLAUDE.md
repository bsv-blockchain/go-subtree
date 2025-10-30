# CLAUDE.md

## 📦 Project Summary

**go-subtree** is a high-performance Go library for managing Bitcoin SV (BSV) merkle tree subtree structures. It provides efficient tools for organizing transactions hierarchically, essential for block assembly, transaction validation, and merkle proof generation in BSV blockchain infrastructure.

### Core Capabilities
- **Subtree Management**: Create and manipulate merkle trees with dynamic node addition/removal
- **Merkle Operations**: Generate merkle proofs, calculate roots, handle incomplete trees
- **Serialization**: Binary format support for storage and transmission with streaming deserialization
- **Transaction Tracking**: Link full transaction data to tree nodes, track inpoints and conflicts
- **Performance**: Zero runtime dependencies, allocation-free operations, parallel tree computation

### Target Use Cases
Bitcoin block construction, SPV proof generation, teranode transaction organization.

## 🤖 Welcome, Claude

This repository uses **`AGENTS.md`** as the single source of truth for:

* Coding conventions (naming, formatting, commenting, testing)
* Contribution workflows (branch prefixes, commit message style, PR templates)
* Release, CI, and dependency‑management policies
* Security reporting and governance links

> **TL;DR:** **Read `AGENTS.md` first.**
> All technical or procedural questions are answered there.

### Quick Checklist for Claude

1. **Study `AGENTS.md`**
   Make sure every automated change or suggestion respects those rules.
2. **Follow branch‑prefix and commit‑message standards**
   They drive Mergify auto‑labeling and CI gates.
3. **Never tag releases**
4. **Pass CI**
   Run `go fmt`, `goimports`, `go vet`, `staticcheck`, and `golangci‑lint` locally before opening a PR.

If you encounter conflicting guidance elsewhere, `AGENTS.md` wins.
Questions or ambiguities? Open a discussion or ping a maintainer instead of guessing.

Happy hacking!
