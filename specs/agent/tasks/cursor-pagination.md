# cursor-pagination

**Specs:** `api-service/API_CONTRACT.md`
**Status:** pending

## Context

`GET /notifications` uses offset pagination (`page` + `page_size`), which requires PostgreSQL to scan all preceding rows. Since `id` is UUID v7 (time-ordered), it can serve directly as a cursor — no composite key or JSON encoding needed.

## What to change

### Cursor encoding

Cursor is the last row's `id` (UUID v7), base64url-encoded. Decoding failure or invalid UUID → 400.

### API change

Replace `page` with `cursor`. Response replaces `page`/`total_pages` with `next_cursor`:

```json
{
  "data": [...],
  "pagination": {
    "page_size": 20,
    "total": 4821,
    "next_cursor": "<base64url(uuid)> | null"
  }
}
```

`next_cursor` is null on the last page.

### Query

```sql
WHERE id < $cursor ORDER BY id DESC LIMIT $page_size + 1
```

Fetch `page_size + 1` rows; if `len(result) == page_size+1`, a next page exists — encode `result[page_size-1].id` as `next_cursor` and trim the slice to `page_size`.

### Changes

- `NotificationRepository`: add `ListByCursor(ctx, cursorID *uuid.UUID, pageSize int, filter ListFilter)`
- `NotificationService`: add `ListByCursor`
- `GET /notifications` handler: if `cursor` param present → `ListByCursor`; else → existing offset path

## Tests

- `TestListByCursor_firstPage` — no cursor → first N results + next_cursor
- `TestListByCursor_secondPage` — use next_cursor → correct second page, no overlap
- `TestListByCursor_lastPage` — next_cursor is null on final page
- `TestListByCursor_invalidCursor` — 400 on bad cursor
- `TestListByCursor_filtersPreserved` — status/channel filters work with cursor
