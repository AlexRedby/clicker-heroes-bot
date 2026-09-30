# Hero progression

The bot buys levels for the latest available hero and saves for the next one when its price is within ten times current gold. It does not return to earlier heroes, estimate DPS efficiency, or buy upgrades.

## Reading the game

1. Require the full-screen Heroes tab, recognizable quantity bar, and scrollbar on the primary display. A changed or unknown layout skips the attempt. Lists with no recognizable thumb currently require further support.
2. Select `x1` and verify its orange selected state. Each button then shows a numeric price instead of `MAX`.
3. Drag the scrollbar thumb only when it is above the bottom, and verify its final position. A blue button identifies the deepest available candidate. A following dark row with a level means a later owned hero cannot be leveled, so the bot waits. A following unowned row must have a clear empty level area. A missing or obscured successor of an owned candidate skips the attempt.
4. Run Tesseract on cropped gold, price and level text regions. Gold and levels use white text masks. Prices use a yellow text mask with brightness adjusted for disabled buttons, a crop that excludes the coin and button border, and smooth enlargement. Level recognition isolates the text line and requires a readable `Lvl` label and integer value. Hero names are compared as static image regions to keep the selected row consistent across captures.
5. Enable **Always use scientific notation** in the game. Accept ordinary values with up to three decimal places and normalized scientific mantissas between one and ten. Compare base-10 logarithms so large exponents do not overflow a floating-point amount. Zero gold is represented as negative infinity.
6. Restore and verify `MAX`, then require the same hero, screen geometry, stable list position and enabled button. For an owned hero, wait if the next price is at most ten times gold; otherwise buy current MAX levels. A newly available unowned hero is bought first.
7. Read the level before and after a purchase, moving the pointer away from hero buttons before the confirmation capture to dismiss tooltips. Only a larger readable integer on the same row with a known stable list confirms success. Overlay or animation pixel differences alone cannot confirm a purchase.

OCR reads inherit run cancellation and have a one-second timeout. Captures, recognition and input remain sequential. Fish collection is checked between hero captures and while waiting for confirmation. Pause/resume invalidates pending input decisions; if it interrupts quantity restoration, the next attempt verifies the selected quantity again.

Unreadable text or ambiguous rows skip purchases. Live checks must still cover locked next-hero prices in `x1`, window scaling, scrolling, pause/resume, and the transition to a newly available hero.
