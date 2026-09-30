# Hero progression

The bot buys levels for the latest affordable hero and saves for the next one when its price is within ten times the current gold. It does not return to earlier heroes, estimate DPS efficiency, or buy upgrades.

## Reading the game

1. Require the Heroes tab and the visible `Lvl` quantity bar. The game should fill the captured screen.
2. Select `x1` so each hero button shows a numeric price. Under `MAX`, that field contains `MAX` instead of a price.
3. Drag the scrollbar thumb to the bottom. A blue button identifies an affordable hero. If the following dark row already shows a level, it is the latest owned hero and the bot waits instead of buying an earlier one. A dark row without a level is the next unowned hero.
4. Run Tesseract only on the gold display and the next button's price. Both regions are cropped and enlarged; gold text is isolated by its white color. Full-screen OCR is slower and mixes hero text with unrelated numbers. Hero names and levels are not OCR inputs.
5. Accept only plain decimal or scientific notation. The game setting **Always use scientific notation** is required for large numbers. Compare base-10 logarithms so large exponents do not overflow a floating-point amount.
6. Restore `MAX` and confirm the selected hero button is still enabled before clicking it. For an owned hero, wait if the next price is at most ten times current gold; otherwise buy current MAX levels. When the next hero becomes affordable, it is the latest enabled button and is bought first.

Unreadable numbers or a missing quantity bar stop that purchase attempt. Fish detection continues. A live game check should cover `x1` prices on a locked next hero, scrolling, pause/resume, and a transition to the next affordable hero.
