# TODO

## Screen and click reliability

- Resolve macOS screen capture permissions and verify `shot` and fish clicks in a running game.
- Verify F8 pause/resume and fish and optional monster click coordinates on Windows and macOS with display scaling.
- Identify the game window and the hero, mercenary, and prestige panels from real screenshots before adding purchase actions.

## Hero progression

- Capture a live `x1` screenshot with a locked next hero and verify its price crop and OCR result.
- Verify bottom-first scrolling, MAX restoration, purchase confirmation, and F8 pause/resume in a running game.
- Diagnose any remaining unconfirmed hero purchase from its saved before/after screenshots.

## Mercenaries

- Detect completed quests, claim their rewards, and send available mercenaries on new quests.
- Choose quests by reward and duration; leave ruby spending and revivals for a separate decision.

## Prestige and later upgrades

- Read expected Hero Souls and recommend Ascension when progress stalls; verify the full restart loop before enabling automatic Ascension.
- Add Ancient spending after Ascension, then consider gilds, relics, and active skills.
- Read expected Ancient Souls and recommend Transcension; automate the reset and Outsider spending only after the earlier loops are reliable.
