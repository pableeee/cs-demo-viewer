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
}

// PlayerInfo is the static info for a player (referenced by index in frames/kills).
type PlayerInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Round contains all sampled frames and kills for one round.
type Round struct {
	Num    int     `json:"n"`
	Winner string  `json:"w"` // "CT", "T", or ""
	Frames []Frame `json:"frames"`
	Kills  []Kill  `json:"kills"`
}

// Frame is one sampled tick's snapshot of all player states.
type Frame struct {
	Tick    int           `json:"tick"`
	Players []PlayerState `json:"p"`
}

// PlayerState is one player's state at a sampled tick, serialized as a compact JSON array:
// [idx, flags, hp, x, y, z, yaw]
// flags: 0=CT+alive, 1=CT+dead, 2=T+alive, 3=T+dead
// x, y, z: world coords rounded to nearest integer
// yaw: degrees rounded to nearest integer
type PlayerState struct {
	Idx   int
	Flags int // 0=CT+alive 1=CT+dead 2=T+alive 3=T+dead
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
// [tick, atkIdx, vicIdx, weapon, headshot(0/1), atkX, atkY, vicX, vicY]
type Kill struct {
	Tick   int
	AtkIdx int
	VicIdx int
	Weapon string
	HS     bool
	AtkX   int
	AtkY   int
	VicX   int
	VicY   int
}

func (k Kill) MarshalJSON() ([]byte, error) {
	hs := 0
	if k.HS {
		hs = 1
	}
	return json.Marshal([]any{k.Tick, k.AtkIdx, k.VicIdx, k.Weapon, hs, k.AtkX, k.AtkY, k.VicX, k.VicY})
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
	var freezeEndTick int // only sample frames after freeze ends

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
		return i
	}

	captureFrame := func(tick int) Frame {
		frame := Frame{Tick: tick}
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
		cur = &Round{Num: roundNum}
		freezeEndTick = 0 // will be set by RoundFreezetimeEnd
		inRound = true
	})

	p.RegisterEventHandler(func(e events.RoundFreezetimeEnd) {
		if cur == nil {
			return
		}
		freezeEndTick = p.GameState().IngameTick()
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
		// Capture a final frame at the round-end tick so the last kill flash renders.
		tick := p.GameState().IngameTick()
		if f := captureFrame(tick); len(f.Players) > 0 {
			cur.Frames = append(cur.Frames, f)
		}
		// Only keep rounds with meaningful live-play data.
		if len(cur.Frames) >= 5 {
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
		cur.Kills = append(cur.Kills, Kill{
			Tick:   tick,
			AtkIdx: getIdx(e.Killer),
			VicIdx: getIdx(e.Victim),
			Weapon: wep,
			HS:     e.IsHeadshot,
			AtkX:   iround(ap.X),
			AtkY:   iround(ap.Y),
			VicX:   iround(vp.X),
			VicY:   iround(vp.Y),
		})
	})

	for {
		ok, err := p.ParseNextFrame()
		if err != nil {
			return nil, fmt.Errorf("parse demo: %w", err)
		}

		if inRound && cur != nil {
			tick := p.GameState().IngameTick()
			// Skip freeze time; freezeEndTick == 0 means freeze hasn't ended yet.
			if freezeEndTick > 0 && tick >= freezeEndTick && tick%SampleTicks == 0 {
				if f := captureFrame(tick); len(f.Players) > 0 {
					cur.Frames = append(cur.Frames, f)
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
