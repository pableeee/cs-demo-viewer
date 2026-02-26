package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pable/cs-demo-viewer/internal/demo"
	"github.com/pable/cs-demo-viewer/internal/maps"
	"github.com/pable/cs-demo-viewer/internal/viewer"
)

// uniqueOutPath returns outDir/base.html, or outDir/base_2.html etc. if the file already exists.
func uniqueOutPath(outDir, base string) string {
	p := filepath.Join(outDir, base+".html")
	if _, err := os.Stat(p); err != nil {
		return p // doesn't exist yet
	}
	for n := 2; ; n++ {
		p = filepath.Join(outDir, fmt.Sprintf("%s_%d.html", base, n))
		if _, err := os.Stat(p); err != nil {
			return p
		}
	}
}

func main() {
	out := flag.String("o", "", "output file (single mode) or output directory (dir mode); default: alongside input")
	dir := flag.String("dir", "", "process all .dem files in this directory")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: demoview [flags] <demo.dem>\n")
		fmt.Fprintf(os.Stderr, "       demoview -dir <directory> [-o <outdir>]\n\n")
		fmt.Fprintf(os.Stderr, "Generates a self-contained HTML round-replay viewer from a CS2 demo.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *dir != "" {
		// Bulk mode: process every .dem in the directory.
		entries, err := os.ReadDir(*dir)
		if err != nil {
			log.Fatalf("read dir: %v", err)
		}
		outDir := *out
		if outDir == "" {
			outDir = *dir
		}
		if err := os.MkdirAll(outDir, 0755); err != nil {
			log.Fatalf("create output dir: %v", err)
		}
		ok, fail := 0, 0
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".dem") {
				continue
			}
			demoFile := filepath.Join(*dir, e.Name())
			if err := processDemoFile(demoFile, outDir, true); err != nil {
				log.Printf("SKIP %s: %v", e.Name(), err)
				fail++
			} else {
				ok++
			}
		}
		log.Printf("done: %d succeeded, %d failed/skipped", ok, fail)
		return
	}

	// Single mode.
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}
	demoFile := flag.Arg(0)
	outputFile := *out
	if outputFile == "" {
		outputFile = replaceExt(demoFile, ".html")
	}
	if err := processDemoTo(demoFile, outputFile); err != nil {
		log.Fatal(err)
	}
}

// processDemoFile parses a demo and writes an HTML file.
// In bulk mode the output filename is "<outDir>/<basename>_<mapname>.html".
// In single mode outDir is ignored and the exact outputFile path is used instead.
func processDemoFile(demoFile, outDir string, bulk bool) error {
	f, err := os.Open(demoFile)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	log.Printf("parsing %s ...", demoFile)
	d, err := demo.Parse(f)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	log.Printf("  map: %s  rounds: %d  players: %d", d.MapName, len(d.Rounds), len(d.Players))

	meta, ok := maps.GetMeta(d.MapName)
	if !ok {
		return fmt.Errorf("unsupported map %q", d.MapName)
	}

	radarPNG, err := maps.RadarPNG(d.MapName)
	if err != nil {
		return fmt.Errorf("radar PNG: %w", err)
	}

	lower, hasLower := maps.GetLower(d.MapName)
	var radarLowerPNG []byte
	if hasLower {
		radarLowerPNG, err = maps.RadarPNGLower(d.MapName)
		if err != nil {
			return fmt.Errorf("lower radar PNG: %w", err)
		}
	}

	var outputFile string
	if bulk {
		fi, err := os.Stat(demoFile)
		if err != nil {
			return fmt.Errorf("stat: %w", err)
		}
		date := fi.ModTime().Format("2006-01-02")
		base := date + "_" + d.MapName
		outputFile = uniqueOutPath(outDir, base)
	} else {
		outputFile = outDir // outDir holds the exact path in single mode
	}

	out, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer out.Close()

	if err := viewer.Write(out, d, meta, radarPNG, radarLowerPNG, lower, hasLower); err != nil {
		return fmt.Errorf("generate HTML: %w", err)
	}

	log.Printf("  wrote %s", outputFile)
	return nil
}

// processDemoTo is the single-file entry point with an explicit output path.
func processDemoTo(demoFile, outputFile string) error {
	return processDemoFile(demoFile, outputFile, false)
}

func replaceExt(path, ext string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[:i] + ext
	}
	return path + ext
}
