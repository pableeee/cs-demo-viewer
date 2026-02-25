package maps

import (
	_ "embed"
	"embed"
	"fmt"
)

//go:embed overviews/*.png
var overviewFS embed.FS

// Meta holds map coordinate metadata from the CS2 overview txt files.
type Meta struct {
	PosX  float64
	PosY  float64
	Scale float64
}

// Lower holds metadata for the lower level of multi-floor maps.
type Lower struct {
	Meta
	// ZMax is the world Z below which a player is considered on the lower level.
	ZMax float64
}

var metas = map[string]Meta{
	"de_ancient":  {PosX: -2953, PosY: 2164, Scale: 5.0},
	"de_anubis":   {PosX: -2796, PosY: 3328, Scale: 5.22},
	"de_dust2":    {PosX: -2476, PosY: 3239, Scale: 4.4},
	"de_inferno":  {PosX: -2087, PosY: 3870, Scale: 4.9},
	"de_mirage":   {PosX: -3230, PosY: 1713, Scale: 5.0},
	"de_nuke":     {PosX: -3453, PosY: 2887, Scale: 7.0},
	"de_overpass": {PosX: -4831, PosY: 1781, Scale: 5.2},
	"de_train":    {PosX: -2308, PosY: 2078, Scale: 4.082077},
	"de_vertigo":  {PosX: -3168, PosY: 1762, Scale: 4.0},
}

// Lower level metadata for multi-floor maps.
// PosX/PosY/Scale for lower levels are the same as upper in CS2 (shared radar space).
var lowers = map[string]Lower{
	// de_nuke lower: pit/lower bomb site is below z=-495
	"de_nuke": {Meta: metas["de_nuke"], ZMax: -495},
	// de_vertigo lower: scaffold level below z=11700
	"de_vertigo": {Meta: metas["de_vertigo"], ZMax: 11700},
	// de_train lower: underground below z=-130
	"de_train": {Meta: metas["de_train"], ZMax: -130},
}

// GetMeta returns coordinate metadata for a map. Second return is false if unknown.
func GetMeta(mapName string) (Meta, bool) {
	m, ok := metas[mapName]
	return m, ok
}

// GetLower returns lower-level metadata for multi-floor maps.
func GetLower(mapName string) (Lower, bool) {
	l, ok := lowers[mapName]
	return l, ok
}

// RadarPNG returns the PNG bytes for the upper radar of mapName.
func RadarPNG(mapName string) ([]byte, error) {
	path := fmt.Sprintf("overviews/%s.png", mapName)
	b, err := overviewFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no radar image for %q (supported: de_ancient, de_anubis, de_dust2, de_inferno, de_mirage, de_nuke, de_overpass, de_train, de_vertigo)", mapName)
	}
	return b, nil
}

// RadarPNGLower returns the PNG bytes for the lower-level radar of mapName.
// Returns nil, nil if the map has no lower level.
func RadarPNGLower(mapName string) ([]byte, error) {
	if _, ok := lowers[mapName]; !ok {
		return nil, nil
	}
	path := fmt.Sprintf("overviews/%s_lower.png", mapName)
	return overviewFS.ReadFile(path)
}
