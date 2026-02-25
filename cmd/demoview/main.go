package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/pable/cs-demo-viewer/internal/demo"
	"github.com/pable/cs-demo-viewer/internal/maps"
	"github.com/pable/cs-demo-viewer/internal/viewer"
)

func main() {
	out := flag.String("o", "", "output HTML file (default: <demo>.html)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: demoview [flags] <demo.dem>\n\n")
		fmt.Fprintf(os.Stderr, "Generates a self-contained HTML round-replay viewer from a CS2 demo.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	demoFile := flag.Arg(0)
	outputFile := *out
	if outputFile == "" {
		if strings.HasSuffix(demoFile, ".dem") {
			outputFile = demoFile[:len(demoFile)-4] + ".html"
		} else {
			outputFile = demoFile + ".html"
		}
	}

	f, err := os.Open(demoFile)
	if err != nil {
		log.Fatalf("open demo: %v", err)
	}
	defer f.Close()

	log.Printf("parsing %s ...", demoFile)
	d, err := demo.Parse(f)
	if err != nil {
		log.Fatalf("parse demo: %v", err)
	}
	log.Printf("map: %s  rounds: %d  players: %d", d.MapName, len(d.Rounds), len(d.Players))

	meta, ok := maps.GetMeta(d.MapName)
	if !ok {
		log.Fatalf("unsupported map %q — supported maps: de_ancient, de_anubis, de_dust2, de_inferno, de_mirage, de_nuke, de_overpass, de_train, de_vertigo", d.MapName)
	}

	radarPNG, err := maps.RadarPNG(d.MapName)
	if err != nil {
		log.Fatalf("radar PNG: %v", err)
	}

	lower, hasLower := maps.GetLower(d.MapName)
	var radarLowerPNG []byte
	if hasLower {
		radarLowerPNG, err = maps.RadarPNGLower(d.MapName)
		if err != nil {
			log.Fatalf("lower radar PNG: %v", err)
		}
	}

	outFile, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("create output: %v", err)
	}
	defer outFile.Close()

	if err := viewer.Write(outFile, d, meta, radarPNG, radarLowerPNG, lower, hasLower); err != nil {
		log.Fatalf("generate HTML: %v", err)
	}

	log.Printf("wrote %s", outputFile)
}
