# Handoff: kando TUI (Bubble Tea)

## Overview
kando is a personal kanban CLI that replaces a notepad todo list. This package specifies the terminal UI: a four-lane board (Backlog / Todo / Doing / Done) where the active lane is expanded and the other three are collapsed to title lists, plus card detail, filter, and archive views.

## About the design files
`Kando TUI.dc.html` is a **design reference built in HTML**, not code to port. Recreate it in Go with Bubble Tea + Lip Gloss (+ Bubbles for text input / viewport). Open the HTML in a browser to see the mockups; the ids below (`2a`, `1d`…) are badges in that file.

## Fidelity
**High-fidelity.** Colors, layout proportions, copy, and keybindings are final. Exact pixel geometry translates to character cells (1 line = 1 row, `1ch` = 1 column).

## Chosen direction
- **Board layout: `2a` / `2b`** (focus lane with bordered columns). Everything else in the file is exploration: `1a`–`1c` were alternatives; `1d`–`1f` show the *content and behavior* of the secondary screens but were drawn in a discarded theme (Slate) and borderless style — implement them with the tokens and bordered style below.
- **Adaptive theme:** `2a` Paper on light terminals, `2b` Ember on dark. Use `lipgloss.AdaptiveColor{Light, Dark}` for every token. Truecolor.
- Target terminal: 120×40. Must degrade gracefully (see Responsive).

## Design tokens

| Token | Paper (light) | Ember (dark) | Use |
|---|---|---|---|
| bg | `#f6f1e8` | `#1a1512` | terminal background (do not paint; inherit) |
| fg | `#2b2622` | `#ead9c8` | body text, key hints |
| muted | `#8f8579` | `#7d6b5c` | inactive lane headers, meta, ages, help labels |
| border | `#d9cfc0` | `#33291f` | inactive column and card borders |
| accent | `#1f7a6d` | `#e0a458` | active lane header + border, selected card border, tags, ✓ marks, app name in dark theme |
| accent2 | `#b5532b` | `#8fb98a` | blocked flag (⊘) |
| selectionBg | `#ece4d6` | `#2a211a` | selected card background |

Typography: the terminal's monospace font; only styles used are **bold** (`Bold(true)`), muted color, strikethrough (done titles), and uppercase + letter-spacing for lane headers (render uppercase; Lip Gloss cannot letter-space, so join header letters with no spacing or accept plain uppercase).

Borders: `lipgloss.RoundedBorder()` for columns and cards. Radius/shadows in the HTML are irrelevant in a terminal.

Glyphs: `•` bullet, `✓` done, `⊘` blocked, `▣` / `▢` checklist done/open, `▸` list cursor (detail left pane).

## Screens

### 1. Board (`2a`/`2b`) — default view
Vertical structure, 40 rows:
1. Row 1 header: `kando` (bold; accent color in dark theme, fg in light) · 2 spaces · board name `life` (muted) · right-aligned date `Thu 3 Sep` (muted). 1-col horizontal padding on the whole screen.
2. Row 2 blank.
3. Rows 3–37 lanes.
4. Row 38 blank.
5. Row 39 footer key hints; right-aligned card count `11 cards`.

Lanes: horizontal layout, gap 2 columns. Widths: three collapsed lanes fixed at **22 columns** each (including border); the active lane takes the remainder (`120 - 2 (padding) - 3×22 - 3×2 = 46` cols at 120 wide). Lane order is always Backlog, Todo, Doing, Done; the *active* one expands in place (it is not moved to the center).

Collapsed lane (inactive):
- Rounded border in `border`, 1-col inner padding.
- Header row: lane name uppercase, muted; count right-aligned, muted. Then one blank row.
- One row per card: `• Title` in fg (truncate with `…`); Done lane shows `✓ Title` with ✓ in accent and title muted.

Active lane:
- Rounded border in `accent`; header row in accent, bold, uppercase, count right-aligned; blank row.
- Cards stacked with one blank row between them. Each card is a rounded box (`border`), 2-col inner padding, 3 content rows:
  1. Title (fg) … age right-aligned (muted), e.g. `3d`.
  2. Notes preview, muted, single line, truncated with `…`. Empty row if no notes.
  3. Meta row, 2-col gaps: tag in accent (`#errand`), checklist progress muted (`1/4`), blocked flag in accent2 (`⊘ waiting on pads`). Omit absent items.
- Selected card: border `accent`, background `selectionBg`, title bold.
- Done cards in the active Done lane: dashed border (`lipgloss.Border` with `╌`/`┆` or `NormalBorder` in `border` color), ✓ in accent, title struck through and muted, meta row shows tag + age only.

Footer (muted labels, keys in fg, medium weight): `j/k move  h/l tab lane  a add  enter open  H/L move card  d done  / filter  ? help  q quit`. Two-space gap between groups.

### 2. Card detail (`1d` content; restyle to chosen theme)
Full-view swap, same header row with breadcrumb: `kando  life › Todo › Renew passport`.
Two panes: left **30 cols**, gap 4 cols, right pane fills remainder with a left rule (`│` in `border`) and 3-col left padding.
- Left: active lane name bold with accent count, underline in accent; then card titles, cursor row `▸ Title` bold on `selectionBg`, others `• Title` muted.
- Right, top to bottom: title (bold); meta row muted: tag (accent), `created Mon 31 Aug`, `in Todo since Tue 1 Sep`; blank; `NOTES` label (muted uppercase); notes wrapped at 64 cols; blank; `CHECKLIST 1/4` (count in accent); items — done `▣ text` muted struck, focused `▢ text` on `selectionBg` with a block cursor after the text, open `▢ text`; blank; `BLOCKED` label; `— not blocked. b to set a reason` muted.
Footer: `j/k item  x toggle  o new item  e edit notes  t tag  m move  b block  esc back`.
Editing (notes, title, new item, tag) uses an inline `textinput`/`textarea` in place of the field, accent cursor.

### 3. Filter (`1e` content)
Entered with `/`. Header row is replaced by the prompt: `/` in accent bold, then the query text with a block cursor; right-aligned `N of 11 match` muted. The board below re-renders with non-matching cards removed; lane counts reflect matches; an empty lane shows `· · ·` in `border` color. Filter is live per keystroke over title and `#tag`; `!blocked` and `age>7d` are supported operators. `enter` keeps the filter active (prompt collapses back into the header with the query shown muted), `esc` clears.
Footer: `type to filter title or #tag  enter keep filter  esc clear   !blocked age>7d also work`.

### 4. Archive (`1f` content)
Reached with `D` (or via the Done lane). Header breadcrumb `kando  life › archive`, right `N done`. Content max 80 cols: groups `THIS WEEK 3`, `LAST WEEK 4`, `EARLIER 3` (muted uppercase, count, underline in `border`), each item one row: `✓` (accent) · title (fills, truncates) · tag (accent, 8 cols) · date right-aligned 10 cols muted. Blank row between groups. Trailing note: `Older entries live in ~/.kando/life/archive.md` (path in fg).
Footer: `j/k move  u undo (back to Doing)  enter open  / filter  esc board`.

## Interactions & keys
Both vim keys and arrows work everywhere.
- Board: `j/k ↓↑` select card; `h/l ←→ tab shift-tab` change active lane (selection resets to first card of that lane); `H/L` move selected card to previous/next lane; `J/K` move selected card down/up inside its lane (clamps at the ends; the footer is unchanged, so this key pair lives in the `?` overlay only — see R-1); `enter` open detail; `a` quick-add (an inline text input appears as a new card at the top of the active lane, accent border; `enter` saves, `esc` cancels); `d` move to Done; `/` filter; `?` help overlay; `q` quit.
- Selection wraps within a lane. Done cards are selectable (for `u` undo).
- No animation. Redraw is instant; keep it flicker-free by rendering the whole frame each `View()`.

## Responsive
- Width ≥ 120: as specified. 100–119: collapsed lanes shrink to 18 cols. < 100: collapsed lanes become a single-row tab strip above the active lane (lane name + count, active one inverted in accent), and the active lane takes full width.
- Height: lanes region = height − 4. Active lane scrolls (viewport) keeping the selection visible; collapsed lanes truncate with a final `… +N` row.

## State
- `boards[]`, current board; `lanes[4]` of cards; card `{id, title, notes, tag, checklist[]{text, done}, blocked bool, blockedReason, createdAt, movedAt, doneAt}`.
- UI: `mode ∈ {board, detail, filter, archive, help, quickAdd, editing}`, `activeLane`, `selectedIndex`, `filterQuery`, `detailCursor`, terminal `width/height`, theme resolved via `lipgloss.HasDarkBackground()`.
- Persistence: plain files under `~/.kando/<board>/` (board and `archive.md`); age is derived from `createdAt`, dates rendered `Mon 31 Aug`.

## Sample data (as shown in mockups)
Backlog: Learn to make sourdough `#home` 12d · Replace kitchen tap `#home` 20d · Read Piranesi `#read` 5d.
Todo: Renew passport `#errand` 3d 1/4 (selected) · Tax docs to accountant `#money` 2d · Book dentist `#health` 1d.
Doing: Fix bike brake `#home` 4d ⊘ waiting on pads · Birthday card for mom 1d.
Done: Cancel gym membership `#money` · Return library books `#errand` · Pay electric bill `#money`.

## Files
- `Kando TUI.dc.html` — all mockups. Chosen board: `2a`, `2b`. Secondary-screen content: `1d`, `1e`, `1f`. Alternatives (not chosen): `1a`, `1b`, `1c`.
