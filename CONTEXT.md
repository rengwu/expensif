# Expensif Context

Language for the Expensif expense-tracking domain. This is a personal / small-household expense tracker, so terms are deliberately plain and user-facing.

## Language

**Expense**:
A single purchase or payment recorded by a user. It has an amount, category, description, date, currency, and optionally a payer.
_Avoid_: transaction, record, entry

**Category**:
A user-defined label that groups expenses, such as "food" or "transport".
_Avoid_: tag, bucket

**Description**:
A short free-text note explaining what the expense was for, such as "lunch with team".
_Avoid_: memo, note, detail

**Currency**:
The ISO 4217 code in which an expense amount is recorded, e.g. USD, MYR, JPY.
_Avoid_: money, denomination

**User**:
A person tracked in the system. A user can be selected as the payer for an expense.
_Avoid_: account, profile, person (when used as a generic substitute)

**Payer**:
The user who paid for an expense. Recorded as `paidBy` / `paidById` on an expense.
_Avoid_: paid by, spender

**Preferred Currency**:
The currency chosen in Preferences, used to display converted totals across expenses recorded in different currencies.
_Avoid_: base currency, home currency

**Preferences**:
The singleton settings for the application: preferred currency, default payer, and timezone.
_Avoid_: settings, config

**Exchange Rate**:
The conversion rate from the base currency (USD) to a target currency, fetched from Frankfurter and cached per day.
_Avoid_: fx rate, conversion factor

**Converted Amount**:
The expense amount expressed in the preferred currency for display. It is computed at render time, not stored.
_Avoid_: normalized amount, display amount

**Daily View**:
A page that is date-indexed: every day in the Window appears, newest first, whether or not it carries an expense, with a subtotal per spending day. Empty days render as muted rows. Rendered as a Ledger.

**Ledger**:
The Daily View's single continuous layout — no cards. Each day is one row: an empty day is a muted single line with an always-visible add affordance; a spending day lists its expenses between two Rails with a right-aligned day total. Month transitions carry a heavier divider.
_Avoid_: table, feed, list

**Rail**:
A thin vertical line running the full height of a day's row in the Ledger. Two Rails frame the expenses column and, because days carry no padding between them, read as unbroken lines down the page.
_Avoid_: border, divider, gutter

**Window**:
The rolling 30-day range ending at today that the Daily View renders on first load (`[today-29, today]`). Older Windows load on demand via infinite scroll until the earliest expense is reached. The Window never extends past today.
_Avoid_: range, page, view

**Upcoming**:
Future-dated days, shown as an ungapped continuation of the Ledger *above* today, marked only by a tinted date and capped at the three days nearest today (the rest collapse into one overflow row linking to the Calendar View). There is no separate "Upcoming section" — it is the same Ledger continuing.
_Avoid_: scheduled, future section, forecast

**Calendar View**:
A monthly grid showing which days have expenses and how much was spent, using a heat map to encode magnitude.

**List View**:
A page showing expenses in a single table with totals and conversion information, bounded to today: future-dated expenses (a Pending Charge among them) are excluded from the rows, the total and the category breakdown alike, so "total spent" means money actually spent. They remain visible on the Daily View until their date arrives.

**Export**:
Downloading the account's expenses as a CSV file, filtered by a date range plus an optional category and payer. Unlike the List View, whose filtering is client-side, Export filters on the server (the `ExpensesMatching` query, with a `CountMatching` sibling for the live count). One canonical US/ISO CSV: ten fixed columns, dot decimals, comma-separated, UTF-8 with a BOM.
_Avoid_: report, dump, backup, download (as a noun for the feature)

**Export Preview**:
The live row count and a raw sample of the first rows shown on the Export page as the filters change, so the size and contents are visible before committing to a download. The sample shows each expense's entered amount and currency, not the converted amount — conversion happens only in the downloaded file.
_Avoid_: preview pane, dry run

**Recurring Rule**:
A thing that charges on a schedule — rent, a subscription, an installment — listed on the Recurring page with its name, amount, currency, category, optional payer, interval ("Every 2 weeks"), optional end date, and whether it is enabled. A Rule is not a passive reference row: it mints ordinary Expenses on its own dates, and the Rule's name *is* their Description, so there is no separate description field. Disabling a Rule keeps its configuration but stops its clock entirely.
_Avoid_: subscription (not everything that recurs is one — rent and installments are Rules too), schedule, template

**Pending Charge**:
The one charge each enabled Rule keeps materialized ahead of today, so what is coming is visible in Upcoming and can be corrected before it happens. It is an ordinary future-dated Expense linked to its Rule — **not** a distinct type and not a state: nothing in the system asks whether an Expense is pending, there is no approval step, and when its date passes it simply becomes history while the next one is minted behind it. It can be edited or deleted like any other Expense.
_Avoid_: pending transaction, scheduled expense, draft, provisional (each implies a pending/confirmed state machine that deliberately does not exist)

**Anchor Day**:
The day of the month a monthly Rule aims for, stored separately from its next charge date and never re-derived from it. Each occurrence is min(anchor day, days in that month), so a Rule anchored on the 31st runs Jan 31 → Feb 28 → **Mar 31** → Apr 30: dates are always recomputed from the anchor, never accumulated onto the previous — possibly clamped — date. This is the single most important invariant in recurring expenses, because the failure it prevents is silent and slow.
_Avoid_: due day, billing day, day of month (when the distinction from the clamped charge date is what matters)

**Catch-up**:
Minting the charges a Rule owed while the app was not running — each on its own true date, in order — which is what the generation run at startup does. It covers time the *app* was absent and never a stretch a human deliberately paused: disabling a Rule stops its clock, and re-enabling resumes from today with nothing backfilled. A generation run reports how many expenses it added on the next page rendered — the ones it backfilled and the newly materialized Pending Charge alike — so expenses never appear silently.
_Avoid_: replay, sync, reconciliation (comparing expected charges against what a bank actually billed, which Expensif does not do)

**Monthly Equivalent**:
The single figure at the top of the Recurring page: every enabled Rule normalized to what it costs per month, converted into the Preferred Currency and summed. It is presented as approximate (`≈ $257.41 / month`) because week- and day-based intervals genuinely do not divide into a month — they are averaged over the mean 30.44-day one. Disabled Rules are excluded, and when any Rule cannot be converted the figure hides entirely rather than showing a partial total.
_Avoid_: monthly total, monthly spend, budget

**Category Suggestion**:
A recently used category offered as a quick-select pill in the add/edit form.

**Description Suggestion**:
A previously used description for a given category, offered as a quick-select pill in the add/edit form.

**Timezone**:
The IANA timezone (e.g. `Asia/Singapore`) used to compute "today", "yesterday", and calendar ranges. Dates are stored as bare `YYYY-MM-DD` strings; the timezone is applied at display time.
_Avoid_: locale, time offset

**Heat Level**:
A 1–5 quintile rank of a day's total spend relative to other days with any spend, used by the Calendar View to size and color the heat blob.
_Avoid_: intensity, score

**Island**:
A selectively hydrated interactive region mounted on an otherwise server-rendered HTML page. Each island has a named container and an entry script. Most islands are React components; the Daily View's infinite scroll and the Export page's live preview are vanilla-TypeScript islands that fetch and swap server-rendered HTML (see ADR 0001).
_Avoid_: widget, component (when the specific partial-hydration pattern matters)

## Cross-Context Notes

- **Date vs. Time**: Expensif stores only a calendar date (`YYYY-MM-DD`). The timezone is a display concern, not part of the stored domain fact.
- **Amount vs. Converted Amount**: The amount recorded on an expense is immutable relative to the expense's own currency. The converted amount is a view-model value derived from the exchange rate at display time.
- **User vs. Payer**: A user is a person in the system; a payer is the role that person plays for a specific expense.
- **Recurring Rule vs. Expense**: A Rule states what it *expects* to charge; the Expenses it mints are the record of what happened. A Rule owns only the future — its Pending Charge — and can never rewrite or delete an Expense whose date has passed, so deleting a Rule keeps the expenses it already created (they survive as ordinary unlinked ones).
