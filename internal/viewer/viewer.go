package viewer

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/pable/cs-demo-viewer/internal/demo"
	"github.com/pable/cs-demo-viewer/internal/maps"
)

//go:embed template.html
var templateHTML string

// ViewerData is everything the HTML template needs.
type ViewerData struct {
	MapName    string            `json:"map"`
	Meta       mapMeta           `json:"meta"`
	Radar      string            `json:"radar"`       // "data:image/png;base64,..."
	RadarLower string            `json:"radar_lower"` // "" if no lower level
	HasLower   bool              `json:"has_lower"`
	LowerZMax  float64           `json:"lower_z_max"` // z threshold for lower level
	Players    []demo.PlayerInfo `json:"players"`
	Rounds     []demo.Round      `json:"rounds"`
	Stats      []demo.PlayerStat `json:"stats"` // parallel to Players
}

type mapMeta struct {
	PosX  float64 `json:"pos_x"`
	PosY  float64 `json:"pos_y"`
	Scale float64 `json:"scale"`
}

// Write generates the self-contained HTML viewer and writes it to w.
func Write(w io.Writer, d *demo.DemoData, meta maps.Meta, radarPNG []byte, radarLowerPNG []byte, lower maps.Lower, hasLower bool) error {
	vd := ViewerData{
		MapName: d.MapName,
		Meta: mapMeta{
			PosX:  meta.PosX,
			PosY:  meta.PosY,
			Scale: meta.Scale,
		},
		Radar:    "data:image/png;base64," + base64.StdEncoding.EncodeToString(radarPNG),
		Players:  d.Players,
		Rounds:   d.Rounds,
		Stats:    d.Stats,
		HasLower: hasLower,
	}
	if hasLower && radarLowerPNG != nil {
		vd.RadarLower = "data:image/png;base64," + base64.StdEncoding.EncodeToString(radarLowerPNG)
		vd.LowerZMax = lower.ZMax
	}

	jsonBytes, err := json.Marshal(vd)
	if err != nil {
		return fmt.Errorf("marshal viewer data: %w", err)
	}

	html := strings.Replace(templateHTML, "/*INJECT_DATA*/", string(jsonBytes), 1)
	_, err = io.WriteString(w, html)
	return err
}
