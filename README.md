# Fulla

A local-first secret custodian for humans, agents, and independent machines.

Fulla is under implementation. It is not yet suitable for live credential
cutover. The first preview will be v0.1.0 after the documented acceptance gates.

One Go binary, exact-byte entries, explicit process injection, mutually pinned
peer synchronization, encrypted history, and separately protected recovery.
See [the product contract](docs/product-spec.md) and
[implementation plan](docs/implementation-plan.md).

## Ancestry and license

Fulla is an independent Agensfield descendant of [biox/pa](https://github.com/biox/pa)
and [Arda's safe-sync fork](https://github.com/ardasevinc/pa). It is not endorsed
by upstream. The transfer framing derives from fork commit
`f75734b8775f72d5d2f9630c08c2b48bdb6d8104`; original notices are preserved.
The combined work is licensed AGPL-3.0-or-later, see [LICENSE](LICENSE).

Fulla protects stored and transported secrets. Processes running as the owning
Unix user, explicitly selected editors/plugins, and injected child programs
are trusted. It does not isolate agents sharing that account.
