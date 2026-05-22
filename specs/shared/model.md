---
name: Shared Enums and Types
owns: shared/model/
depends:
---

# spec: shared/model

## rules

- All channel values: `sms`, `email`, `push` (immutable)
- All priority values: `high`, `normal`, `low` (immutable)
- All notification statuses: `pending`, `delivered`, `failed`, `cancelled` (immutable)
- IDs are UUID v7, app-generated (see STANDARDS.md)
- No types exported; only enums as string constants

## implementation notes

- Enums defined as string constants in `shared/model/enums.go`
- No separate types package; constants live alongside model
- Re-exported by both api/internal/model/ and processor/internal/model/ (avoids import cross-service)

## verification

- [ ] All enums appear in at least one spec's rules section
- [ ] Constants are untyped strings (const ChannelSMS = "sms", not `const ChannelSMS string = "sms"`)
- [ ] No unexported types in shared/model/
