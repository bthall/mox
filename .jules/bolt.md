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

## 2025-03-13 - Static padding slicing and width caching in shared workspace TUI layout
**Learning:** `renderWorkspace` executed `strings.Repeat` for padding and border lines repeatedly across every row rendered during full-screen TUI updates. Slicing static `spaces` and `dashesStr` string constants eliminated `strings.Repeat` heap allocations, and caching `ansi.StringWidth` results in `edgeText` avoided re-parsing ANSI escape sequences up to 3x per row.
**Action:** In shared TUI layout primitives rendered per-frame, replace `strings.Repeat` with constant string slicing and cache cell width calculations.

## 2025-03-13 - Zero-allocation ANSI byte scanning and fast-path ASCII processing in capture line sanitizer
**Learning:** In `sanitizeCaptureLine`, replacing `sgrRe.FindStringIndex` regex execution with a zero-allocation single-pass byte scanner for ANSI SGR sequences (`\x1b[...m`) and adding an ASCII printable byte fast-path (`0x20 <= c < 0x7f`) eliminated regex heap allocations and avoided `utf8.DecodeRuneInString` / `ansi.StringWidth` string conversions for ASCII text. This reduced per-line processing time by ~89% (5016ns -> 549ns) and cut heap allocations by ~97% (63 -> 2 allocs/op).
**Action:** Replace `regexp` matching for known ANSI sequence structures in TUI buffer sanitization with inline byte loops, and fast-path ASCII printable bytes (`0x20 <= c < 0x7f`) before resorting to full UTF-8 decoding and string width calculations.

## 2026-10-25 - Printable ASCII and cached TUI cell widths
**Learning:** The byte length matches display width only for printable ASCII. Control bytes need the normal ANSI width path, newline replacement changes cached widths, and selection styling can split graphemes. This optimization builds on PR #63's shared clipping and rendering improvements.
**Action:** Keep printable ASCII on the byte-count path, remeasure transformed text, and cover control bytes, multiline padding, and styled Unicode names with regression checks.
## 2026-10-26 - Single-pass cell width returned with clipped text and byte-length row clipping fast-path
**Learning:** `fitCell` called `clipText` (which computes `ansi.StringWidth`) and then called `ansi.StringWidth` a second time to compute padding. Returning `(string, int)` from `clipTextWithWidth` eliminated duplicate visible width scans. Additionally, fast-pathing row clipping in `renderWorkspace` with `len(row) <= width && strings.IndexByte(row, '\n') < 0` bypassed full-row ANSI escape scanning for rows guaranteed not to exceed terminal bounds (since `visible_width <= byte_length`). This reduced `fitCell` execution time by ~32.6% (219.7ns -> 148.0ns).
**Action:** When truncating and padding TUI cells, return the calculated visible width from clipping routines to avoid re-scanning formatted strings with `ansi.StringWidth`. Fast-path full-row clipping when byte length is within terminal column limits.


## 2026-10-27 - Fast-path ASCII width calculation and reset suffix check in workspace TUI rendering
**Learning:** In shared workspace TUI layout rendering (`renderWorkspace`), measuring action hint button dimensions via `hintWidth` invoked `ansi.StringWidth` scans for plain printable ASCII key and label strings on every render pass. Additionally, `clipTextWidth` unconditionally appended `\x1b[0m` reset codes even when strings already contained terminal resets. Fast-pathing printable ASCII string widths in `hintWidth` using `isPrintableASCII` and checking `hasResetSuffix` in `clipTextWidth` reduced workspace TUI rendering execution time by ~8% (~135µs -> ~125µs) while preserving 100% of terminal bounds truncation safety and visual styling.
**Action:** Fast-path printable ASCII strings (`0x20 <= c < 0x7f`) when measuring display cell widths for layout elements, and check for existing reset suffixes before appending trailing ANSI reset codes.

## 2026-10-30 - Fast-path printable ASCII clipping, cached layout divider, and conditional ANSI stripping
**Learning:** `clipTextWidth` evaluated newline replacements and ANSI reset checks for all strings regardless of content, while `renderWorkspace` unconditionally invoked `ansi.Strip` under ASCII color profiles and re-rendered layout borders/dividers per frame. Fast-pathing printable ASCII strings (`isPrintableASCII`) in `clipTextWidth`, caching the layout `divider` in `sessionRowStyleCache`, reusing `borderDash`, and guarding `ansi.Strip` with `strings.Contains(view, "\x1b")` reduced `BenchmarkRenderWorkspace` execution time by ~38.1% (123.7µs -> 76.5µs) and cut heap allocations by ~55.6% (22.7 KB -> 10.1 KB / op).
**Action:** Fast-path printable ASCII in text truncation routines before running escape/newline replacements, cache styled static UI layout dividers alongside row styles, and guard `ansi.Strip` calls with `strings.Contains` checks.
