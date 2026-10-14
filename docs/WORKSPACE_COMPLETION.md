# Daily workspaces — 15 October 2026

This pass completes the interface refresh of tasks, habits, goals, calendar,
settings and notifications. It continues `INTERACTION_REDESIGN.md` and preserves
the implemented product's data model and server-side rules.

## Changes

- Tasks have date, duration and priority shortcuts, quick status filters, colored
  status cards and a detail header with the real planned date and estimate.
  Detailed filters stay synchronized with quick filters; arbitrary values remain
  editable. List and board use the same saved records.
- Habit creation uses three illustrated mode cards. Check-ins offer real amounts
  derived from the minimum step and remaining target. Choosing an amount does
  not create a log until the user submits it. Current/best streaks and completed
  periods have distinct visual feedback.
- Goals offer visual progress-mode choices and a template gallery. The detail
  header shows the actual progress ring and dates. Clicking an already selected
  mode preserves entered values, including metric units and target values.
- Calendar views use labeled chips. Event duration presets support both full-day
  and timed events; the live preview reflects the actual title and date range.
  Existing repeat-series editing, timezone validation and drag/resize remain.
- Settings include a live profile preview, seven-day schedule preview, weekday
  schedule starters, color swatches, illustrated icon choices, theme cards and a
  replayable animation preview. Exact inputs, explicit save actions and version
  conflict handling are retained. The color input is controlled so reopening a
  saved direction displays its saved color instead of the browser's black default.
- Notifications have an unread count, current-page read/unread filters, clearer
  status cards, snooze presets and colored delivery-channel groups. Read state
  persists after reload; choosing a snooze duration still requires confirmation.

## Motion and accessibility

Short card entrances, selected-state feedback, progress updates and a finite
animation preview extend the shared motion system. Account and operating-system
reduced-motion preferences suppress decorative movement. Buttons keep keyboard
activation, visible focus, accessible names and pressed states. Disabled form
fieldsets cover the new choices during saving and client initialization.

Responsive layouts cover both themes, narrow screens and expanded editors.
Technical explanations and optional fields remain available in existing
disclosures. No new dependency, external media request or API contract is added.

## Verification

The new browser scenarios use real local API/database persistence in isolated
accounts. They check task shortcuts and filters, explicit habit logging, goal
progress, calendar dates, profile/schedule/direction preferences, actual animation
CSS and saved reduced-motion behavior. The route audit visits 15 sections in both
themes at desktop and 360 px widths, expands primary editors and runs Axe WCAG
checks. Existing scenarios continue to cover the rest of the application.

- Production build, application/test TypeScript, ESLint and formatting pass.
  All 40 component/unit tests and all three repository-tooling tests pass.
- The final production build passed all 39 existing authenticated scenarios,
  including persistence, repeat submissions, stale editors, offline drafts,
  calendar movement rollback, focus controls/video and account isolation.
- The combined 42-scenario run exhausted the isolated fixture's 40-registration
  allowance before the three new scenarios could start. The fixture now permits
  80 registrations and has a 12-minute context within a 15-minute runner timeout;
  production limits are untouched. All three new scenarios pass in a separate
  run on the same final application build. The amended combined run was not
  repeated; the 42 scenarios are verified across these two runs.
- The new route sweep checks 15 sections in both themes, at desktop and mobile
  widths, including expanded editors and Axe WCAG checks. The existing sweep
  also passes for the 12 previously refreshed sections.
- All 14 public browser scenarios pass, including four viewport widths in both
  themes, keyboard/mobile navigation, reduced motion, retry behavior and forms
  waiting for client handlers on a slow connection.
- Local screenshots from the runs are preserved in `.data/workspaces-proof/`.
  They contain isolated test records, not the user's personal data.

Browser viewport and automated accessibility checks do not replace testing on
physical devices or with assistive technology. This is an interface revision of
the existing product, not delivery of unimplemented roadmap stages.
