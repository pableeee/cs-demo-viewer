package demo

import (
	"encoding/json"
	"fmt"
	"io"
	"math"

	demoinfocs "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	common "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/events"
)

func iround(f float64) int { return int(math.Round(f)) }

// SampleTicks is how many ticks between sampled player-position frames.
// At 64 ticks/sec, 16 ticks = 4 fps keyframes, interpolated to 60 fps in the viewer.
const SampleTicks = 16

// DemoData is the full parsed representation of a demo.
type DemoData struct {
	MapName string       `json:"map"`
	Players []PlayerInfo `json:"players"`
	Rounds  []Round      `json:"rounds"`
	Stats   []PlayerStat `json:"stats"` // parallel to Players, indexed by player index
}

// PlayerInfo is the static info for a player (referenced by index in frames/kills).
type PlayerInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PlayerStat accumulates per-player match stats across all rounds.
type PlayerStat struct {
	K   int `json:"k"`   // kills
	D   int `json:"d"`   // deaths
	HS  int `json:"hs"`  // headshot kills
	DMG int `json:"dmg"` // damage dealt (excluding team damage)
	R   int `json:"r"`   // rounds played (for ADR = DMG/R)
}

// Round contains all sampled frames and kills for one round.
type Round struct {
	Num       int          `json:"n"`
	Winner    string       `json:"w"`              // "CT", "T", or ""
	CTScore   int          `json:"cts"`             // CT score at START of this round
	TScore    int          `json:"ts"`              // T score at START of this round
	FreezeEnd int          `json:"fe"`              // tick when freeze time ended
	Frames    []Frame      `json:"frames"`
	Kills     []Kill       `json:"kills"`
	Bomb      []BombAction `json:"bomb"`
	Grenades  []Grenade    `json:"grenades"`
	Shots     []Shot         `json:"shots"`
	Dmg       [][2]int       `json:"dmg,omitempty"`    // per-player damage: [playerIdx, healthDamage]
	Trails    []GrenadeTrail `json:"trails,omitempty"` // grenade throw arcs
}

// Frame is one sampled tick's snapshot of all player states.
type Frame struct {
	Tick    int           `json:"tick"`
	Players []PlayerState `json:"p"`
}

// PlayerState is one player's state at a sampled tick, serialized as a compact JSON array:
// [idx, flags, hp, x, y, z, yaw]
// flags: 0=CT+alive, 1=CT+dead, 2=T+alive, 3=T+dead
type PlayerState struct {
	Idx   int
	Flags int
	HP    int
	X     int
	Y     int
	Z     int
	Yaw   int
}

func (ps PlayerState) MarshalJSON() ([]byte, error) {
	return json.Marshal([7]int{ps.Idx, ps.Flags, ps.HP, ps.X, ps.Y, ps.Z, ps.Yaw})
}

// Kill is serialized as a compact JSON array:
// [tick, atkIdx, vicIdx, weapon, headshot(0/1), atkX, atkY, vicX, vicY, assisterIdx, flashAssist(0/1)]
// assisterIdx: -1 if no assist; flashAssist: 1 if the assist was via flashbang
type Kill struct {
	Tick        int
	AtkIdx      int
	VicIdx      int
	Weapon      string
	HS          bool
	AtkX        int
	AtkY        int
	VicX        int
	VicY        int
	AssisterIdx int
	FlashAssist bool
}

func (k Kill) MarshalJSON() ([]byte, error) {
	hs := 0
	if k.HS {
		hs = 1
	}
	fa := 0
	if k.FlashAssist {
		fa = 1
	}
	return json.Marshal([]any{k.Tick, k.AtkIdx, k.VicIdx, k.Weapon, hs, k.AtkX, k.AtkY, k.VicX, k.VicY, k.AssisterIdx, fa})
}

// BombAction is serialized as a compact JSON array: [tick, action, x, y, site]
// action: 0=plant_begin, 1=planted, 2=defuse_begin, 3=defused, 4=exploded, 5=dropped, 6=pickup
type BombAction struct {
	Tick   int
	Action int
	X      int
	Y      int
	Site   string
}

func (b BombAction) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{b.Tick, b.Action, b.X, b.Y, b.Site})
}

// Grenade is serialized as a compact JSON array: [startTick, endTick, type, x, y, throwerIdx]
// type: 0=smoke, 1=flash, 2=HE, 3=molotov, 4=smoke-CT, 5=smoke-T; endTick=0 means instant
// throwerIdx: index into Players slice (-1 if unknown)
type Grenade struct {
	StartTick  int
	EndTick    int
	Type       int
	X          int
	Y          int
	ThrowerIdx int
}

func (g Grenade) MarshalJSON() ([]byte, error) {
	return json.Marshal([6]int{g.StartTick, g.EndTick, g.Type, g.X, g.Y, g.ThrowerIdx})
}

// Shot is serialized as a compact JSON array: [tick, playerIdx]
type Shot struct {
	Tick int
	PIdx int
}

func (s Shot) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]int{s.Tick, s.PIdx})
}


// GrenadeTrail is the throw arc of a grenade, serialized as: [startTick, endTick, type, throwerIdx, [[tickOffset,x,y],...]]
// tickOffset is the elapsed ticks from startTick at each sampled point.
type GrenadeTrail struct {
	StartTick  int
	EndTick    int
	Type       int
	ThrowerIdx int
	Points     [][3]int // [tickOffset, x, y]
}

func (gt GrenadeTrail) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{gt.StartTick, gt.EndTick, gt.Type, gt.ThrowerIdx, gt.Points})
}

// equipToGrenadeType maps equipment type to the Grenade type constant.
// Returns -1 for non-tracked types.
func equipToGrenadeType(t common.EquipmentType) int {
	switch t {
	case common.EqSmoke:
		return 0
	case common.EqFlash:
		return 1
	case common.EqHE:
		return 2
	case common.EqMolotov, common.EqIncendiary:
		return 3
	}
	return -1
}


// Parse reads a CS2 demo from r and returns the structured DemoData.
func Parse(r io.Reader) (*DemoData, error) {
	p := demoinfocs.NewParser(r)
	defer p.Close()

	data := &DemoData{}
	pidx := make(map[uint64]int) // steamID64 → Players index

	var cur *Round
	var inRound bool
	var roundNum int
	var freezeEndTick int  // only sample frames after freeze ends
	var lastSampledTick int // deduplicate frames caused by full-snapshot packets
	var ctScore, tScore int
	lastShot := map[int]int{}            // playerIdx → last shot tick (dedup)
	roundVicDmg := map[int]map[int]int{} // attIdx → vicIdx → accumulated hp-dmg this round
	pendingThrows := map[int64]struct{ tick, throwerIdx int }{} // grenade uniqueID → throw info
	lastMolotovThrowerIdx := -1          // thrower of the most recent molotov projectile (for InfernoStart)
	var bombX, bombY int
	var bombSite string

	// getIdx returns the Players-slice index for a player, growing the slice if needed.
	// data.Stats is kept parallel to data.Players.
	getIdx := func(pl *common.Player) int {
		if pl == nil {
			return -1
		}
		id := pl.SteamID64
		if i, ok := pidx[id]; ok {
			data.Players[i].Name = pl.Name
			return i
		}
		i := len(data.Players)
		pidx[id] = i
		data.Players = append(data.Players, PlayerInfo{
			ID:   fmt.Sprintf("%d", id),
			Name: pl.Name,
		})
		data.Stats = append(data.Stats, PlayerStat{}) // keep parallel
		return i
	}

	captureFrame := func(tick int) Frame {
		frame := Frame{Tick: tick}
		bomb := p.GameState().Bomb()
		var carrierID uint64
		if bomb != nil && bomb.Carrier != nil {
			carrierID = bomb.Carrier.SteamID64
		}
		for _, pl := range p.GameState().Participants().Playing() {
			if pl == nil || pl.SteamID64 == 0 {
				continue
			}
			pos := pl.Position()
			flags := 2 // T+alive
			if pl.Team == common.TeamCounterTerrorists {
				flags = 0 // CT+alive
			}
			if !pl.IsAlive() {
				flags++ // CT+dead=1, T+dead=3
			}
			if pl.SteamID64 == carrierID {
				flags |= 4 // bomb carrier
			}
			frame.Players = append(frame.Players, PlayerState{
				Idx:   getIdx(pl),
				Flags: flags,
				HP:    pl.Health(),
				X:     iround(pos.X),
				Y:     iround(pos.Y),
				Z:     iround(pos.Z),
				Yaw:   iround(float64(pl.ViewDirectionX())),
			})
		}
		return frame
	}

	p.RegisterEventHandler(func(e events.RoundStart) {
		if p.GameState().IsWarmupPeriod() {
			return
		}
		roundNum++
		cur = &Round{Num: roundNum, CTScore: ctScore, TScore: tScore}
		freezeEndTick = 0
		lastSampledTick = 0
		inRound = true
		lastShot = map[int]int{}
		roundVicDmg = map[int]map[int]int{}
		pendingThrows = map[int64]struct{ tick, throwerIdx int }{}
		lastMolotovThrowerIdx = -1
	})

	p.RegisterEventHandler(func(e events.RoundFreezetimeEnd) {
		if cur == nil {
			return
		}
		freezeEndTick = p.GameState().IngameTick()
		cur.FreezeEnd = freezeEndTick
	})

	p.RegisterEventHandler(func(e events.RoundEnd) {
		if cur == nil {
			return
		}
		switch e.Winner {
		case common.TeamCounterTerrorists:
			cur.Winner = "CT"
		case common.TeamTerrorists:
			cur.Winner = "T"
		}
		if cur.Winner == "CT" {
			ctScore++
		} else if cur.Winner == "T" {
			tScore++
		}
		// Capture a final frame at the round-end tick so the last kill flash renders.
		tick := p.GameState().IngameTick()
		if f := captureFrame(tick); len(f.Players) > 0 {
			cur.Frames = append(cur.Frames, f)
		}
		// Only keep rounds with meaningful live-play data.
		if len(cur.Frames) >= 5 {
			// Count rounds played for each participant (use first frame).
			if len(cur.Frames) > 0 {
				for _, ps := range cur.Frames[0].Players {
					if ps.Idx >= 0 && ps.Idx < len(data.Stats) {
						data.Stats[ps.Idx].R++
					}
				}
			}
			data.Rounds = append(data.Rounds, *cur)
		}
		cur = nil
		inRound = false
	})

	p.RegisterEventHandler(func(e events.Kill) {
		if cur == nil || e.Killer == nil || e.Victim == nil {
			return
		}
		tick := p.GameState().IngameTick()
		ap := e.Killer.Position()
		vp := e.Victim.Position()
		var wep string
		if e.Weapon != nil {
			wep = e.Weapon.Type.String()
		}
		ai := getIdx(e.Killer)
		vi := getIdx(e.Victim)
		asi := -1
		if e.Assister != nil {
			asi = getIdx(e.Assister)
		}
		cur.Kills = append(cur.Kills, Kill{
			Tick:        tick,
			AtkIdx:      ai,
			VicIdx:      vi,
			Weapon:      wep,
			HS:          e.IsHeadshot,
			AtkX:        iround(ap.X),
			AtkY:        iround(ap.Y),
			VicX:        iround(vp.X),
			VicY:        iround(vp.Y),
			AssisterIdx: asi,
			FlashAssist: e.AssistedFlash,
		})
		// Accumulate match stats.
		if ai >= 0 && ai < len(data.Stats) {
			data.Stats[ai].K++
			if e.IsHeadshot {
				data.Stats[ai].HS++
			}
		}
		if vi >= 0 && vi < len(data.Stats) {
			data.Stats[vi].D++
		}
	})

	p.RegisterEventHandler(func(e events.PlayerHurt) {
		if cur == nil || e.Attacker == nil || e.Player == nil {
			return
		}
		if e.Attacker.Team == e.Player.Team {
			return // skip self and team damage
		}
		ai := getIdx(e.Attacker)
		vi := getIdx(e.Player)
		if ai >= 0 && ai < len(data.Stats) {
			data.Stats[ai].DMG += e.HealthDamage
			cur.Dmg = append(cur.Dmg, [2]int{ai, e.HealthDamage})
		}
		if ai >= 0 && vi >= 0 {
			if roundVicDmg[ai] == nil {
				roundVicDmg[ai] = map[int]int{}
			}
			roundVicDmg[ai][vi] += e.HealthDamage
		}
	})

	// ── Bomb events ──────────────────────────────────────────────────────────

	p.RegisterEventHandler(func(e events.BombPlantBegin) {
		if cur == nil || e.Player == nil {
			return
		}
		tick := p.GameState().IngameTick()
		pos := e.Player.Position()
		bombX, bombY = iround(pos.X), iround(pos.Y)
		bombSite = string(rune(e.Site))
		cur.Bomb = append(cur.Bomb, BombAction{Tick: tick, Action: 0, X: bombX, Y: bombY, Site: bombSite})
	})

	p.RegisterEventHandler(func(e events.BombPlanted) {
		if cur == nil || e.Player == nil {
			return
		}
		tick := p.GameState().IngameTick()
		pos := e.Player.Position()
		bombX, bombY = iround(pos.X), iround(pos.Y)
		bombSite = string(rune(e.Site))
		cur.Bomb = append(cur.Bomb, BombAction{Tick: tick, Action: 1, X: bombX, Y: bombY, Site: bombSite})
	})

	p.RegisterEventHandler(func(e events.BombDefuseStart) {
		if cur == nil || e.Player == nil {
			return
		}
		tick := p.GameState().IngameTick()
		cur.Bomb = append(cur.Bomb, BombAction{Tick: tick, Action: 2, X: bombX, Y: bombY, Site: bombSite})
	})

	p.RegisterEventHandler(func(e events.BombDefused) {
		if cur == nil || e.Player == nil {
			return
		}
		tick := p.GameState().IngameTick()
		cur.Bomb = append(cur.Bomb, BombAction{Tick: tick, Action: 3, X: bombX, Y: bombY, Site: string(rune(e.Site))})
	})

	p.RegisterEventHandler(func(e events.BombExplode) {
		if cur == nil {
			return
		}
		tick := p.GameState().IngameTick()
		cur.Bomb = append(cur.Bomb, BombAction{Tick: tick, Action: 4, X: bombX, Y: bombY, Site: bombSite})
	})

	p.RegisterEventHandler(func(e events.BombDropped) {
		if cur == nil || e.Player == nil {
			return
		}
		tick := p.GameState().IngameTick()
		pos := e.Player.Position()
		bombX, bombY = iround(pos.X), iround(pos.Y)
		cur.Bomb = append(cur.Bomb, BombAction{Tick: tick, Action: 5, X: bombX, Y: bombY, Site: bombSite})
	})

	p.RegisterEventHandler(func(e events.BombPickup) {
		if cur == nil {
			return
		}
		tick := p.GameState().IngameTick()
		cur.Bomb = append(cur.Bomb, BombAction{Tick: tick, Action: 6, X: bombX, Y: bombY, Site: bombSite})
	})

	// ── Grenade events ───────────────────────────────────────────────────────

	p.RegisterEventHandler(func(e events.SmokeStart) {
		if cur == nil {
			return
		}
		tick := p.GameState().IngameTick()
		smokeType := 0 // generic / unknown team
		if e.Thrower != nil {
			if e.Thrower.Team == common.TeamCounterTerrorists {
				smokeType = 4 // CT smoke
			} else if e.Thrower.Team == common.TeamTerrorists {
				smokeType = 5 // T smoke
			}
		}
		cur.Grenades = append(cur.Grenades, Grenade{
			StartTick:  tick,
			EndTick:    tick + 1152, // ~18 s at 64 ticks/s
			Type:       smokeType,
			X:          iround(e.Position.X),
			Y:          iround(e.Position.Y),
			ThrowerIdx: getIdx(e.Thrower),
		})
	})

	p.RegisterEventHandler(func(e events.HeExplode) {
		if cur == nil {
			return
		}
		tick := p.GameState().IngameTick()
		cur.Grenades = append(cur.Grenades, Grenade{
			StartTick:  tick,
			EndTick:    0,
			Type:       2,
			X:          iround(e.Position.X),
			Y:          iround(e.Position.Y),
			ThrowerIdx: getIdx(e.Thrower),
		})
	})

	p.RegisterEventHandler(func(e events.FlashExplode) {
		if cur == nil {
			return
		}
		tick := p.GameState().IngameTick()
		cur.Grenades = append(cur.Grenades, Grenade{
			StartTick:  tick,
			EndTick:    0,
			Type:       1,
			X:          iround(e.Position.X),
			Y:          iround(e.Position.Y),
			ThrowerIdx: getIdx(e.Thrower),
		})
	})

	p.RegisterEventHandler(func(e events.InfernoStart) {
		if cur == nil {
			return
		}
		tick := p.GameState().IngameTick()
		pos := e.Inferno.Entity.Position()
		cur.Grenades = append(cur.Grenades, Grenade{
			StartTick:  tick,
			EndTick:    tick + 448, // ~7 s at 64 ticks/s
			Type:       3,
			X:          iround(pos.X),
			Y:          iround(pos.Y),
			ThrowerIdx: lastMolotovThrowerIdx, // set by GrenadeProjectileDestroy just before
		})
		lastMolotovThrowerIdx = -1
	})

	// ── Grenade trajectory (throw arc) ──────────────────────────────────────

	p.RegisterEventHandler(func(e events.GrenadeProjectileThrow) {
		if cur == nil || e.Projectile == nil {
			return
		}
		pi := -1
		if e.Projectile.Thrower != nil {
			pi = getIdx(e.Projectile.Thrower)
		}
		pendingThrows[e.Projectile.UniqueID()] = struct{ tick, throwerIdx int }{p.GameState().IngameTick(), pi}
	})

	p.RegisterEventHandler(func(e events.GrenadeProjectileDestroy) {
		if cur == nil || e.Projectile == nil || e.Projectile.WeaponInstance == nil {
			return
		}
		gt := equipToGrenadeType(e.Projectile.WeaponInstance.Type)
		if gt < 0 {
			return
		}
		// Team-coloured smokes
		if gt == 0 && e.Projectile.Thrower != nil {
			if e.Projectile.Thrower.Team == common.TeamCounterTerrorists {
				gt = 4
			} else if e.Projectile.Thrower.Team == common.TeamTerrorists {
				gt = 5
			}
		}
		// Track molotov thrower so InfernoStart (fired next) can pick it up.
		if gt == 3 && e.Projectile.Thrower != nil {
			lastMolotovThrowerIdx = getIdx(e.Projectile.Thrower)
		}
		uid := e.Projectile.UniqueID()
		info, ok := pendingThrows[uid]
		if !ok {
			return // no recorded throw, skip
		}
		delete(pendingThrows, uid)
		startTick := info.tick
		traj := e.Projectile.Trajectory2
		if len(traj) < 2 {
			return
		}
		// Subsample to at most 80 points
		step := 1
		if len(traj) > 80 {
			step = len(traj) / 80
		}
		points := make([][3]int, 0, 80)
		for i := 0; i < len(traj); i += step {
			te := traj[i]
			tickOff := int(math.Round(te.Time.Seconds()*64)) - startTick
			if tickOff < 0 {
				tickOff = 0
			}
			points = append(points, [3]int{tickOff, iround(te.Position.X), iround(te.Position.Y)})
		}
		// Always include the final point
		last := traj[len(traj)-1]
		lastOff := int(math.Round(last.Time.Seconds()*64)) - startTick
		if lastOff < 0 {
			lastOff = 0
		}
		if points[len(points)-1][0] != lastOff {
			points = append(points, [3]int{lastOff, iround(last.Position.X), iround(last.Position.Y)})
		}
		cur.Trails = append(cur.Trails, GrenadeTrail{
			StartTick:  startTick,
			EndTick:    p.GameState().IngameTick(),
			Type:       gt,
			ThrowerIdx: info.throwerIdx,
			Points:     points,
		})
	})

	// ── Weapon fire (deduplicated per player per SampleTicks window) ─────────

	p.RegisterEventHandler(func(e events.WeaponFire) {
		if cur == nil || e.Shooter == nil {
			return
		}
		tick := p.GameState().IngameTick()
		pi := getIdx(e.Shooter)
		if last, ok := lastShot[pi]; ok && tick-last < SampleTicks {
			return
		}
		cur.Shots = append(cur.Shots, Shot{Tick: tick, PIdx: pi})
		lastShot[pi] = tick
	})

	for {
		ok, err := p.ParseNextFrame()
		if err != nil {
			return nil, fmt.Errorf("parse demo: %w", err)
		}

		if inRound && cur != nil {
			tick := p.GameState().IngameTick()
			// Skip freeze time; freezeEndTick == 0 means freeze hasn't ended yet.
			// Also skip if tick hasn't advanced — full-snapshot (DEM_FullPacket) packets
			// replay the same tick and would create duplicate frames, causing a periodic
			// 1-frame freeze in playback every ~64 ticks (1 s).
			if freezeEndTick > 0 && tick >= freezeEndTick && tick > lastSampledTick && tick%SampleTicks == 0 {
				if f := captureFrame(tick); len(f.Players) > 0 {
					cur.Frames = append(cur.Frames, f)
					lastSampledTick = tick
				}
			}
		}

		if !ok {
			break
		}
	}

	data.MapName = p.Header().MapName
	return data, nil
}
