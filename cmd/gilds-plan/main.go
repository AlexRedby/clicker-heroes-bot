// gilds-plan previews redistribution from a local save. It has no game input.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"clicker-heroes-bot/internal/ancientcalc"
)

func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > limit {
		return nil, errors.New("input must be a nonempty regular file within the size limit")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = errors.New("input exceeds the size limit")
	}
	return data, err
}

func run() error {
	save := flag.String("save", "", "exported Clicker Heroes save (read only)")
	reserve := flag.String("reserve", "1%", "protected Hero Souls, as a percentage or absolute decimal")
	historyPath := flag.String("history", "", "existing transfer receipt JSON (read only); omission blocks eligibility")
	flag.Parse()
	if *save == "" || flag.NArg() != 0 {
		return errors.New("usage: gilds-plan -save FILE [-reserve 1%] [-history FILE]")
	}
	data, err := readBounded(*save, ancientcalc.MaxSaveInput)
	if err != nil {
		return err
	}
	var history *ancientcalc.GildHistory
	if *historyPath != "" {
		b, err := readBounded(*historyPath, 4096)
		if err != nil {
			return err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(b, &fields); err != nil {
			return err
		}
		for _, key := range []string{"transcensions", "transcensionTimestamp", "lastTargetID", "pendingTargetID"} {
			if string(fields[key]) == "" || string(fields[key]) == "null" {
				return fmt.Errorf("transfer history is missing %s", key)
			}
		}
		if len(fields) != 4 {
			return errors.New("unknown transfer history fields")
		}
		history = &ancientcalc.GildHistory{}
		if err := json.Unmarshal(b, history); err != nil {
			return err
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	plan, err := ancientcalc.CalculateGilds(ctx, data, *reserve, history)
	if err != nil {
		return err
	}
	out := json.NewEncoder(os.Stdout)
	out.SetIndent("", "  ")
	return out.Encode(plan)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
