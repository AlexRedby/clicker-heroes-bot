//go:build !windows && !darwin

package bot

import (
	"fmt"
	"image"
)

func windowedPreflight() error                              { return fmt.Errorf("-windowed requires Windows or macOS") }
func readNativeScene() (nativeScene, error)                 { return nativeScene{}, windowedPreflight() }
func captureNativeDisplay(nativeScene) (image.Image, error) { return nil, windowedPreflight() }
func nativeTargetVisible(nativeScene, image.Point) bool     { return false }
func focusNativeWindow(string) error                        { return windowedPreflight() }
func nativeMousePosition() (image.Point, error)             { return image.Point{}, windowedPreflight() }
func nativeMouseArgument(image.Point) (image.Point, error)  { return image.Point{}, windowedPreflight() }

func windowedHookPreflight() error { return nil }
