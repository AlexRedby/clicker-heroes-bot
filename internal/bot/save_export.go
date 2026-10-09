package bot

import (
	"context"
	"errors"
	"fmt"
	"image"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/transcension"
	"clicker-heroes-bot/internal/vision"
)

func saveControl(screen image.Image, which int) (image.Point, bool, error) {
	// Icon patches stay inside the opaque artwork, excluding surrounding scenery.
	// The settings patch aligns to the 4:1 sampling grid for 640px HUDs.
	regions := [...]image.Rectangle{image.Rect(290, 192, 449, 216), image.Rect(1006, 137, 1034, 165), image.Rect(1232, 16, 1256, 40)}
	names := [...]string{"ui/save.png", "ui/menu-close.png", "ui/settings.png"}
	found, err := vision.MatchControl(screen, regions[which], names[which])
	r := vision.Rect(screen, regions[which])
	return r.Min.Add(r.Size().Div(2)), found, err
}
func saveMenu(screen image.Image) bool {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return false
	}
	for _, point := range []image.Point{{270, 170}, {1005, 550}} {
		p := vision.Rect(screen, image.Rect(point.X, point.Y, point.X+1, point.Y+1)).Min
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
	ctx                     context.Context
	options                 saveExportOptions
	before                  exportSnapshot
	relicsOnly              bool
	goalsOnly, achievements bool
	initialSetup            bool
	prestige, prestigeOnly  bool
	gildsOnly               bool
	restoreAllocation       bool
	generation              uint64
}
type exportResult struct {
	at                  time.Time
	transcension        *transcension.Snapshot
	transcensionErr     error
	achievements        *ancientcalc.AchievementState
	achievementErr      error
	plan                *ancientPlan
	gilds               *ancientcalc.GildPlan
	gildErr             error
	prestige            *ancientcalc.TranscensionPreview
	relics              *ancientcalc.RelicPreview
	relicErr            error
	heroSetup           *ancientcalc.HeroSetupPlan
	heroErr, ancientErr error
}

func (p *gamePipeline) reportRelics(preview *ancientcalc.RelicPreview, err error) {
	message := relicReport(preview, err)
	if message != p.relicMessage {
		fmt.Println(message)
		p.relicMessage = message
	}
}

type saveExporter struct {
	requested, active, waiting bool
	relicsOnly                 bool
	prestigeOnly               bool
	gildsOnly                  bool
	goalsOnly                  bool
	initialSetup               bool
	step                       exportStep
	window                     string
	status                     string
	before                     exportSnapshot
	lastFrame, jobFrame        uint64
	deadline, nextAction       time.Time
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
func (e *saveExporter) suspend() {
	step, window, before := e.step, e.window, e.before
	e.interrupt()
	if step >= exportRestoreGame && step < exportReadFile && before != nil {
		e.step, e.window, e.before = exportCloseMenu, window, before
	} else {
		e.step = exportOpenMenu
	}
	e.deadline, e.nextAction = time.Time{}, time.Time{}
}

// Save may finish before F8, while its completion reaches the coordinator later.
// Retain file ownership only, never an old observation or purchase decision.
func (p *gamePipeline) rememberCompletedSave(done actionResult) {
	a := done.action
	if done.acted && done.err == nil && a.kind == handleExport && a.export.step == exportSave &&
		a.export.before != nil && p.export.requested && p.export.window == a.export.window {
		p.export.before, p.export.step = a.export.before, exportCloseMenu
	}
}
func (p *gamePipeline) exportFailed(err error) {
	p.invalidateAchievementGoals()
	reason := fmt.Sprintf("save export failed at %s: %v", p.export.step, err)
	if p.export.status != "" {
		reason += " (" + p.export.status + ")"
	}
	reason += "; retrying navigation in 5s"
	p.export.suspend()
	p.export.nextAction = time.Now().Add(5 * time.Second)
	fmt.Println(reason)
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
	if p.frame.id == 0 || now.Before(e.nextAction) {
		return true
	}
	if !e.active {
		if e.before != nil && e.step == exportCloseMenu {
			e.active = true
		} else if !p.frame.context.known || (!p.frame.context.heroes && !p.frame.context.ancients && !p.frame.context.outsiders && !p.frame.context.saveMenu) {
			return true
		}
		if !e.active && (p.frame.context.window == "" || p.frame.context.window == "!outside-game") {
			p.exportFailed(fmt.Errorf("game process identity unavailable"))
			return true
		}
		if !e.active {
			e.active, e.step, e.window = true, exportOpenMenu, p.frame.context.window
		}
		e.deadline = now.Add(20 * time.Second)
		p.queue = make(map[actionKind]gameAction)
		if e.relicsOnly && p.relic.active {
			fmt.Println("save export: checking relic equipment")
		} else if e.relicsOnly {
			fmt.Println("save export: checking relics before Ascension (read only)")
		} else {
			fmt.Println("save export: started")
		}
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
			e.step, e.waiting = exportRestoreGame, false
		}
		if !c.saveMenu && c.known && (c.heroes || c.ancients || c.outsiders) {
			e.step, e.waiting = exportReadFile, false
			e.deadline = now.Add(30 * time.Second)
		}
	}
	if e.waiting && !now.Before(e.nextAction) {
		e.waiting = false // Repeating navigation is safe; Save itself is never replayed here.
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
	e.nextAction = now.Add(time.Second)
	fmt.Printf("save export: %s\n", a.export.step)
	switch a.export.step {
	case exportSave:
		e.deadline = now.Add(20 * time.Second)
		e.before = a.export.before
		e.step, e.waiting = exportRestoreGame, false
	case exportRestoreGame:
		e.step, e.waiting = exportCloseMenu, false
	}
}
func readExport(job exportJob) (exportResult, error) {
	var last error
	for {
		data, path, err := readFreshExport(job.ctx, job.options.dir, job.before)
		if err != nil {
			if last != nil {
				return exportResult{}, fmt.Errorf("fresh export could not be decoded: %v (%w)", last, err)
			}
			return exportResult{}, fmt.Errorf("waiting for new or changed clickerHeroSave*.txt in %q: %w", job.options.dir, err)
		}
		result := exportResult{at: time.Now()}
		prestige := job.prestige || job.prestigeOnly || job.restoreAllocation
		if prestige {
			value, err := transcension.ReadSnapshot(job.ctx, data, time.Now().UTC(), job.generation)
			result.transcensionErr = err
			if err != nil && job.prestigeOnly {
				last = err
				continue
			}
			if err == nil {
				result.transcension, result.prestige = &value, &value.Preview
			}
		}
		if job.achievements {
			value, err := ancientcalc.ReadAchievementState(job.ctx, data)
			result.achievementErr = err
			if err == nil {
				result.achievements = &value
			} else if job.goalsOnly {
				last = err
				continue
			}
		}
		if job.initialSetup {
			value, err := ancientcalc.PlanHeroSetup(job.ctx, data)
			if err != nil && !errors.Is(err, ancientcalc.ErrHeroSetupUnsupported) {
				last = err
				continue
			}
			result.heroErr = err
			if err == nil {
				result.heroSetup = &value
			}
		}
		allocate := !job.prestigeOnly
		if job.prestigeOnly && job.restoreAllocation && result.transcension != nil {
			missing, err := ancientcalc.MissingActiveAncients(job.ctx, result.transcension.State, job.options.skillRate, job.options.beyond8k)
			result.ancientErr = err
			allocate = err == nil && len(missing) == 0
		}
		if !job.relicsOnly && !job.goalsOnly && !job.gildsOnly && allocate {
			value, err := calculateAncientData(job.ctx, data, path, job.options.reserve, job.options.skillRate, job.options.beyond8k)
			if err != nil {
				if !job.initialSetup && !prestige {
					last = err
					continue
				}
				result.ancientErr = err
			} else {
				result.plan = &value
				if result.prestige == nil {
					result.prestige = value.Transcension
				}
			}
		}
		if result.prestige == nil && !job.goalsOnly && !prestige {
			// An advisory Outsider roster does not require owned Ancients and
			// cannot block a valid relic-only export when metadata is absent.
			value, err := ancientcalc.PreviewTranscension(job.ctx, data)
			if job.ctx.Err() != nil {
				return exportResult{}, job.ctx.Err()
			}
			if err == nil {
				result.prestige = &value
			}
		}
		if result.plan != nil {
			result.gilds = result.plan.Gilds
		} else if !job.goalsOnly && !job.prestigeOnly {
			value, err := ancientcalc.CalculateGilds(job.ctx, data, job.options.reserve)
			result.gildErr = err
			if err == nil {
				result.gilds = &value
			}
		}
		if !job.goalsOnly {
			preview, err := ancientcalc.PreviewRelics(job.ctx, data)
			if err == nil {
				result.relics = &preview
			} else if job.relicsOnly {
				// The writer can pause between chunks longer than the stability window.
				// A read-only check also waits for a complete, supported inventory.
				last = err
				continue
			}
			result.relicErr = err
		}
		if err := job.ctx.Err(); err != nil {
			return exportResult{}, err
		}
		if removed, err := removeGeneratedExport(job.ctx, path, job.before, data); err != nil {
			fmt.Printf("save export: cleanup failed for %q: %v\n", path, err)
		} else if removed {
			fmt.Println("save export: removed generated export:", path)
		}
		return result, nil
	}
}
