package main

import (
	"context"
	_ "embed"
	"fmt"
	"image"
	"time"
)

//go:embed assets/save-controls.png
var saveControlsPNG []byte
var saveControlsImage decodedPNG

func saveControl(screen image.Image, which int) (image.Point, bool, error) {
	// Icon patches stay inside the opaque artwork, excluding surrounding scenery.
	regions := [...]image.Rectangle{image.Rect(290, 192, 449, 216), image.Rect(1006, 137, 1034, 165), image.Rect(1233, 17, 1255, 39)}
	refs := [...]image.Rectangle{image.Rect(0, 0, 318, 48), image.Rect(0, 48, 56, 104), image.Rect(0, 104, 44, 148)}
	atlas, err := saveControlsImage.get(saveControlsPNG)
	if err != nil {
		return image.Point{}, false, err
	}
	found, err := matchControl(screen, regions[which], atlas, refs[which])
	r := controlRect(screen, regions[which])
	return r.Min.Add(r.Size().Div(2)), found, err
}
func saveMenu(screen image.Image) bool {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return false
	}
	for _, point := range []image.Point{{270, 170}, {1005, 550}} {
		p := controlRect(screen, image.Rect(point.X, point.Y, point.X+1, point.Y+1)).Min
		r, g, b := rgb(screen.At(p.X, p.Y))
		if r < 230 || g < 220 || b < 150 {
			return false
		}
	}
	// Save can remain highlighted after exporting; the menu X is independent.
	_, found, err := saveControl(screen, 1)
	return found && err == nil
}

type saveExportOptions struct {
	dir, reserve, planOutput string
	skillRate                float64
	beyond8k                 bool
}
type exportStep uint8

const (
	exportOpenMenu exportStep = iota
	exportSave
	exportRestoreGame
	exportCloseMenu
	exportReadFile
)

func (step exportStep) String() string {
	return [...]string{"open menu", "Save", "restore game focus", "close menu", "read fresh export"}[step]
}

type exportCommand struct {
	step   exportStep
	window string
	before exportSnapshot
}
type exportJob struct {
	ctx     context.Context
	options saveExportOptions
	before  exportSnapshot
}
type saveExporter struct {
	requested, active, waiting bool
	step                       exportStep
	window                     string
	status                     string
	before                     exportSnapshot
	lastFrame, jobFrame        uint64
	deadline                   time.Time
	cancel                     context.CancelFunc
}

func (e *saveExporter) interrupt() {
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	e.active, e.waiting = false, false
	e.before = nil
	e.lastFrame, e.jobFrame = 0, 0
}
func (p *gamePipeline) exportFailed(err error) {
	reason := fmt.Sprintf("save export failed at %s: %v", p.export.step, err)
	if p.export.status != "" {
		reason += " (" + p.export.status + ")"
	}
	reason += "; close any dialog, focus the game and press F8 to retry"
	p.export.interrupt()
	p.controls.pause(reason)
	if p.frame.image != nil && p.frame.image.Bounds().Dx() >= 640 {
		path := fmt.Sprintf("artifacts/save-export-failure-%s.png", time.Now().Format("20060102-150405.000"))
		if err := saveImage(path, p.frame.image); err != nil {
			fmt.Printf("save export diagnostic failed: %v\n", err)
		} else {
			fmt.Println("saved save export failure screenshot:", path)
		}
	}
}
func (p *gamePipeline) planExport(now time.Time) bool {
	e := &p.export
	if p.options.export == nil || !e.requested {
		return p.frame.context.saveMenu
	}
	if p.frame.id == 0 {
		return true
	}
	if !e.active {
		if !p.frame.context.known || (!p.frame.context.heroes && !p.frame.context.ancients && !p.frame.context.saveMenu) {
			return true
		}
		if p.frame.context.window == "" || p.frame.context.window == "!outside-game" {
			p.exportFailed(fmt.Errorf("game process identity unavailable"))
			return true
		}
		e.active, e.step, e.window = true, exportOpenMenu, p.frame.context.window
		e.deadline = now.Add(20 * time.Second)
		p.queue = make(map[actionKind]gameAction)
		fmt.Println("save export: started")
	}
	e.status = fmt.Sprintf("expected window=%q, observed window=%q, recognized=%t, menu=%t", e.window, p.frame.context.window, p.frame.context.known, p.frame.context.saveMenu)
	if now.After(e.deadline) {
		p.exportFailed(fmt.Errorf("transition timed out"))
		return true
	}
	if p.frame.id <= e.lastFrame {
		return true
	}
	c := p.frame.context
	// Save opens Explorer asynchronously. Restoring focus before it opens
	// would let its later activation steal focus during the purchase batch.
	if e.step == exportRestoreGame && c.window == e.window {
		return true
	}
	if c.window != e.window && e.step != exportRestoreGame && e.step != exportCloseMenu {
		p.exportFailed(fmt.Errorf("game window changed"))
		return true
	}
	if e.step == exportOpenMenu && c.saveMenu {
		e.step, e.waiting = exportSave, false
	}
	if e.step == exportCloseMenu {
		if c.window != e.window {
			return true
		}
		if !c.saveMenu && c.known && (c.heroes || c.ancients) {
			e.step, e.waiting = exportReadFile, false
			e.deadline = now.Add(30 * time.Second)
		}
	}
	if e.step == exportReadFile || e.waiting {
		return true
	}
	cmd := &exportCommand{step: e.step, window: e.window}
	a := gameAction{kind: handleExport, frame: p.frame, export: cmd}
	if e.step != exportRestoreGame {
		which := 2
		if e.step == exportSave {
			which = 0
		}
		if e.step == exportCloseMenu {
			which = 1
		}
		point, found, err := saveControl(p.frame.image, which)
		if err != nil {
			p.exportFailed(err)
			return true
		}
		if !found {
			e.status += ", control not recognized"
			return true
		}
		a.point = point
	}
	p.enqueue(a, now)
	return true
}
func (e *saveExporter) sent(a gameAction, now time.Time) {
	e.lastFrame = a.frame.id
	e.waiting = true
	e.deadline = now.Add(20 * time.Second)
	fmt.Printf("save export: %s\n", a.export.step)
	switch a.export.step {
	case exportSave:
		e.before = a.export.before
		e.step, e.waiting = exportRestoreGame, false
	case exportRestoreGame:
		e.step, e.waiting = exportCloseMenu, false
	}
}
func readExportPlan(job exportJob) (*ancientPlan, error) {
	var last error
	for {
		data, path, err := readFreshExport(job.ctx, job.options.dir, job.before)
		if err != nil {
			if last != nil {
				return nil, fmt.Errorf("fresh export could not be decoded: %v (%w)", last, err)
			}
			return nil, fmt.Errorf("waiting for new or changed clickerHeroSave*.txt in %q: %w", job.options.dir, err)
		}
		value, err := calculateAncientData(job.ctx, data, path, job.options.reserve, job.options.skillRate, job.options.beyond8k)
		if err == nil {
			return &value, nil
		}
		// A writer can pause between chunks longer than the file stability window.
		// Only a fully decoded save is eligible for purchases.
		last = err
	}
}
