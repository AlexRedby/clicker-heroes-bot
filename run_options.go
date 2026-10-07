package main

import (
	"errors"
	"math"
)

// Progression is one complete gameplay cycle; individual pipeline switches remain
// internal so analyzers and transactions can be tested independently.
func configureRun(options pipelineOptions) (pipelineOptions, error) {
	if options.progression {
		if options.export == nil || options.export.dir == "" {
			return options, errors.New("-progression requires -export-dir for fresh Ancient plans after Ascension")
		}
		options.heroes, options.skills, options.autoClickers = true, true, true
		options.gilds, options.ascension = true, true
	}
	if options.achievements != nil && (!options.mercenaries || options.export == nil || options.export.dir == "") {
		return options, errors.New("-achievement-goals in run requires -mercenaries and -export-dir")
	}
	if options.ascension && (options.ascensionStall <= 0 || options.ascensionMinGain <= 0 || math.IsNaN(options.ascensionMinGain) || math.IsInf(options.ascensionMinGain, 0)) {
		return options, errors.New("-ascension-stall must be positive; -ascension-min-gain must be finite and positive")
	}
	if options.gilds && options.gildInterval <= 0 {
		return options, errors.New("-gild-interval must be positive")
	}
	if ocrTimeout <= 0 {
		return options, errors.New("-ocr-timeout must be positive")
	}
	if options.fishInterval <= 0 || options.monster && options.clickInterval <= 0 {
		return options, errors.New("-fish-interval must be positive; -interval must be positive when monster clicks are enabled")
	}
	return options, nil
}
