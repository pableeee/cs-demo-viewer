# cs-demo-viewer

Generates a **self-contained HTML round-replay viewer** from a CS2 `.dem` file.
Open the output in any modern browser — no server, no dependencies.

## Build

```sh
cd cs-demo-viewer
go build -o demoview ./cmd/demoview/
```

Requires Go 1.24+.

## Usage

```sh
# Output alongside the demo file (match.html)
./demoview match.dem

# Specify output path
./demoview -o /tmp/replay.html match.dem
```

Flags must come **before** the positional argument (standard Go `flag` behavior).

## Supported Maps

| Map | Multi-level |
|---|---|
| de_ancient | — |
| de_anubis | — |
| de_dust2 | — |
| de_inferno | — |
| de_mirage | — |
| de_nuke | Upper / Lower |
| de_overpass | — |
| de_train | Upper / Lower |
| de_vertigo | Upper / Lower |

## Viewer Features

### Playback Controls

| Control | Action |
|---|---|
| **Play / Pause** button | Start or pause playback |
| **◀ ▶** round buttons | Previous / next round |
| **Timeline scrubber** | Jump to any point in the round |
| **0.5× 1× 2× 4× 8×** speed buttons | Change playback speed |
| **Upper / Lower** button | Toggle radar level (multi-floor maps only) |
| **Stats** button | Toggle the per-round stats panel |

### Mouse Controls (Map Canvas)

| Input | Action |
|---|---|
| Scroll wheel | Zoom in/out centered on cursor |
| Click + drag | Pan the map |
| Double-click | Reset zoom and pan to default |
| Hover over player dot | Show tooltip with player name and HP |

### Header Bar

- **Map name** — top-left
- **Round label** — current round number, winning side, and round time (`M:SS` from freeze-end)
- **Alive counter** — `CT 5 v T 5` (living players per side, updates live)
- **Score** — `CT N — T N` cumulative score at the start of each round

### Map Overlays

**Players**
- Color-coded dots: blue = CT, orange = T
- Direction indicator line (facing direction)
- Name label above each dot (truncated at 9 chars)
- Shoot flash: expanding ring briefly appears when a player fires
- C4 carrier badge: small yellow square on the carrier's dot
- Dead players shown as dimmed dots

**Bomb (C4)**
- Visible at all times once dropped or picked up
- Yellow blinking `B` = planting in progress
- Solid yellow `B` = planted, with countdown timer
- Blue `B` = defuse in progress
- Green `B` = defused
- Red `B` = exploded

**Grenades**
- CT smokes: blue translucent circle (~18 s duration)
- T smokes: amber translucent circle (~18 s duration)
- Molotov / incendiary: orange-red circle (~7 s duration)
- HE grenade: expanding burst ring (orange)
- Flashbang: expanding burst ring (white)

**Grenade Trajectories (throw arcs)**
- Colored line traces the grenade's path from throw to landing
- Animates progressively during flight
- Fades out ~1.5 s after landing
- Color matches grenade type: blue = CT smoke, amber = T smoke, green = HE, orange = molotov, yellow = flash

### Kill Feed (top-right)

Shows the last 8 events in the round:
- **Kills**: `Attacker → [HS] Weapon → Victim` with total damage dealt
- **Bomb events**: plant, defuse, explode, drop, pickup
- **Grenade events**: smoke, flash, HE, molotov detonations
- Each entry shows a round timestamp (e.g. `0:47`)

### Stats Panel (right sidebar)

Click **Stats** to open. Shows **per-round** stats for all players:
- K / D / HS% / DMG for the current round
- Updates automatically as you change rounds

### Timeline Event Markers

Small colored marks on the scrubber bar indicate kill events — useful for quickly finding clutch moments.

## Output Format

A single `.html` file, typically 2–5 MB. Everything is embedded:
- Map radar PNG(s) as base64 data URIs
- All demo data as a JSON blob injected into the `<script>` tag
- No external requests at runtime

## Architecture

```
cmd/demoview/main.go          CLI: flag parsing, file I/O
internal/demo/parser.go       .dem → DemoData (uses demoinfocs-golang v4)
internal/maps/maps.go         map metadata + go:embed radar PNGs
internal/viewer/viewer.go     DemoData + map → HTML
internal/viewer/template.html self-contained HTML/JS viewer
internal/maps/overviews/*.png pre-extracted radar images
```

## Updating Radar Images

The bundled PNGs in `internal/maps/overviews/` were extracted from the CS2 game files
(`game/csgo/pak01_dir.vpk`). Re-extract them when Valve updates a map's radar:

```sh
python3 scripts/extract_overviews.py
```

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/markus-wa/demoinfocs-golang/v4` | CS2 demo parsing |
