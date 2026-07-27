# React Islands Architecture

We render the application as server-side Go HTML templates and hydrate only small, interactive islands with React. Go owns routing, data fetching, and the initial document; React owns the few widgets that need client-side interactivity (pills, tables, mobile navigation).

We chose this over a full SPA because the app's pages are document-like (daily list, calendar, tables) and most of the UI does not need client-side state. We chose it over vanilla JS because the interactive widgets are complex enough that React's component model and testability are worth the build step.

This decision is the foundation of the current frontend architecture. Reversing it would require rebuilding routing, state management, and data loading on the client.

## Note (daily-timeline and export-csv efforts)

The Daily View's infinite-scroll island is the first island that is **not** React: ~30 lines of vanilla TypeScript that fetch a server-rendered HTML fragment from `GET /daily/older` and append it — no components, no JSX, no hydration. This does not reverse the decision. Go still owns routing, data, and the document; the fragment is rendered by the same Go partial as the first window, so no view logic moved to the client. React was skipped here precisely because the island holds no state and renders no markup — reaching for it would have added a build-time component and a hydration step for a sentinel, a fetch, and an `insertAdjacentHTML`. "React owns the islands" is therefore no longer universally true: React owns the islands that carry client-side *state or markup*; an island that only moves server-rendered HTML into the page does not need it.

The Export page's live-preview island (`GET /export/preview`) is the **second** such island and confirms the pattern is not a one-off: it too is vanilla TypeScript moving a server-rendered fragment into the page (`fetch` → `innerHTML`), so it holds no state or markup and skips React for the same reason. It differs only in trigger — it is **input-driven**, re-fetching as the export filters change, where daily-scroll is scroll-triggered. The rule therefore reads: React owns the islands that carry client-side *state or markup*; an island that only moves server-rendered HTML into the page does not — whether a scroll appends it or a filter change replaces it. (The Export page also reverses that effort's own "a form + link needs no island" default: the live preview earns an island because live reassurance needs a fetch, and stays vanilla because it moves server HTML — see the export-csv plan, ticket 05.)
