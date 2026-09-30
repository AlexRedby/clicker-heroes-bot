# Clicker Heroes hotkeys

Reference for Clicker Heroes 1 on desktop, checked on 2026-09-30 against official client build `1.0e12-6144` tooltips and developer announcements. This is a shortcut reference for future bot features, not a list of actions currently automated.

`Hold + click` means keep the key pressed through the click, then release it. The target tab/button matters: the same key can buy levels, move gilds or manage Auto Clickers. Focus the game before sending input.

## Hero levels

| Input | Target | Effect |
| --- | --- | --- |
| Hold `Shift` + click | Hero level button | Buy up to 10 levels. |
| Hold `Z` + click | Hero level button | Buy up to 25 levels. |
| Hold `Ctrl` + click | Hero level button | Buy up to 100 levels. |
| Hold `Q` + click | Hero level button | Buy the affordable MAX quantity, capped at 10,000 levels per click. |
| Tap `T` | Heroes tab | Cycle the persistent purchase quantity, including `x1`. |

A held modifier temporarily overrides the persistent quantity and leaves it unchanged after release. The bot keeps `x1` selected so prices remain numeric, uses `T` only to reach `x1` when needed, and holds `Q` only around a hero purchase. It does not capture a frame to verify MAX or restore MAX afterward. Purchase success is still confirmed from the increased hero level.

Sources: [official client tooltips](https://cdn.clickerheroes.com/gamebuild/builds/6144_new2/Build/6144.data.unityweb), [developer patch notes, including 1.0e11 modifier behavior and MAX cap](https://store.steampowered.com/oldnews/?appgroupname=Clicker+Heroes&appids=363970&enddate=1577865600&feed=steam_community_announcements).

## Ancients

| Input | Target | Effect |
| --- | --- | --- |
| Hold `V` + click | Ancient level button | Enter a custom quantity of levels to purchase. |

Custom quantities support scientific notation with a decimal mantissa, such as `1.1e10`. Ancient quantity modifiers also exist; their exact multipliers and base quantity need a client check before implementing bulk purchases (see the verification list below).

Sources: [official client tooltips](https://cdn.clickerheroes.com/gamebuild/builds/6144_new2/Build/6144.data.unityweb), [1.0e11 developer patch notes](https://store.steampowered.com/oldnews/?appgroupname=Clicker+Heroes&appids=363970&enddate=1577865600&feed=steam_community_announcements).

## Gilded Heroes

| Input | Target | Effect |
| --- | --- | --- |
| Hold `Ctrl` + click | Hero in the Gilded Heroes panel | De-gild with automatic confirmation. |
| Hold `Shift` + click | Hero in the Gilded Heroes panel | Move one randomly selected gild to the clicked hero. |
| Hold `Q` + click | Hero in the Gilded Heroes panel | Move all gilds on other heroes to the clicked hero. |

Gild transfers spend Hero Souls. These modifier clicks need the Gilded Heroes panel, rather than a normal hero level button.

Source: [official client tooltips](https://cdn.clickerheroes.com/gamebuild/builds/6144_new2/Build/6144.data.unityweb).

## Auto Clickers

| Input | Target | Effect |
| --- | --- | --- |
| Hold `C` + click | Eligible Auto Clicker target | Assign an available Auto Clicker. |
| Hold `Shift` while placing an Auto Clicker | Monster area | Assign all remaining Auto Clickers to the monster. |
| Hold `C` + click | Auto Clicker pool/removal button | Remove all assigned Auto Clickers. |

The placement and removal targets are different. Older community shortcut lists give `Q` for removing all; current client text and the official blog give `C`. Verify the intended button in the installed game when adding this feature.

Auto Clickers can level heroes, activate skills and operate the Buy Available Upgrades button. They do not collect the orange fish, even when it appears over them.

Sources: [1.0e9 developer patch notes](https://store.steampowered.com/oldnews/?appgroupname=Clicker+Heroes&appids=363970&enddate=1577865600&feed=steam_community_announcements), [official Auto Clicker guide](https://blog.clickerheroes.com/what-are-auto-clickers-in-clicker-heroes/), [official client tooltips](https://cdn.clickerheroes.com/gamebuild/builds/6144_new2/Build/6144.data.unityweb).

## Mercenaries

| Input | Target | Effect |
| --- | --- | --- |
| Hold `Ctrl` or `Shift` | Mercenary panel | Reveal the Dismiss button; dismissal still requires clicking it. |

Source: [official client tooltip](https://cdn.clickerheroes.com/gamebuild/builds/6144_new2/Build/6144.data.unityweb).

## Mappings to verify before implementing

The following mappings from the [community shortcut list](https://clickerheroes.fandom.com/wiki/Hotkeys) were not confirmed by the official tooltip/announcement checks. Keep them as a checklist for the installed game, not as verified automation contracts.

| Input | Reported mapping to check |
| --- | --- |
| `1` | Clickstorm |
| `2` | Powersurge |
| `3` | Lucky Strikes |
| `4` | Metal Detector |
| `5` | Golden Clicks |
| `6` | Dark Ritual |
| `7` | Super Clicks |
| `8` | Energize |
| `9` | Reload |
| `A` | Toggle Farm Mode / Progression Mode; reported to unlock after beating zone 100. |
| `Shift`, `Z`, `Ctrl`, `Q` + Ancient level click | Reported multipliers: 10, 1,000, 100, 10,000 times the base quantity respectively. Check how the base quantity is chosen. |
| Hold `Shift`, `Z` or `Ctrl` | Reveal Hero/Ancient collapse and expand controls. |

For skills, also check that the skill is unlocked and ready. Use the top-row number keys for initial testing; keypad behavior has not been checked. No direct shortcut for fish collection, scrolling the Heroes list, claiming mercenary quests, Ascension or Transcension was established by this research.

## Bot controls

| Input | Context | Effect |
| --- | --- | --- |
| `F8` (or `Fn+F8` on some Macs) | Bot `run` mode | Start, pause or resume the bot. This is a bot hotkey, not a game shortcut. |
| `Ctrl+C` | Bot terminal | Stop the run. |
