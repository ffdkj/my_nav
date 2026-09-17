# Svelte 5 Nav Dashboard — DnD / Fuzzy Search / PWA Research

Researched **2026-09-17**. Versions read live from `registry.npmjs.org` / GitHub / vendor docs, not recalled. Source URLs in §6.

## 0. Verified current versions

| Package | Latest | Published |
|---|---|---|
| `svelte` | **5.57.0** | 2026-08-28 |
| `svelte-dnd-action` | **0.9.79** | 2026-08-21 |
| `sortablejs` / `@types/sortablejs` | **1.15.7** / **1.15.9** | 2026-02-11 / — |
| `fuse.js` | **7.5.0** | 2026-07-13 |
| `vite-plugin-pwa` | **1.3.0** | 2026-05-05 |
| `@atlaskit/pragmatic-drag-and-drop` | **3.1.0** | 2026-08-29 |
| `@atlaskit/pragmatic-drag-and-drop-hitbox` | **2.2.2** | 2026-09-16 |
| `@atlaskit/pragmatic-drag-and-drop-live-region` | **2.1.0** | 2026-08-29 |
| `@neodrag/svelte` | 2.3.3 stable (2025-06-18); 3.0.0-next.12 (2026-08-10) | — |

`fuse.js@7.6.0-beta.0` exists (prerelease, skip). **`@sortablejs/svelte-sortablejs` is 404 on npm — no official SortableJS Svelte wrapper exists.** vite-plugin-pwa 1.3.0 peers: `vite ^3–^8`, `workbox-build`/`workbox-window ^7.4.1`.

---

## 1. Drag & drop: "sortable grid + drop-on-tile merges into folder"

### 1a. SortableJS 1.15.7

```svelte
<script lang="ts">
  import Sortable from 'sortablejs';
  let boardEl: HTMLElement;
  $effect(() => {                         // runs after DOM commit; node exists
    const outer = Sortable.create(boardEl, {
      group: { name: 'tiles', pull: true, put: true },
      animation: 150, fallbackTolerance: 4,             // keep tiles clickable
      delay: 180, delayOnTouchOnly: true, touchStartThreshold: 4,   // long-press
      onAdd(evt)  { /* evt.to = list it landed in, evt.pullMode */ },
      onEnd(evt)  { /* evt.to / evt.from / evt.oldIndex / evt.newIndex */ },
      onMove(evt) { if ((evt.related as HTMLElement).dataset.noDrop) return false; }
    });
    return () => outer.destroy();         // $effect teardown
  });
</script>
```

`onEnd` fires on every drop with `item, to, from, oldIndex, newIndex, oldDraggableIndex, newDraggableIndex, clone, pullMode`; `onAdd` only when an element arrives from *another* list (same payload); `onUpdate` = reorder within one list; `onRemove` = left for another; `onSort` = any change. **Clone vs move:** `pull:'clone'` copies and leaves the original → `evt.pullMode === 'clone'`; `pull:true` moves → `evt.pullMode === true`; `revertClone:true` animates a clone back. Use `pull:true` + `put:true` — a clone would duplicate the tile.

**⛔ There is NO native "drop onto an item".** SortableJS only drops *into a list, at an index*. This exact iOS-style "combining" feature is open issue **#1854 "Combining items into one?"** (opened 2020-06-21, **still open**); the maintainer: *"This looks to me like a great plugin idea. We'll keep this in mind when designing Sortable2"* — no plugin exists, none planned for v1. The Swap plugin (`Sortable.mount(new Swap()); {swap:true, swapClass, onEnd → evt.swapItem}`) is positional swap, not merge. `onMove` can only veto (`false`) or force insertion before/after (`-1`/`1`) — it cannot change what a drop *means*.

**The actual idiom:** give **each tile its own 1-item Sortable list**, all in the same group. "Dropping onto a tile" becomes "dropping into that tile's list", so that tile's `onEnd` fires with `evt.to` = the target tile, and you merge in your own state. Nested sortables are supported (`dragoverBubble:false` default since 1.8.0); an empty nested list needs `emptyInsertThreshold` (default `5px`) or CSS padding to be droppable. **Grid:** shape-agnostic — CSS grid works. **a11y: none** — open request **#2391 "[feature] keyboard navigation support"**; **#1176** closed. You build keyboard reordering yourself.

### 1b. svelte-dnd-action 0.9.79

**Svelte 5: supported and documented.** Peer `svelte: ">=3.23.0 || ^5.0.0-next.0"` (satisfied by 5.57.0). README: *"Svelte 5 prefers `onconsider` and `onfinalize` (over `on:consider` and `on:finalize`) but works both ways as long as it's consistent within a file."* Shipped `.d.ts` types both.

```svelte
<script lang="ts">
  import { dndzone, TRIGGERS, SOURCES } from 'svelte-dnd-action';
  import type { DndEvent } from 'svelte-dnd-action';
  let tiles = $state<Tile[]>([]);
  let hoverId = $state<string | null>(null);
  const FLIP = 200;
  function consider(e: CustomEvent<DndEvent<Tile>>) {
    tiles = e.detail.items;                                   // make room
    hoverId = e.detail.info.trigger === TRIGGERS.DRAGGED_ENTERED_ANOTHER
      ? e.detail.info.id : null;                              // highlight merge target
  }
  function finalize(e: CustomEvent<DndEvent<Tile>>) {
    const { items, info } = e.detail;
    tiles = items;
    if (info.trigger === TRIGGERS.DROPPED_INTO_ANOTHER) { /* merge */ }
  }
</script>

<section aria-label="App tiles"
  use:dndzone={{ items: tiles, type: 'tile', flipDurationMs: FLIP,
                 useCursorForDetection: true, delayTouchStart: 120 }}
  onconsider={consider} onfinalize={finalize}>
  {#each tiles as t (t.id)}
    <div animate:flip={{ duration: FLIP }} class:merge-ready={hoverId === t.id} aria-label={t.name}>
      <!-- folder tiles render their own nested dndzone of type 'folder-item' -->
    </div>
  {/each}
</section>
```

Payload is `{ items, info }` with `info: { trigger, id, source }` (exact shape from the shipped `.d.ts`). `TRIGGERS`: `DRAG_STARTED, DRAGGED_ENTERED, DRAGGED_ENTERED_ANOTHER, DRAGGED_OVER_INDEX, DRAGGED_LEFT, DRAGGED_LEFT_ALL, DROPPED_INTO_ZONE, DROPPED_INTO_ANOTHER, DROPPED_OUTSIDE_OF_ANY, DRAG_STOPPED` (`DRAGGED_*` pointer-only, `DRAG_STOPPED` keyboard-only). `SOURCES`: `POINTER` (**mouse or touch**) | `KEYBOARD`. `consider` fires as the dragged item needs room (and on keyboard drag start); `finalize` on drop. **You must write `items` back on every event** — the lib dispatches, it doesn't own state.

**Nested / cross-zone (folder in & out):** zones sharing a `type` exchange items; use a **distinct type per level** (`type:'tile'` outer, `type:'folder-item'` in folders) to avoid parent/child conflicts. Nested zones are a first-class documented feature (Trello pattern). For nesting add `data-is-dnd-shadow-item-hint={item[SHADOW_ITEM_MARKER_PROPERTY_NAME]}` and **include that marker in the `#each` key** (`0.9.42+`).

**Long-press / merge hit-test — the flags that matter:** `useCursorForDetection: true` uses **cursor position instead of the dragged element's centre**, documented purpose *"improves accuracy when dragging large elements over small drop targets"* (exactly tile-over-tile). `centreDraggedOnCursor: true` repositions the element's centre onto the cursor (changes the visual). `delayTouchStart: true | number` (`true` = 80 ms) prevents touch drags meant to be scrolls. **Grid caveat:** a shipped flag exists for a grid-specific browser bug — `FEATURE_FLAG_NAMES.USE_COMPUTED_STYLE_INSTEAD_OF_BOUNDING_RECT` via `setFeatureFlag(...)`, whose own doc comment cites issue **#454** (*"rect values take time to update when in grid layout"*) and warns it can misbehave for non-grid zones. That's the lever if grid hit-testing jitters.

**Svelte 5 bugs — use ≥ 0.9.77, ideally 0.9.79.** **#637**: 0.9.58 broke zones storing items as `$state()` (*"Svelte 5 users please use version 0.9.57 until this issue is fixed"*), fixed in 0.9.59. **#644** "Svelte 5 reactive classes behave inconsistently" — *"while dragging the object is not correctly copied, only when dropped"*; breaks if `id` is `$state()` → **keep items as plain objects in a `$state` array; avoid class instances with `$state` fields.** **#677** ("Rendering issue in Svelte 5.43.8 and beyond") closed 2026-08-01 as not reproducible; maintainer: the report *"predates substantial changes now present in v0.9.77"*.

### 1c. Svelte 5-native alternatives

**`@neodrag/svelte`** — pointer-based *free dragging only* (transform/position): no sorting, no drop targets, no collision detection; v3 still `-next`. Usable only as a drag layer atop hand-written reorder. **Not a fit alone.**

**`@atlaskit/pragmatic-drag-and-drop@3.1.0`** — vanilla TS; docs say usable with *"any view library (eg `react`, `svelte`, `vue` etc)"*. **No official Svelte adapter or examples** (none found in repo); wire refs + `$effect` cleanup by hand.
- **Yes — "drop on element with custom hit-testing" is its core strength.** `dropTargetForElements({ element, getData, canDrop, getDropEffect, getIsSticky, onDrop })`. Targets are **nested, resolved deepest-first in bubble order**; `canDrop()` returning `false` on an inner target makes lookup continue upward; `getIsSticky()` holds selection across gaps; `onDrop` fires on *every* target in the chain, so tell child-vs-self apart with `location.current.dropTargets[0]?.element === self.element`. `…-hitbox@2.2.2` adds `attachClosestEdge`/`extractClosestEdge`/`closestCenter`.
- **a11y: deliberately NOT included.** Docs: *"The core package does not enable accessible controls automatically, as there is no one pattern that works well for all situations."* They prescribe a per-item **More (…) button/menu**, live-region announcements (`@atlaskit/pragmatic-drag-and-drop-live-region` → `announce('…')`), focus restoration — and **explicitly advise against arrow-key controls**.
- **⚠️ Touch is a real risk.** It is *"powered by the web platform's built in drag and drop"*, inheriting native constraints. Discussion **#93 "Mobile/Touch Support?"** — a dev who tried it **in Svelte**: *"no luck. It wasn't usable. Same with the examples in the documentation. It would start 'dragging' the element but only 10% it would allow me to drop it somewhere. Also the press + hold time frame is too long."* No maintainer reply; issue **#124** also open. Disqualifying for a phone-first dashboard without a touch polyfill.

### 1d. Recommendation: **svelte-dnd-action@0.9.79**

1. **Only one of the three with first-class Svelte 5 support** (declared peer range, typed `onconsider`/`onfinalize`, Svelte 5 syntax in the README).
2. **Touch works without a polyfill** — pointer/touch-event based, not native DnD. pragmatic-drag-and-drop is reported unusable on touch (#93); SortableJS **hard-disables native DnD on iOS/Android-Chrome in its own source**, so you'd get its fallback *plus* a hand-built merge layer.
3. **Built-in keyboard DnD + ARIA**, which SortableJS lacks entirely (#2391) and pragmatic-drag-and-drop deliberately omits.
4. **`useCursorForDetection`/`centreDraggedOnCursor` exist specifically for large-item-over-small-target** — the merge hit-test via a flag, not hand-rolled `getBoundingClientRect()` math.
5. `type`-scoped cross-zone drops + `DROPPED_INTO_ANOTHER` give folder-in *and* folder-out.

**Trickiest part — overlap detection + merge animation.** Model `Tile = { id, kind: 'link'|'folder', items?: Link[] }`; outer grid = one `dndzone` `type:'tile'`, each folder tile = a **nested `dndzone` of `type:'folder-item'`**, so links drag into folders and back out. Put the folder's `onfinalize` **inside the folder component, closing over the folder id** — the event carries `items`/`info.id`, not the destination zone id, so closure is how you know which folder received it. Set `useCursorForDetection: true` on the outer zone so a ~96 px tile dragged by a thumb still resolves the right target; in `consider`, `DRAGGED_ENTERED_ANOTHER` ⇒ hovering a different zone → apply `merge-ready`, and `DRAGGED_OVER_INDEX`/`DRAGGED_LEFT` clear it. ⚠️ **"Entered another zone" is zone-granular, not a dwell timer** — the library has no dwell/hover-threshold option, so a passing hover can trigger a merge. For the iOS "hold over an icon" feel, gate the merge on **your own timer** started in `consider` on `DRAGGED_ENTERED_ANOTHER` and cancelled on `DRAGGED_LEFT`; that's a design decision you must make, not a library feature. **Animating the merge:** `animate:flip` on **both** `#each` blocks (outer tiles *and* folder contents) with the same `flipDurationMs` passed to each `dndzone` — the README pairs them deliberately. On drop, mutate **both in the same tick**: remove the source from `tiles` *and* push its link into the target folder's `items`; FLIP animates both lists. Keep `dropAnimationDisabled` default (`false`) so the library's own drop animation plays; use `flipDurationMs: 0` + `dropAnimationDisabled: true` only for full manual control. An extra `transition:scale`/`fly` on icons inside the folder gives the "icons fly in" feel.

---

## 2. Fuse.js 7.5.0

Defaults from `src/core/config.ts`: `threshold 0.6`, `distance 100`, `location 0`, `ignoreLocation false`, `ignoreFieldNorm false`, `fieldNormWeight 1`, `minMatchCharLength 1`, `findAllMatches false`, `includeScore false`, `isCaseSensitive false`, `ignoreDiacritics false`, `shouldSort true`, `useExtendedSearch false`, `useTokenSearch false`.

```ts
import Fuse from 'fuse.js';
const keys = [
  { name: 'title', weight: 3 },
  { name: 'url',   weight: 1 },
  { name: 'tags',  weight: 2 }        // dot notation for nested: 'meta.category'
];
const options = { keys, threshold: 0.35, ignoreLocation: true,
                  minMatchCharLength: 2, includeScore: true, ignoreDiacritics: true };
const fuse = new Fuse(links, options);
fuse.search(query, { limit: 20 });
```

**Set `ignoreLocation: true`.** With the default `false`, match position is scored against `location: 0` (string start) with `distance: 100`, so a hit deep inside a long URL is heavily penalised — correct for link titles/URLs. `threshold 0.3–0.4` is the documented range for search-as-you-type (`0.6` returns far too much).

**Fast + reactive in Svelte 5.** A few hundred entries is trivial — documented ~3 ms to index 1,000 items × 3 keys; docs cite *"search 200 items in the browser in <1ms"*. **Never rebuild the instance per keystroke:** `const fuse = $derived.by(() => new Fuse(links, options));` then `const results = $derived(query ? fuse.search(query, { limit: 20 }) : []);`. If `links` mutates often, prefer **one instance + `fuse.setCollection(links)`** in an `$effect` (Fuse also exposes `add`, `remove(predicate)`, `removeAt(idx)`, `getIndex`). For large/rarely-changing data, prebuild with `Fuse.createIndex(keys, list)` then `new Fuse(list, opts, index)`. Also disable unused work: don't set `includeMatches`/`findAllMatches` unless highlighting, and drop `includeScore` in production if unused.

**v6 → v7 (2023-10-24).** Changelog lists one breaking item: *"Extension changed"* — i.e. **proper ESM exports / entry-point extensions, not a search-API change**; follow-up issue **#767 "7.0.0 Breaking Change Documentation Unclear"** confirms it was under-documented. Import stays `import Fuse from 'fuse.js'`; subpaths are `fuse.js/basic|min|min-basic` and (since 7.4.1, typed) `fuse.js/worker|worker-script`. **`keys`/`threshold`/`ignoreLocation` semantics did not change in v7.**

**⚠️ 7.5.0 (2026-07-13) changes scores/ordering — re-baseline.** A minor release that nonetheless: counts tabs/newlines as word separators ([#830](https://github.com/krisk/Fuse/issues/830)); normalises key weights consistently via `KeyStore` ([#833](https://github.com/krisk/Fuse/issues/833)); returns correct top-N under score ties (#835); makes Bitap respect `minMatchCharLength` in the exact-match shortcut (#831). **Don't assert exact `score` values or a specific result order in tests** — assert set membership/ordering intent. #833's real bug: a raw `weight: 100` went into an exponent and **underflowed to `score 0`** before the fix.

---

## 3. vite-plugin-pwa 1.3.0

```ts
import { VitePWA } from 'vite-plugin-pwa';
export default defineConfig({
  plugins: [svelte(), VitePWA({
    registerType: 'prompt',        // user-confirmed update; injectRegister:'auto' is default
    manifest: {
      name: 'My Nav', short_name: 'MyNav',
      start_url: '/',              // default = Vite `base` ("routerBase")
      scope: '/',
      display: 'standalone',       // already the DEFAULT
      background_color: '#ffffff', theme_color: '#0b1220',
      icons: [
        { src: 'pwa-192x192.png', sizes: '192x192', type: 'image/png' },
        { src: 'pwa-512x512.png', sizes: '512x512', type: 'image/png' },
        { src: 'pwa-512x512.png', sizes: '512x512', type: 'image/png', purpose: 'any maskable' }
      ]
    },
    workbox: {
      globPatterns: ['**/*.{js,css,html,ico,png,svg,webmanifest}'],
      navigateFallback: 'index.html',                 // SPA offline shell
      runtimeCaching: [{
        urlPattern: ({ url }) => url.pathname.startsWith('/api/'),
        handler: 'NetworkFirst',
        options: { cacheName: 'api-json', networkTimeoutSeconds: 3,
                   expiration: { maxEntries: 50, maxAgeSeconds: 604800 },
                   cacheableResponse: { statuses: [0, 200] } }
      }]
    },
    devOptions: { enabled: true }  // SW + manifest in `vite dev`
  })]
});
```

**Manifest defaults confirmed in the shipped `dist/index.d.ts` (`ManifestOptions`):** `display` defaults to **`'standalone'`**; `start_url` to `routerBase`; `name`/`short_name` to `_npm_package_name_`; `description` to `_npm_package_description_`; `background_color` `'#ffffff'`; `theme_color` `'#42b883'`; `icons` `[]`. So you must supply the 192/512 PNGs plus a `purpose:'any maskable'` entry; the rest is optional. (A v0.21.2 fix addressed `theme_color`/`description` defaults not applying — don't pin below that.)

**Prompting the user** — the `virtual:pwa-register` module is **mandatory** for `registerType:'prompt'`:

```ts
import { registerSW } from 'virtual:pwa-register';
const updateSW = registerSW({
  onNeedRefresh() { showUpdateToast = true; },  // new SW is `waiting`
  onOfflineReady() { /* first install cached */ },
  onNeedReload() { /* NEW in 1.3.0: after updateSW() takes control; default is location.reload() */ },
  onRegisterError(e) { console.error(e); }
});
// user taps Reload:  updateSW(true)
```

On the non-auto path the client listens for Workbox's `waiting` event and calls `onNeedRefresh`; `updateSW(true)` sends `skipWaiting` + reload. `onNeedReload` is new in 1.3.0 and lets you defer the reload. Needs `/// <reference types="vite-plugin-pwa/client" />`.

**NetworkFirst vs StaleWhileRevalidate for `/api`.** vite-plugin-pwa passes `runtimeCaching` straight to Workbox (`workbox-build ^7.4.1`); the semantics are Workbox's and the plugin docs only *show* examples (`NetworkOnly`, `CacheFirst`) — **the following is my recommendation grounded in Workbox semantics, not a doc quote:** use **`NetworkFirst` for the links JSON** (current when online, still renders offline) and **always set `networkTimeoutSeconds: 3`** — without it, offline users wait for a hanging fetch to fail before the cache is used; add `expiration` to bound stale link sets. Use **`StaleWhileRevalidate` only where one-load-stale is fine and instant paint matters** (icon metadata, rarely-changing config) — it serves cache immediately and updates in the background, so the first load after a deploy can show old data. **Never `CacheFirst` for `/api`** unless the payload is immutable. Switch to **`injectManifest`** over `generateSW` if you later need custom SW logic (response trimming, custom offline fallback for navigation).

---

## 4. Mobile / touch behavior

**The common claim "iOS Safari has no HTML5 drag & drop" is out of date.** caniuse `dragndrop` marks **Safari on iOS 15 – 26.6 as supported**; only **3.2 – 14.8 unsupported**. Recorded caveats: *"Safari doesn't implement the `DragEvent` interface. It adds a `dataTransfer` property to `MouseEvent` instead"* (WebKit bug #103423); `dataTransfer.items` is Chrome-only. **Still unsupported: Firefox for Android and Samsung Internet**, plus Chromium on Android ≤6.

**But "supported" ≠ "usable" — the libraries vote with their code.** SortableJS **hard-disables native DnD on iOS and Android Chrome**: from `src/Sortable.js`, `supportDraggable = documentExists && !ChromeForAndroid && !IOS && ('draggable' in document.createElement('div'))`, with `supportPointer = … && ('PointerEvent' in window) && (!Safari || IOS)`. On those platforms it **never uses native HTML5 DnD** — it runs its own pointer/touch implementation, the strongest primary evidence that native DnD is unreliable on touch despite caniuse (`forceFallback: true` makes desktop match mobile). **pragmatic-drag-and-drop is native-DnD-powered by design** (*"powered by the web platforms built in drag and drop functionality"*) so it inherits these constraints exactly — hence the #93 field report. **svelte-dnd-action doesn't use native DnD at all** (README: *"not using the browser's built in dnd, thanks god"*), which is why it has `delayTouchStart` rather than a fallback flag.

**Long-press vs page scroll gotchas.** A long-press on a tile in a scrollable page must distinguish scroll from drag: svelte-dnd-action via `delayTouchStart: true | number` (`true` = 80 ms); SortableJS via `delay` + `delayOnTouchOnly: true` + `touchStartThreshold` (min pointer movement before a delayed drag is cancelled). Respect `touch-action`: `touch-action: none` on a tile kills page scrolling from it, while leaving it unset lets the browser claim the gesture — prefer `touch-action: manipulation` on tiles and let the library manage the active drag. To **keep tiles clickable as links**, SortableJS has `fallbackTolerance: 3–5`; svelte-dnd-action's `delayTouchStart` is time- not distance-based so a slow deliberate tap can still start a drag — consider `dragHandleZone` + `dragHandle` if accidental drags persist (cost: a visible handle). iOS also needs `-webkit-touch-callout: none` / `user-select: none` on tiles to stop the long-press context menu and text selection hijacking the gesture.

---

## 5. Accessibility

**svelte-dnd-action ships keyboard reorder** (a main reason it's the recommendation): Tab to an item → **Space or Enter** enters drag mode → **arrow keys** move → trigger key or **Escape** exits; mouse drag works independently of keyboard drag mode. `setKeyboardDragTrigger('space'|'enter'|'space_or_enter'|null)` — keys outside the trigger are *left completely untouched*, so `'space'` keeps **Enter free to activate the focused link**, which matters for a dashboard of links (throws on invalid values). `setAriaStrings(overrides|null)` rewrites all screen-reader wording (`dragStarted`, `movedToPosition`, `movedToZoneStart/End`, `dropped`, `zoneActiveInstruction`, `zoneDragDisabledInstruction`); each call defines a whole locale and omitted keys revert to English. `alertToScreenReader(txt)` for custom alerts; `autoAriaDisabled: true` + your own logic if needed. **You must add `aria-label` on the zone container and every draggable item** — README: *"The library will take care of the rest."* The labels are what make the announcements meaningful. `zoneTabIndex`/`zoneItemTabIndex` let you collapse tab stops (e.g. one per board) while preserving keyboard DnD.

**Grid of links — ARIA pattern.** A nav grid is a **list of links, not an ARIA grid widget**: don't use `role="grid"`/`gridcell` (implies 2-D arrow-key navigation and cell semantics you won't implement). Instead use `<nav aria-label="App tiles">` with tiles as `<li>` containing real `<a href>` — this keeps Enter, middle-click, "open in new tab" and screen-reader link lists working. Add `aria-label` per tile zone and per tile (required by svelte-dnd-action); make a folder tile a `<button aria-expanded aria-haspopup>` opening an overlay that contains its nested zone (not a decorative blob); and add an `aria-live="polite"` region for "Moved *GitHub* into folder *Dev*" — the library's alerts cover item movement, but **your merge is a state change it knows nothing about**, so announce it yourself (`alertToScreenReader()` is the hook). Honour `prefers-reduced-motion` for FLIP/merge animations.

**Contrast:** SortableJS has **no** keyboard support (#2391) — all of the above would be yours. pragmatic-drag-and-drop deliberately ships none and *advises against* arrow keys, prescribing a More-menu + `announce()` live region instead (coherent, but a much larger build). ⚠️ A secondary source (a Medium article) claiming it "includes keyboard navigation and screen reader support" is **wrong** — the vendor's own accessibility page says the opposite.

---

## 6. Sources

**Context7 library IDs used:** `/isaachagoel/svelte-dnd-action`, `/sortablejs/sortable`, `/krisk/fuse`, `/vite-pwa/vite-plugin-pwa`.

**Primary (vendor/source):** [svelte-dnd-action README](https://github.com/isaachagoel/svelte-dnd-action/blob/master/README.md) · [shipped `dist/index.d.ts` @0.9.79](https://cdn.jsdelivr.net/npm/svelte-dnd-action@0.9.79/dist/index.d.ts) · [SortableJS README](https://github.com/SortableJS/Sortable/blob/master/README.md) · [`src/Sortable.js`](https://github.com/SortableJS/Sortable/blob/master/src/Sortable.js) · [Fuse CHANGELOG](https://github.com/krisk/Fuse/blob/main/CHANGELOG.md) · [`src/core/config.ts`](https://github.com/krisk/Fuse/blob/main/src/core/config.ts) · [Fuse performance](https://github.com/krisk/Fuse/blob/main/docs/performance.md) · [vite-plugin-pwa docs](https://vite-pwa-org.netlify.app/) · [vite-plugin-pwa `dist/index.d.ts` @1.3.0](https://cdn.jsdelivr.net/npm/vite-plugin-pwa@1.3.0/dist/index.d.ts) · [pragmatic-drag-and-drop drop targets](https://atlassian.design/components/pragmatic-drag-and-drop/core-package/drop-targets) · [accessibility guidelines](https://atlassian.design/components/pragmatic-drag-and-drop/accessibility-guidelines) · [platform constraints](https://atlassian.design/components/pragmatic-drag-and-drop/web-platform-design-constraints)

**Compatibility issues:** svelte-dnd-action [#637](https://github.com/isaachagoel/svelte-dnd-action/issues/637) · [#644](https://github.com/isaachagoel/svelte-dnd-action/issues/644) · [#677](https://github.com/isaachagoel/svelte-dnd-action/issues/677) · SortableJS [#1854](https://github.com/SortableJS/Sortable/issues/1854) (combining) · [#2391](https://github.com/SortableJS/Sortable/issues/2391) (keyboard) · Fuse [#767](https://github.com/krisk/Fuse/issues/767) · [#830](https://github.com/krisk/Fuse/issues/830) · [#831](https://github.com/krisk/Fuse/issues/831) · [#833](https://github.com/krisk/Fuse/issues/833) · [#835](https://github.com/krisk/Fuse/issues/835) · pragmatic-drag-and-drop [discussion #93](https://github.com/atlassian/pragmatic-drag-and-drop/discussions/93) (mobile) · [#124](https://github.com/atlassian/pragmatic-drag-and-drop/issues/124) · [caniuse `dragndrop` data](https://github.com/Fyrd/caniuse/blob/main/features-json/dragndrop.json)

### Uncertainty flags
- **caniuse says iOS Safari 15+ supports HTML5 DnD, yet every library here treats touch as a fallback problem.** Both are reported above; the practical read is "technically present, not dependable for a drag-heavy grid". **Not tested on a device — verify on a real iPhone before committing.**
- **No maintainer answer exists** in pragmatic-drag-and-drop discussion #93, so its touch story is unconfirmed by the vendor.
- **"Each tile is its own 1-item Sortable list" is a community pattern** (referenced in #1854's reporter's own attempts and comparable nested-list setups), not documented by SortableJS as a supported merge mechanism.
- **`@neodrag/svelte` v3** is still `-next`; evaluated only for API scope.
- **The Workbox strategy recommendation (§3)** is my reasoning from Workbox semantics, not a quoted vite-plugin-pwa recommendation.
- The fork **`aholland/svelte-dnd-action`** was mentioned in #677 as carrying fixes; **not evaluated**.
