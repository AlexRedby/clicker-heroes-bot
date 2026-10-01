# Offline ancient planner

`plan.cjs` reads one JSON object from stdin and writes the plan JSON to stdout. Runtime has no network or game/save mutation. Install the pinned dependencies with `npm ci --ignore-scripts`.

Upstream source is `tomcur/ClickerHeroesCalculator` at commit `1c35f5598f210c689a618a7e46a14769f7bf9a39` (MIT). `upstream/calc.js`, `upstream/data/model.js`, `upstream/data/functions.js`, and the v14307 data are copied computational sources. Data retains only the Ancient, Outsider, item-bonus and achievement definitions used by that model. The bridge only converts the ES module exports to CommonJS, replaces four lodash transforms in the model with equivalent native Object APIs, and removes the unused browser `utils` import. Decoding and import mapping are standalone Node code; native zlib replaces browser/pako inflate.

The input `beyond8k` is the explicit Wepwawet setting supplied by the caller. `reserve` accepts an absolute Decimal or a percentage such as `1%`; output `reserve` is always absolute. `ascensions` is read from `numWorldResets`; pending hero/primal soul rewards are intentionally not included in any soul value. Calculations use Decimal precision 100. Quantities longer than 30 characters are compacted to 15 significant digits, rounded down, for the game input field; internal spend remains exact at that precision.

`testdata/ancient-save.txt` is a sanitized Unity export containing only calculator inputs, without account or login fields.
