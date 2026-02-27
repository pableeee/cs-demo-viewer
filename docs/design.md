# cs-demo-viewer — Design Document

## Overview

`demoview` parses a CS2 `.dem` file and produces a single self-contained `.html`
file that replays each round on the map radar. The HTML file embeds all data and
assets — no server or network requests are needed at view time.

The pipeline has three stages:

```
.dem file  →  parser.go (DemoData)  →  viewer.go (HTML)  →  template.html (JS renderer)
```

---

## Data Pipeline

### Stage 1: Parsing (`internal/demo/parser.go`)

Uses `demoinfocs-golang v4` (event-driven model). The parser registers handlers
for each relevant event type and drives them with a `ParseNextFrame()` loop.

**Key state variables:**

| Variable | Purpose |
|---|---|
| `cur *Round` | Round being built; nil outside rounds |
| `inRound bool` | True between RoundStart and RoundEnd |
| `roundNum int` | Monotonically increasing round counter |
| `freezeEndTick int` | Tick when buy/freeze time ended; no frames sampled before this |
| `lastSampledTick int` | Last tick at which a frame was captured (deduplication guard) |
| `ctScore, tScore int` | Running cumulative scores, incremented at RoundEnd |
| `lastShot map[int]int` | playerIdx → last WeaponFire tick (shot deduplication) |
| `roundVicDmg map[int]map[int]int` | attIdx → vicIdx → accumulated HP damage this round |
| `pendingThrows map[int64]int` | grenade uniqueID → throw tick (for trajectory recording) |
| `bombX, bombY int` | Last known bomb world position |
| `bombSite string` | Last known bomb site ("A", "B", or "") |

**Event handlers registered:**

| Event | Action |
|---|---|
| `RoundStart` | Create new `cur`, reset per-round state |
| `RoundFreezetimeEnd` | Set `freezeEndTick`, store in `cur.FreezeEnd` |
| `RoundEnd` | Set winner, increment running score, capture final frame, append round if ≥5 frames |
| `Kill` | Append kill with per-round damage from `roundVicDmg`, update match stats |
| `PlayerHurt` | Accumulate `roundVicDmg`, append to `cur.Dmg`, accumulate match DMG stat |
| `BombPlantBegin` | Action 0: record player position as bomb position, site from event |
| `BombPlanted` | Action 1: update bomb position from player |
| `BombDefuseStart` | Action 2: use last known `bombX/Y/Site` |
| `BombDefused` | Action 3 |
| `BombExplode` | Action 4 |
| `BombDropped` | Action 5: update bomb position from player |
| `BombPickup` | Action 6 |
| `SmokeStart` | Fixed-duration grenade (type 4=CT, 5=T), `EndTick = tick + 1152` (~18 s) |
| `HeExplode` | Instant grenade (type 2), `EndTick = 0` |
| `FlashExplode` | Instant grenade (type 1), `EndTick = 0` |
| `InfernoStart` | Fixed-duration grenade (type 3), `EndTick = tick + 448` (~7 s); position from `e.Inferno.Entity.Position()` |
| `GrenadeProjectileThrow` | Record `pendingThrows[uid] = tick` |
| `GrenadeProjectileDestroy` | Build `GrenadeTrail` from `Trajectory2`, subsample to ≤80 points |
| `WeaponFire` | Append `Shot` if > `SampleTicks` since last shot for this player |

**Frame sampling loop:**

The main loop calls `p.ParseNextFrame()` after each event dispatch.
Frames are captured at regular tick intervals:

```
if freezeEndTick > 0 && tick >= freezeEndTick
   && tick > lastSampledTick           // DEM_FullPacket deduplication
   && tick % SampleTicks == 0
```

`SampleTicks = 16` → 4 keyframes/second at 64 tick/s.

### DEM_FullPacket Deduplication (Critical Fix)

CS2 demos periodically emit a `DEM_FullPacket` (full game state snapshot) alongside
the normal delta frames, roughly every 64 ticks. When `ParseNextFrame()` processes
such a packet, `p.GameState().IngameTick()` returns the **same tick** as the
preceding delta frame. Without a guard, the sampling loop would capture two identical
frames at the same tick, producing a 1-frame freeze visible every ~1 second during
playback.

Fix: `lastSampledTick` tracks the last tick at which a frame was captured.
The condition `tick > lastSampledTick` prevents duplicate captures.
`lastSampledTick` is reset to 0 at each `RoundStart`.

### Stage 2: HTML Generation (`internal/viewer/viewer.go`)

Marshals `DemoData` to JSON and injects it into the template via a literal string
replacement of the `/*INJECT_DATA*/` placeholder. Radar PNGs are base64-encoded
and inlined as `data:image/png;base64,...` URIs.

```go
html := strings.Replace(templateHTML, "/*INJECT_DATA*/", string(jsonBytes), 1)
```

The HTML template is compiled into the binary at build time via `//go:embed`.

### Stage 3: JS Renderer (`internal/viewer/template.html`)

Single-file HTML/JS, ~750 lines. All rendering is on a `<canvas>` element using
the 2D Canvas API. No external libraries.

---

## Data Format Reference

All types use custom `MarshalJSON` to emit compact arrays rather than objects,
minimizing output size.

### Top-level: `ViewerData`

```json
{
  "map":        "de_mirage",
  "meta":       { "pos_x": -3230, "pos_y": 1713, "scale": 5.0 },
  "radar":      "data:image/png;base64,...",
  "radar_lower": "",
  "has_lower":  false,
  "lower_z_max": 0,
  "players":    [ ... ],
  "rounds":     [ ... ],
  "stats":      [ ... ]
}
```

**`meta`**: CS2 overview coordinate origin and scale.
World coordinate → radar pixel: `px = (world - pos_x) / scale * (canvasSize / 1024)`.

### `PlayerInfo`

```json
{ "id": "76561198034202275", "name": "s1mple" }
```

Players are stored in a flat array. All other data references them by index.

### `PlayerStat`

```json
{ "k": 25, "d": 14, "hs": 10, "dmg": 3124, "r": 24 }
```

Parallel to the `players` array. `r` = rounds played, used to compute ADR.

### `Round`

```json
{
  "n":    5,
  "w":    "CT",
  "cts":  3,
  "ts":   1,
  "fe":   12288,
  "frames":   [ ... ],
  "kills":    [ ... ],
  "bomb":     [ ... ],
  "grenades": [ ... ],
  "shots":    [ ... ],
  "dmg":      [ ... ],
  "trails":   [ ... ]
}
```

- `w`: winner `"CT"`, `"T"`, or `""` (draw / incomplete)
- `cts`, `ts`: cumulative score at the **start** of this round (before this round's result)
- `fe`: freeze-end tick; used for round-elapsed-time display and frame sampling start

### `Frame`

```json
{ "tick": 13056, "p": [ [2, 0, 87, 512, -340, 64, 180], ... ] }
```

### `PlayerState` — compact 7-element array

```
[idx, flags, hp, x, y, z, yaw]
```

| Field | Type | Description |
|---|---|---|
| `idx` | int | Index into `players` array |
| `flags` | int | Bitmask: bit 0 = dead, bit 1 = T-side, bit 2 = bomb carrier |
| `hp` | int | Current health (0–100) |
| `x`, `y`, `z` | int | World coordinates (rounded to nearest integer) |
| `yaw` | int | View direction in degrees (0–360) |

**Flags combinations:**
- `0` = CT + alive
- `1` = CT + dead
- `2` = T + alive
- `3` = T + dead
- `4` = CT + alive + bomb carrier
- `6` = T + alive + bomb carrier

### `Kill` — compact 10-element array

```
[tick, atkIdx, vicIdx, weapon, hs, atkX, atkY, vicX, vicY, dmg]
```

| Field | Description |
|---|---|
| `tick` | Game tick of the kill |
| `atkIdx` | Attacker's index in `players` |
| `vicIdx` | Victim's index in `players` |
| `weapon` | Weapon name string (e.g. `"AK-47"`) |
| `hs` | 1 = headshot, 0 = body |
| `atkX/Y` | Attacker world position at kill time |
| `vicX/Y` | Victim world position at kill time |
| `dmg` | Total HP damage attacker dealt to victim this round (from `PlayerHurt` accumulation) |

### `BombAction` — compact 5-element array

```
[tick, action, x, y, site]
```

| `action` | Meaning |
|---|---|
| 0 | Plant begin |
| 1 | Planted (C4 armed) |
| 2 | Defuse begin |
| 3 | Defused |
| 4 | Exploded |
| 5 | Dropped |
| 6 | Picked up |

Position (`x`, `y`) is the last known bomb world position.
`site` is `"A"`, `"B"`, or `""` (not applicable for drop/pickup events).

### `Grenade` — compact 5-element array

```
[startTick, endTick, type, x, y]
```

| `type` | Grenade | Duration |
|---|---|---|
| 0 | Smoke (generic/unknown team) | `endTick - startTick` |
| 1 | Flash | instant (`endTick = 0`) |
| 2 | HE | instant (`endTick = 0`) |
| 3 | Molotov / Incendiary | `endTick - startTick` |
| 4 | Smoke — CT thrower | 1152 ticks (~18 s) |
| 5 | Smoke — T thrower | 1152 ticks (~18 s) |

`endTick = 0` means instant — the JS renderer uses `GREN_FADE_TICKS` (64) for
display duration.

### `Shot` — compact 2-element array

```
[tick, playerIdx]
```

Shots are deduplicated: at most one per player per `SampleTicks` (16 tick) window.
Used to drive the muzzle-flash ring on firing players.

### `GrenadeTrail` — compact 4-element array

```
[startTick, endTick, type, [[tickOffset, x, y], ...]]
```

- `startTick`: tick of `GrenadeProjectileThrow`
- `endTick`: tick of `GrenadeProjectileDestroy` (when nade hits/explodes)
- `type`: same constants as `Grenade` (4/5 for CT/T smokes)
- Points: up to 80 `[tickOffset, worldX, worldY]` triples, subsampled from
  `Trajectory2`. `tickOffset` is `time.Duration.Seconds() * 64` — elapsed ticks
  from throw, not absolute game ticks.

### `Dmg` — array of `[2]int`

```
[[playerIdx, hpDamage], ...]
```

Per-damage-event log for the round. Used by the stats panel to compute per-round
damage totals. Does not include team damage.

---

## Coordinate System

CS2 world coordinates use a right-handed Z-up system. The radar image maps
world X→right and world Y→up (screen Y is inverted).

**World to canvas pixel:**

```js
function w2c(wx, wy) {
  const m = DEMO.meta, cs = canvas.width / RADAR_SIZE;
  const bx = ((wx - m.pos_x) / m.scale) * cs;
  const by = ((m.pos_y - wy)  / m.scale) * cs;   // Y flipped
  const cx = canvas.width / 2, cy = canvas.height / 2;
  return [(bx - cx) * zoom + cx + panX,
          (by - cy) * zoom + cy + panY];
}
```

**World radius to canvas pixels:**

```js
function worldRToPx(worldR) {
  return zoom * (worldR / DEMO.meta.scale) * (canvas.width / RADAR_SIZE);
}
```

---

## Zoom and Pan

State: `zoom` (default 1), `panX`, `panY` (default 0).

**Zoom to mouse cursor** (scroll wheel handler):

```js
const mx = e.offsetX, my = e.offsetY;
panX = mx - (mx - canvas.width/2 - panX) * (newZoom / zoom) - canvas.width/2;
panY = my - (my - canvas.height/2 - panY) * (newZoom / zoom) - canvas.height/2;
zoom = newZoom;
clampPan();
```

**Pan clamping** (prevents dragging map fully off screen):

```js
function clampPan() {
  const maxPan = canvas.width / 2 * (zoom - 1);
  panX = Math.max(-maxPan, Math.min(maxPan, panX));
  panY = Math.max(-maxPan, Math.min(maxPan, panY));
}
```

**Radar image position under zoom:**

```js
const imgX = canvas.width/2  * (1 - zoom) + panX;
const imgY = canvas.height/2 * (1 - zoom) + panY;
ctx.drawImage(img, imgX, imgY, sz * zoom, sz * zoom);
```

Double-click resets `zoom = 1, panX = 0, panY = 0`.

---

## Playback and Interpolation

Frames are sampled at 4 fps (every 16 ticks). The JS renderer runs at up to 60 fps
via `requestAnimationFrame`. A float `framePos` tracks the sub-frame position:

```
framePos += (deltaTime_s * speed * SAMPLE_FPS)
```

Player positions are linearly interpolated between `floor(framePos)` and
`ceil(framePos)`:

```js
const t = framePos - Math.floor(framePos);
const px = ps0[PS_X] + (ps1[PS_X] - ps0[PS_X]) * t;
```

Yaw is interpolated with shortest-arc logic to handle the 0/360 wrap.

`speed` values available: 0.5×, 1×, 2×, 4×, 8×.

---

## Rendering Order

Each frame is rendered in this order (painter's algorithm — later items appear on top):

1. Radar image (with zoom/pan transform)
2. Active smokes (semi-transparent circles, drawn first so players appear on top)
3. Active molotovs
4. Grenade trails (throw arcs, fading)
5. HE / flash burst rings
6. Bomb marker (`B` text circle)
7. Kill flash markers (expanding ring at kill positions)
8. Players (alive, then dead — dead drawn first so alive appear on top)
   - For each alive player: direction line → circle → name label → shoot flash ring → C4 badge
9. Kill feed (DOM overlay, not canvas)
10. Tooltip (DOM overlay)

---

## Kill Feed

The kill feed is a DOM overlay (not canvas), positioned absolute top-right.
It shows the last `KILL_FEED_MAX = 8` entries, merged from three event streams:

```js
const merged = [
  ...kills.map(k   => ({ t: k[K_TICK],  type: 'kill', data: k  })),
  ...bombEvts.map(b => ({ t: b[BA_TICK], type: 'bomb', data: b  })),
  ...nadeEvts.map(g => ({ t: g[GR_ST],  type: 'nade', data: g  })),
].sort((a, b) => a.t - b.t)
 .filter(ev => ev.t <= currentTick)
 .slice(-KILL_FEED_MAX);
```

The feed re-renders only when the set of visible events changes (tracked by a
signature string), avoiding DOM thrashing on every frame.

---

## Stats Panel

The stats panel is a right-side collapsible panel showing a table of:
`Player | K | D | HS% | DMG` for the **current round**.

Stats are derived at render time from `round.kills` (for K/D/HS) and `round.dmg`
(for damage totals), not from the global `data.stats` (which are match totals).
The panel header reads "Round N Stats" and updates when the round changes.

---

## Multi-Level Maps

For maps with multiple vertical levels (de_nuke, de_train, de_vertigo), two radar
images are embedded. The JS viewer determines which level to use based on the
player's `z` coordinate relative to `lower_z_max`:

```js
const onLower = ps[PS_Z] < DEMO.lower_z_max;
```

A toggle button (Upper/Lower) also allows manual override.
The appropriate image is drawn before player rendering.

---

## Map Coordinate Metadata

From `internal/maps/maps.go`:

| Map | PosX | PosY | Scale |
|---|---|---|---|
| de_ancient | -2953 | 2164 | 5.0 |
| de_anubis | -2796 | 3328 | 5.22 |
| de_dust2 | -2476 | 3239 | 4.4 |
| de_inferno | -2087 | 3870 | 4.9 |
| de_mirage | -3230 | 1713 | 5.0 |
| de_nuke | -3453 | 2887 | 7.0 |
| de_overpass | -4831 | 1781 | 5.2 |
| de_train | -2308 | 2078 | 4.082077 |
| de_vertigo | -3168 | 1762 | 4.0 |

Lower level Z thresholds:

| Map | ZMax |
|---|---|
| de_nuke | -495 |
| de_train | -130 |
| de_vertigo | 11700 |

---

## Constants Reference

| Constant | Value | Meaning |
|---|---|---|
| `SampleTicks` | 16 | Ticks between frame captures (parser) |
| `RADAR_SIZE` | 1024 | Radar image width/height in pixels |
| `SAMPLE_FPS` | 4 | Keyframes per second |
| `PLAYER_R` | 8 | Player dot radius (canvas px at zoom=1) |
| `DIR_LEN` | 18 | Direction line length (canvas px at zoom=1) |
| `KILL_FLASH_TICKS` | 48 | Kill position flash duration (~0.75 s) |
| `KILL_FEED_MAX` | 8 | Max entries in kill feed |
| `SMOKE_WORLD_R` | 170 | Smoke world-unit radius |
| `MOLOTOV_WORLD_R` | 120 | Molotov world-unit radius |
| `GREN_FADE_TICKS` | 64 | HE/flash display duration (1 s at 64 tick/s) |
| `SHOOT_FLASH_TICKS` | 12 | Muzzle flash ring duration (~0.19 s) |
| `BOMB_FLASH_PERIOD` | 32 | Bomb blink period when planting (~0.5 s) |
| `TRAIL_FADE_TICKS` | 96 | Grenade trail fade duration after landing (~1.5 s) |

Smoke/molotov durations (parser-side):

| Grenade | Parser ticks | Approx. duration |
|---|---|---|
| Smoke | 1152 | ~18 s |
| Molotov / Incendiary | 448 | ~7 s |

---

## Known Limitations and Trade-offs

**Grenade duration is fixed, not event-driven.** Smoke and molotov end-ticks are set
to `startTick + constant` rather than from `SmokeExpired` / `InfernoExpired` events.
This was chosen to avoid a cross-round boundary bug: the `Expired` events can fire
after `data.Rounds = append(data.Rounds, *cur)` copies the round, so mutations to
the round after appending would be lost. The fixed duration matches the in-game
durations closely enough for practical use.

**Shot deduplication caps rate at 1 shot per SampleTicks window.** Rapid-fire weapons
(e.g. SMGs) may show fewer flash rings than actual shots, but this prevents the
`shots` array from bloating with hundreds of entries per burst.

**Damage in kills is total round damage, not final-shot damage.** `Kill.DMG` is the
sum of all `PlayerHurt` events from that attacker to that victim during the round.
This better reflects "how much work did they put in" than final-bullet damage alone.

**Grenade trajectory uses `Trajectory2[i].Time`**, a `time.Duration` field recording
elapsed real time from throw. Converting to ticks: `tickOffset = Time.Seconds() * 64`.
This is accurate for 64-tick demos; POV demos may behave differently.

**Only one player index per player.** If a player disconnects and reconnects with a
different `SteamID64` (rare in FACEIT/ESEA), they will appear as two separate entries
in the players array.

**Rounds with fewer than 5 frames are discarded.** This filters out warmup rounds and
knife rounds that end immediately.
