# Interactive workspaces — 15 October 2026

This pass addresses the nine screenshots of form-heavy inner screens. It builds
on the shared visual system and focus clock in `VISUAL_REFRESH.md`, and changes
how the workspaces are used, as well as their appearance.

## Workflows

- **Planning:** a keyboard-operable week strip respects the account's first day
  of the week. Time presets, a 0–1440 minute slider, editable work windows and a
  day ribbon lead into selectable task cards and up to three main tasks. The
  saved capacity appears as four distinct metrics. Confirmation, immutable
  original commitments, rest days, versions and unsaved-change guards retain
  their existing behavior.
- **Journal:** four optional prompts append to the current text and only fill
  an empty title. A formatting toolbar inserts Markdown at the selection.
  Date and title share a row; optional tags, goal links and filters are
  disclosures. Saved entries have their own cards. The existing server-rendered
  sanitized preview, account isolation and optional tab draft remain in use.
- **Focus history:** confirmed work, pending time, rest and session counts have
  separate tiles. Presets select today, the last seven days or the current
  calendar month. Custom ranges leave the presets unselected. Sessions form a
  dated timeline with status and outcome badges; detailed filters and interval
  breakdowns are expandable. The live clock and silent controllable mini-video
  from the previous iteration remain.
- **Analytics:** period and metric chips drive the real queries and charts.
  Four summary tiles and selectable week cards complement the charts. Custom
  dates and the accessible data tables start collapsed and remain reachable.
  Changing the period preserves the chosen metric and expanded table. A visible
  updating notice identifies previous data while the next range loads. No
  illustrative values are added to empty accounts.
- **Navigator and schedule:** task selection uses tiles; date presets and a
  compact custom range precede the proposed timeline. Deadline Radar has four
  colored, expandable urgency groups with actual demand, capacity and deficit.
  The empty radar has a concise illustrated state. Underlying calculations,
  preview/confirmation, rollback and unknown estimates retain their behavior.
- **Game and reflection screens:** the level crest, XP wallet and shortcuts,
  achievement medals/filter chips, reward wallet, character stage, boss emblem,
  season progress, suggestion catalog, routine checklists and mood/energy chips
  share the same palette and feedback patterns.

## Motion and accessibility

- Cards enter with a short stagger; metrics animate numeric changes; charts
  animate their actual data. Selected controls, completed checklist items and
  hover/press states provide visible feedback.
- Success notices have a finite confetti burst. Decorative floating runs twice;
  it does not loop indefinitely. Both system and account reduced-motion settings
  suppress decorative animations. The existing focus video has play/pause.
- Controls use native buttons, inputs, details and labels, with pressed states,
  grouped metrics and decorative icons hidden from assistive technology. The
  Markdown editor retains a named textarea and uses the existing safe renderer.
- Styles cover light/dark themes and narrow layouts. No new runtime dependency,
  external media request or backend data contract is introduced.
- Server-rendered forms and the theme picker wait for their client handlers
  before accepting input. A browser regression holds JavaScript downloads to
  verify the disabled state, then releases them and checks that input is retained.
  Theme checks wait for the account preference to save and verify it again after
  navigation.

## Verification

- Production build and TypeScript compilation pass. ESLint, Prettier and the
  repository hygiene check pass. All 40 component/unit tests pass.
- All 14 public browser scenarios pass, including delayed-JavaScript form
  readiness, four viewport widths in both themes, keyboard navigation, mobile
  focus trapping, reduced motion and retry behavior.
- The full authenticated browser run passed 38 of 39 scenarios. Its remaining
  analytics scenario exposed a lost chart selection on period changes. Keeping
  the previous response during refresh fixed that state loss; the follow-up
  also exposed a stale hidden tooltip position after resizing. The plot now
  contains that tooltip without clipping the page or the scrollable data tables.
  The complete analytics scenario passes on the final production build, including
  preserved metric/table state, exact source links, resize and Axe checks.
- The affected planning workflows, journal persistence and analytics report /
  personal-target scenarios pass on the revised build. The new route sweep
  covers 12 sections at desktop and 360 px widths, verifies saved light/dark
  themes after navigation, expands relevant filters and runs Axe WCAG checks.
- Visual evidence is saved locally under `.data/interaction-proof/`, including
  populated planning and formatted journal records, focus history and desktop /
  mobile views of all 12 sections.

Browser scenarios use isolated test accounts; screenshots contain test records.
Chromium viewport checks do not substitute for physical-device or
assistive-technology testing.

This is an interface revision of the implemented product; it does not claim to
deliver later, unimplemented product-roadmap stages.
