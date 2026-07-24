# Community alternate-name mappings (TSV).
# Format: variant<TAB>law_id  (law_id = normalized catalog id, e.g. bgb)
# See variants.tsv for examples.

Linked ordinances (amendment-by-reference) live in `linked_instruments.tsv`:

```text
parent_law_id<TAB>kind<TAB>gii_slug<TAB>notes[<TAB>effective_from<TAB>section_hint]
```

- `notes` must include a parseable BGBl citation (fail-safe freshness).
- Optional `effective_from` (`YYYY-MM-DD` Inkrafttreten) and `section_hint` (e.g. `§ 1`) drive past/current/future status per section.
- Example: `milog` → `milov4` / `milov5` (section-scoped rate Verordnungen, not full MiLoG replacements).
