# Visual refresh — 14 October 2026

This iteration follows the request to replace the restrained, form-heavy design
with a colorful contemporary interface, clocks, animation and mini-video. It
supersedes the visual direction in UX_REDESIGN.md; the existing data and API
contracts remain in use.

## What changed

- A shared light/dark visual system for all routes: indigo, lavender, mint,
  peach and rose, rounded surfaces, soft depth, section icons, a new sidebar,
  compact breadcrumbs and mobile navigation. Buttons have hover/press feedback;
  major surfaces have a short entrance transition.
- Focus has a live SVG clock, three mode buttons, start/pause/resume controls,
  server-derived countdown and progress, session statistics and optional
  relationship/search/sound settings. Pomodoro 25/5 is the initial selection.
  Existing timer limits, synchronization, confirmations and time accounting
  remain authoritative. History and the timing explanation are disclosures.
- Calendar has one compact toolbar with view selection and a filter dropdown;
  the calendar grid is the main content. Month cells, events, today and source
  colors are restyled. Drag/resize, recurrence and accessible event list remain.
- Today has a generated landscape and three counts from actual account data.
  Tasks, habits, focus, progress and calendar widgets have distinct surfaces.
  Saved widget order/visibility continue to apply.
- Goals have illustrated color bands and actual progress rings. Habit cards have
  period rings. Templates, notifications, settings, forms, empty states, public
  landing and authentication pages use the shared visual system.
- The landing combines the generated landscape with a static example of the
  focus clock. Its 25-minute example is an entry point, not an active session.

## Assets and motion

The built-in **OpenAI ImageGen** tool generated the landscape; no CLI fallback
or personal API key was used. The output was copied into the repository as a
WebP with the composition preserved:

- `apps/web/public/art/ascend-landscape.webp` — 158,654 bytes.
- `apps/web/public/art/orbit-loop.webm` — 37,861 bytes; 8 seconds, 640×400,
  24 fps, VP9, no audio, seamless orbit and clouds.
- `apps/web/public/art/orbit-poster.svg` — still fallback for the animation.

The video is an original vector animation rendered locally, not an AI-generated
video. It includes visible play/pause controls. System reduced-motion and the
account's `motion: reduce` setting prevent automatic playback. CSS animations
also respect these preferences. Explicit play remains available. Browser
autoplay refusal leaves the poster and play button usable. No external media
request or new runtime animation dependency is needed.

Regenerate the original vector clip with
`node scripts/render-focus-loop.mjs <local-ffmpeg-path>`; the source uses the
existing Sharp installation and an ffmpeg executable with MJPEG input and VP9
encoding. The committed assets do not require either tool at runtime/build time.

Exact final ImageGen prompt:

> Use case: stylized-concept. Asset type: hero landscape artwork for a modern personal productivity web app called Ascend. Create a beautiful cinematic 3D editorial landscape: a small coral orange sun over layered lavender and deep indigo mountains, a winding pale mint path gently climbing toward the light, soft sculptural rounded shapes, tiny translucent glass spheres and clouds. Calm optimistic atmosphere, sophisticated playful art direction, rich periwinkle sky, soft film grain, ambient shadows and beautiful lighting. Wide panoramic composition 1536x1024, enough clear quiet sky at upper left for separate interface copy. No text, no logos, no UI, no watermark. This is an actual background image asset, not a screenshot or mockup.

## Verification

- Production build, TypeScript, ESLint and formatting checks passed.
- Unit tests: 40/40, across 14 files.
- Public browser suite: 13/13, including light/dark layouts at 360, 768,
  1280 and 1440 pixels, keyboard navigation, menu focus and reduced motion.
- The first full authenticated browser run passed 35/37 scenarios. The mobile
  calendar label was corrected to remain accessible. The responsive test now
  waits for Chromium's resize layout to settle. All five affected calendar,
  focus and visual scenarios passed in subsequent targeted runs; the complete
  37-test suite was not repeated afterward.
- Calendar checks cover recurring edits across five views, drag, resize,
  persistence and failed-move rollback. Rollback checks assert the original
  time and allow one pixel of browser layout rounding.
- Focus checks cover server time across two tabs, pause/reload, confirmation,
  Pomodoro work/break boundaries and disabled sound. The new visual scenario
  verifies all three clock modes, duration edits, silent video playback/pause
  and system reduced motion.
- Four main routes (Today, Focus, Calendar and Settings) pass mobile WCAG A/AA
  automated scans in both themes. Seventeen additional account sections were
  checked at 360 pixels for overflow and application errors.
- Desktop and mobile screenshots were inspected; local proof files are in
  `.data/visual-proof/`. No generated test accounts or screenshots are committed.

Browser fixtures use isolated test accounts; screenshots do not represent the
user's personal records. Chromium mobile viewports are not physical iOS/Android
tests. Automated accessibility checks do not replace assistive-technology review.
