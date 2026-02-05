# Phase Overlap Audit (Revised)

**Scope:** quick scan for protocol versioned features that look like "separate phases" in code.

## Clarification

We treat the product as a single, continuously evolving system. Protocol versions (e.g., `*.v1`, `envelope.v2`) are wire-format versions, not product phases. Both can coexist in code without implying separate product phases or runtime modes.

## Notes

* Presence of `envelopev2`, `garlic`, `tunnel`, `netdb` does not mean separate phases. They are protocol/versioned components.
* We should avoid naming or config that implies separate phases unless there is a real runtime mode.

---

This doc is informational and should not be used to gate runtime behavior.
