## 2025-03-09 - Fast-path prefix check & allocation elimination in buffer line sanitizer
**Learning:** In terminal buffer capture pipelines (e.g. `capture-pane`), lines are repeatedly sanitized every tick/render. Calling `regexp.FindStringIndex` on every rune iteration creates massive overhead (~10µs/line). Checking `strings.HasPrefix(s, "\x1b[")` before regex matching and slicing a constant `"        "[:n]` instead of `strings.Repeat` reduced per-line processing time by ~70% (10µs -> 3µs) and cut allocations significantly (708 B/op -> 362 B/op).
**Action:** When inspecting string processing in frequent TUI tick/render routines, add fast-path prefix/substring checks before regular expressions and replace repeated string construction with constant string slicing.

## 2025-03-10 - Fast-path ANSI check and cell width caching in CLI table layout
**Learning:** `renderTable` calculated `visibleWidth` twice for every cell (width calculation + row rendering), executing regex replacements on every string. Checking `!strings.Contains(s, "\x1b")` before regex replacement eliminated regex execution and heap allocations for uncolored strings (35.7ns vs 262.8ns, 0 B vs 80 B). Pre-calculating visible widths into a flat slice and slicing constant spaces for padding reduced `renderTable` execution time by ~62% (14.0µs -> 5.2µs) and allocations by ~74% (132 -> 34 allocs).
**Action:** Fast-path string operations without ANSI codes with `strings.Contains(s, "\x1b")` and cache cell layout metadata when performing multi-pass text formatting.

## 2025-03-11 - Flat slice preallocation for LCS dynamic programming matrix in TUI save diff
**Learning:** Allocating 2D slices (`make([][]int, n+1)`) in LCS dynamic programming creates `n+2` heap allocations per diff calculation in the TUI save preview. Preallocating a single flat `[]int` slice with stride `m+1` and preallocating result slice capacity reduced allocations by ~88% (25 -> 3 allocs/op) and execution time by ~24% (4.38µs -> 3.32µs).
**Action:** In dynamic programming algorithms or matrix operations, use a flat 1D slice with index arithmetic (`i*stride + j`) instead of nested slices to eliminate per-row slice allocations.

## 2025-03-12 - Zero-allocation ANSI escape scanning and string truncation in table layout
**Learning:** Replacing `ansiRE.ReplaceAllString` with a zero-allocation single-pass scanner for ANSI SGR sequences in `visibleWidth` reduced execution time by ~85% (663.6ns -> 97.5ns) and eliminated 100% of heap allocations (80 B -> 0 B). Additionally, replacing `[]rune(s)` conversions in `truncate` with `range s` index scanning eliminated intermediate `[]rune` heap allocations.
**Action:** Avoid `regexp.ReplaceAllString` and `[]rune` slice conversions for string layout/formatting routines; use single-pass string index iteration and inline sequence scanning instead.
