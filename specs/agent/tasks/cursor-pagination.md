# cursor-pagination

**Specs:** `api-service/API_CONTRACT.md`
**Status:** pending

## Context

`GET /notifications` currently uses offset-based pagination (`page` + `page_size`). For large
datasets this becomes expensive as PostgreSQL must scan and discard all preceding rows. Cursor-based
pagination (keyset pagination) is O(log n) regardless of depth.

## What to change

### API contract addition

New query parameter: `cursor` (opaque string, base64-encoded). When present, `page` is ignored.

Response gains a `next_cursor` field (null on last page):

```json
{
  "data": [...],
  "pagination": {
    "page_size": 20,
    "total": 4821,
    "next_cursor": "eyJpZCI6Inh4eCIsImNyZWF0ZWRfYXQiOiIuLi4ifQ=="
  }
}
```

### Cursor encoding

Cursor encodes the last row's (`created_at`, `id`) pair as JSON, base64url-encoded.
Decoding failure → 400 VALIDATION_ERROR.

### Repository change

Add `ListByCursor` to `NotificationRepository`:

```
ListByCursor(ctx, cursor *Cursor, pageSize int, filter ListFilter) ([]*model.Notification, *Cursor, error)
  — WHERE (created_at, id) < (cursor.CreatedAt, cursor.ID)  [for DESC order]
  — LIMIT pageSize + 1 (peek to detect next page)
  — returns next cursor if len(result) == pageSize+1, else nil
```

### Service change

Add `ListByCursor` to `NotificationService`. Existing `List` (offset) remains for backwards
compatibility during transition.

### Handler change

`GET /notifications`: if `cursor` param present → call `ListByCursor`; else → existing offset path.

## Tests

- `TestListByCursor_firstPage` — no cursor → returns first N results + next_cursor
- `TestListByCursor_secondPage` — decode next_cursor from first page → correct second page
- `TestListByCursor_lastPage` — next_cursor is null on final page
- `TestListByCursor_invalidCursor` → 400
- `TestListByCursor_filtersPreserved` — status/channel filters apply with cursor
