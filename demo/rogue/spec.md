# Rogue 5.4.4 WebAssembly Port — Specification

## Overview

A faithful port of Rogue 5.4.4 from C to Rust, compiled to WebAssembly, playable in a browser at `demo/rogue/rogue-wasm/www/index.html`. The original C source is at `demo/rogue/original-rogue/` — approximately 16,800 lines across 33 files.

This is an **exact mechanical port**: the Rust code must reproduce the same gameplay behavior as the C original for any given RNG seed. The only permitted behavioral differences are the I/O layer (ncurses replaced by a WASM-to-JS terminal bridge) and persistence (filesystem save/load replaced by localStorage).

## Scope

### In Scope

- Complete Rust reimplementation of all Rogue 5.4.4 game systems
- Compilation to `wasm32-unknown-unknown` via `wasm-pack`
- Single-page HTML deliverable with embedded JS terminal renderer
- Classic 80x24 ASCII terminal display
- All player commands (movement, inventory, combat, meta)
- Full monster AI and daemon/fuse timing system
- Wizard mode (`MASTER` / `wizard.c`): create objects, teleport, level skip, map reveal, identify, and all debug commands — both as gameplay feature and as the primary integration test harness
- Deterministic seed support (equivalent to the C version's `SEED` env var) for reproducible games
- Save/load via browser localStorage
- Keyboard input handling in the browser

### Out of Scope

- Mobile/touch input
- Sound or music
- Graphical tiles or non-ASCII rendering
- Multiplayer or networking
- High-score server or cross-session leaderboards (local only)
- Accessibility features beyond what the original had
- Performance profiling or optimization beyond "runs smoothly at 60fps"

## Constraints

### Fidelity

The port is a **1:1 mechanical translation**. For any given RNG seed, the port must produce identical game states as the C original. This means:

1. **RNG**: The exact linear congruential generator must be reproduced:
   - Formula: `seed = seed * 11109 + 13849`, output = `(seed >> 16) & 0xffff`
   - `rnd(range)` = `abs(RN) % range` (returns 0 when range is 0)
   - `roll(number, sides)` = sum of `number` calls to `rnd(sides) + 1`
   - Seed initialization: `seed = time + pid` (in WASM: `Date.now()` + synthetic value)

2. **Dungeon generation**: Rooms placed on a 3x3 grid (`MAXROOMS=9`), dimensions derived from `bsze.x = NUMCOLS/3`, `bsze.y = NUMLINES/3`. Room removal: `rnd(4)` rooms marked ISGONE. Dark rooms: `rnd(10) < level-1`. Maze rooms: `rnd(15) == 0` when dark. Passage connectivity via spanning tree over the hardcoded `rdes[]` adjacency matrix, with extra cycle-creating passages.

3. **Monster stats**: All 26 monsters (A-Z) with exact stats from `extern.c`: carry%, flags, strength, experience, level, armor, HP formula (`roll(lvl, 8)`), damage strings. Monster level/armor/exp scaling beyond `AMULETLEVEL=26` must match the `lev_add` formula.

4. **Combat math**: To-hit: `rnd(20) + wplus >= (20 - at_lvl) - op_arm`. Damage: `roll(ndice, nsides) + dplus + add_dam[str]`. Strength tables (`str_plus[]`, `add_dam[]`) must match the C arrays exactly. All special attacks (aquator rust, rattlesnake poison, wraith/vampire drain, etc.) must reproduce the same probability checks and effects.

5. **Item tables**: All generation probabilities must match `extern.c`:
   - Item type distribution: potion 26%, scroll 36%, food 16%, weapon 7%, armor 7%, ring 4%, stick 4%
   - Per-type probabilities for all 14 potions, 18 scrolls, 9 weapons, 8 armors, 14 rings, 14 wands/staves
   - Weapon damage tables (wield and throw), armor AC values, ring/wand charge ranges

6. **Daemon/fuse system**: `MAXDAEMONS=20`. All daemon timing (`doctor`, `runners`, `swander`, `stomach`) and fuse scheduling must match the BEFORE/AFTER execution order and the `spread()` formula.

7. **Level progression**: `AMULETLEVEL=26`. Monster selection via `lvl_mons[]`/`wand_mons[]` arrays. Trap generation: `rnd(10) < level`, count = `rnd(level/4) + 1` capped at `MAXTRAPS=10`. Item generation: 9 attempts per level at 36% each. Gold: `rnd(50 + 10*level) + 2`.

8. **Hunger system**: `HUNGERTIME=1300`, `STARVETIME=850`, `STOMACHSIZE=2000`. Food consumption rate, slow digestion ring effect, and starvation death must all match.

9. **Save system**: All game state that the C version serializes must be captured in localStorage. This includes: player stats/position, inventory, current level map, monster positions/states, daemon/fuse queue, room states, discovered item names, and RNG seed. A save from one session must restore identically in another.

### Technology

- Language: Rust (stable toolchain)
- Target: `wasm32-unknown-unknown`
- Build: `cargo build --target wasm32-unknown-unknown` (and `wasm-pack` for JS bindings)
- Tests: `cargo test` (native target for unit tests)
- Formatting: `cargo fmt --check`
- No external game engine or framework dependencies
- JS terminal renderer: vanilla JS, no npm dependencies required in the HTML deliverable

### Deliverable

A single HTML file at `demo/rogue/rogue-wasm/www/index.html` that:
- Loads the WASM module
- Renders an 80x24 character grid
- Uses a monospace font on a dark background
- Displays `@` for the player, `#` for corridors, `.` for floors, `+` for doors, monster letters A-Z, and all other standard Rogue display characters
- Accepts keyboard input for all Rogue commands
- Shows the status line on row 24 (0-indexed row 23)

## Assumptions

1. The user has Rust stable, `cargo`, `rustc`, and `wasm-pack` installed.
2. The deliverable will be served locally (e.g., `python -m http.server`) — no CDN or hosting requirements.
3. The original C source at `demo/rogue/original-rogue/` is the authoritative reference for all gameplay behavior.
4. Browser target: modern evergreen browsers (Chrome, Firefox, Safari, Edge — latest two versions). No IE11 support.
5. Wizard mode in the WASM port does not require password authentication — it can be enabled via a JS API call, URL parameter, or equivalent browser-friendly mechanism (the C version's `crypt()`-based password check is replaced by a simpler gate).

## Verification Approach

### Deterministic Verification (Automated)

1. **Build**: `cargo build --target wasm32-unknown-unknown` succeeds with no errors
2. **Format**: `cargo fmt --check` passes
3. **Unit tests**: `cargo test` passes — unit tests for RNG, combat, dungeon gen, items, monster stats (see DoD for full test list)
4. **Gameplay integration tests**: `cargo test` includes tests that drive the game engine programmatically through multi-turn gameplay sequences with a fixed seed and verify game state (see DoD AC-13 for full scenarios)
5. **Deliverable exists**: `demo/rogue/rogue-wasm/www/index.html` exists and references the WASM module
6. **No artifact pollution**: `git diff` does not include `target/`, `node_modules/`, `dist/`, or other build artifacts

### Semantic Verification (Review)

1. **Fidelity review**: Code review comparing Rust implementations of each game system against the C original, checking that constants, formulas, and control flow match
2. **Playability**: The game loads in a browser, accepts input, generates dungeons, spawns monsters, and plays through at least several levels without crashes
3. **Save/load**: Save game, close tab, reopen, restore — game state is preserved
4. **Wizard mode**: Wizard commands (create object, teleport, level skip, map reveal) all function correctly in the browser

## Non-Goals / Deferrals

- **Wizard password authentication**: The C version's `crypt()`-based wizard password is replaced by a simpler browser-friendly mechanism. The wizard commands themselves are fully in scope.
- **Score file encryption**: The `xcrypt.c` XOR cipher for score files is unnecessary in a browser context.
- **Multi-user score isolation**: The original's UID-based score file isolation doesn't apply in a browser.
- **Signal handling**: SIGINT/SIGTSTP/SIGUSR signal handling from `mach_dep.c` is not applicable to WASM.
- **Cross-compilation to native**: The port targets WASM only. A native Rust binary is not a deliverable (though tests run natively).
- **Pixel-perfect terminal rendering**: The JS renderer should be faithful to 80x24 ASCII, but exact font metrics, cursor blinking, and terminal escape sequence fidelity are not requirements.
- **Original save file format compatibility**: The WASM port's localStorage format does not need to be compatible with the C version's binary save files.
