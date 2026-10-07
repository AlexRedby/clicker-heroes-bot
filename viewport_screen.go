package main

import (
	"fmt"
	"image"
)

type windowCapture struct{ geometry viewportGeometry }

func viewportHUD(screen image.Image) bool {
	if screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return false
	}
	_, settings, err := saveControl(screen, 2)
	if err != nil || !settings {
		return false
	}
	known, _, err := progressionMode(screen)
	return err == nil && (known || heroQuantityBarPresent(screen))
}

func findViewport(screen image.Image, window image.Rectangle) (image.Rectangle, error) {
	var found image.Rectangle
	for _, candidate := range viewportCandidates(window) {
		if !candidate.In(screen.Bounds()) || !viewportHUD(compactCrop(screen, candidate)) {
			continue
		}
		if !found.Empty() {
			return image.Rectangle{}, fmt.Errorf("ambiguous game viewport")
		}
		found = candidate
	}
	if found.Empty() {
		return found, fmt.Errorf("game viewport HUD not recognized; show an unobscured HUD")
	}
	return found, nil
}

func (v *windowCapture) capture() (image.Image, error) {
	unavailable := func(reason string) (image.Image, error) {
		return &viewportImage{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), reason: reason,
			geometry: viewportGeometry{Scene: nativeScene{Window: "!outside-game"}}}, nil
	}
	scene, err := readNativeScene()
	if err != nil {
		return unavailable(err.Error())
	}
	screen, err := captureNativeDisplay(scene)
	if err != nil {
		return unavailable(fmt.Sprintf("capture game display: %v", err))
	}
	if screen == nil || screen.Bounds().Empty() {
		return unavailable("capture returned no pixels")
	}
	after, err := readNativeScene()
	if err != nil || scene != after {
		return unavailable("window or display changed during capture")
	}
	geometry := viewportGeometry{Scene: scene, Capture: screen.Bounds()}
	if v.geometry.Scene == scene && v.geometry.Capture == geometry.Capture {
		geometry.ROI = v.geometry.ROI
	} else {
		window, err := windowPixels(scene, screen.Bounds())
		if err != nil {
			return unavailable(err.Error())
		}
		geometry.ROI, err = findViewport(screen, window)
		if err != nil {
			return unavailable(err.Error())
		}
	}
	crop := compactCrop(screen, geometry.ROI)
	c, err := recognizedGame(crop)
	if err != nil || !c.known {
		return unavailable("game viewport context not recognized")
	}
	// A normal HUD must keep its anchors; existing modals may dim them.
	if !c.saveMenu && !c.ancientDialog && !c.ascension && !c.questDialog && !c.mercenaryDialog && !c.relicJunk && c.modal == noGildModal && !viewportHUD(crop) {
		return unavailable("game viewport anchors changed")
	}
	if geometry != v.geometry {
		fmt.Printf("viewport: window=%s display=%d desktop=%v capture=%v ROI=%v DPI=%d\n", scene.Window, scene.Display, scene.Desktop, geometry.Capture, geometry.ROI, scene.DPI)
	}
	v.geometry = geometry
	return &viewportImage{Image: crop, geometry: geometry}, nil
}
