# CLAUDE.md — cs-demo-viewer

## Purpose

Generates a self-contained HTML round-replay viewer from a CS2 `.dem` file.

## Build

```sh
cd cs-demo-viewer
go build -o demoview ./cmd/demoview/
```

## Usage

```sh
./demoview match.dem                     # outputs match.html alongside the demo
./demoview -o /path/to/output.html match.dem
```

Flags must come **before** the positional argument (Go standard flag behavior).

## Output

A single `.html` file with everything embedded (map radar PNG as base64, demo data as JSON).
Open in any modern browser — no server required.

## Supported maps

de_ancient, de_anubis, de_dust2, de_inferno, de_mirage, de_nuke, de_overpass, de_train, de_vertigo

Multi-level maps (de_nuke, de_train, de_vertigo) include both upper and lower radars.
The viewer's Upper/Lower toggle shows the appropriate level.

## Architecture

```
cmd/demoview/main.go          entry point, flag parsing
internal/demo/parser.go       demo → DemoData (demoinfocs-golang v4)
internal/maps/maps.go         map metadata + go:embed radar PNGs
internal/viewer/viewer.go     DemoData + map → self-contained HTML
internal/viewer/template.html HTML/JS viewer (injected with JSON data)
internal/maps/overviews/*.png pre-extracted radar images (from CS2 VPK)
```

## JSON data format (embedded in HTML)

PlayerState is a compact 7-element array: `[idx, flags, hp, x, y, z, yaw]`
- flags: 0=CT+alive, 1=CT+dead, 2=T+alive, 3=T+dead
- x, y, z: world coordinates (integer)
- yaw: degrees (integer)

Kill is a 9-element array: `[tick, atkIdx, vicIdx, weapon, hs(0/1), atkX, atkY, vicX, vicY]`

## Sampling

Positions are sampled every 16 ticks (4 fps at 64 tick/s). The JS viewer interpolates
between keyframes for smooth 60 fps playback.

## Radar image extraction

The bundled PNGs in `internal/maps/overviews/` were extracted from the CS2 game files
(`game/csgo/pak01_dir.vpk`) using the `scripts/extract_overviews.py` script.
Re-extract when Valve updates a map's radar.

## Documentation rule

Any change to flags, CLI flags, data formats, or supported maps must be reflected in
`go-cs-metrics/docs/cs2-pipeline-flow.md` if the change affects the broader pipeline.
