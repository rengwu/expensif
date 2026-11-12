---
type: task
blocked_by: []
---

# Validate expense dates on the write path

## Question

`validateExpenseInput` (`internal/service/service.go:47-66`) checks the amount, the
category, and the description. It does not check the date. It only substitutes today
when the date is empty.

Proven by probe, not inference — all returned `201`:

    {"date":"2031-01-01"}   accepted, renders above today
    {"date":"banana"}       accepted, renders as a day header reading "banana"

`templates/form.html:46` is an `<input type="date">` with no `max`, so the browser
won't stop a future date either, and the API has no browser at all.

[What happens to expenses dated after today](./03-future-dated-expenses.md) decided
that garbage dates get fixed at the source rather than defended against on read. This
ticket specifies that fix.

## Answer

Implemented date validation in `validateExpenseInput`.

- A non-empty date must parse as `2006-01-02`. Anything else returns `ErrInvalidDate`.
- Future dates remain valid; Upcoming depends on them (ticket 03).
- Empty dates still default to today.
- Both `CreateExpense` and `UpdateExpense` are covered because both call `validateExpenseInput`.
- `ErrInvalidDate` is mapped to `400 Bad Request` by `isValidationErr` in `handlers_api.go`.
- The HTML form already uses `<input type="date">`, which constrains browsers to well-formed dates; no `max=` attribute was added because future dates are allowed.
- Existing rows with unparseable dates are left as-is. This was accepted in ticket 03 and this ticket is scoped to the write path only.

Files changed:
- `internal/service/service.go` — added `ErrInvalidDate`, parse check in `validateExpenseInput`.
- `internal/web/handlers_api.go` — `isValidationErr` includes `ErrInvalidDate`.
- `internal/service/service_test.go` — added tests for invalid, valid, past, and future dates.
- `internal/web/handlers_api_test.go` — added tests for invalid date on create and update, and updated `TestIsValidationErr`.
