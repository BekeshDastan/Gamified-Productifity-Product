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

## Finishing pass — 16 October 2026

The follow-up remains focused on the implemented application, as requested.
No social or integration roadmap stage is started in this pass.

- The guide now offers three interactive routes: starting with a small action,
  focusing on a task and finishing the day. Each route has three concrete steps
  linked to working screens. Finite staggered motion respects both account and
  operating-system reduced-motion preferences. Text remains fully opaque during
  the transition so its contrast does not temporarily fall below the readable level.
- Privacy preferences use illustrated choices and keyboard-accessible native
  checkbox switches. The page clearly separates today's private records from
  preferences for future social features. Changing these preferences does not
  publish anything or introduce sharing APIs.
- Security has verification status and device cards identifying the current
  session. Existing session revocation remains server-enforced. Registration,
  login, password reset and password change have explicit show/hide controls;
  these preserve the input value and do not submit the form.
- Loading and failed-query states in tasks, goals and the settings editors now
  use shared visual states with retry controls. Unknown application URLs lead
  back to the user's day; unknown public URLs still lead to the landing page.
- Success notices in task, goal, profile, schedule, appearance, direction/tag and
  privacy editors disappear when a new unsaved change is entered. They no longer
  imply that the current draft has already been saved.
- Added real-browser coverage for keyboard password visibility, saved privacy
  choices, failed-save recovery, selective device revocation, guide navigation,
  reduced motion, failed-query retry and both 404 destinations. The guide is also
  audited at 320 px in both themes. Existing profile coverage checks that a saved
  notice clears on editing.

The growing browser suite has an 18-minute isolated database lifetime and a
20-minute outer runner timeout. Each browser scenario retains its own bounded
timeout. This change affects only the acceptance runner, not production limits.

### Verification of the finishing pass

- One complete production-browser run passed all **45 authenticated scenarios**
  in 11.7 minutes, using the real Go API, isolated PostgreSQL schema and SMTP
  fixture. This closes the previous pass's separate-run acceptance gap.
- The two route sweeps passed for 12 and 15 sections in both themes at desktop
  and 360 px widths, including expanded editors; the new guide also passes at
  320 px. Axe checks and horizontal-overflow assertions passed.
- All **40 component/unit tests** passed. Production build, application and test
  TypeScript, ESLint, repository checks and whole-repository formatting passed.
- Local screenshots and logs are preserved under `.data/support-proof/` and
  `.data/support-*.log`. Browser records are isolated test data.
- All **14 public browser scenarios** and all **three tooling tests** passed,
  including delayed JavaScript readiness, keyboard/mobile navigation, both
  404 return paths and recovery from a server error.
