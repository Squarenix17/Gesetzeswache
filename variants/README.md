# Community alternate-name mappings (TSV).
# Format: variant<TAB>law_id  (law_id = normalized catalog id, e.g. bgb)
# See variants.tsv for examples.

Linked ordinances (amendment-by-reference) live in `linked_instruments.tsv`:

```text
parent_law_id<TAB>kind<TAB>gii_slug<TAB>notes[<TAB>effective_from<TAB>section_hint]
```

- `notes` must include a parseable BGBl citation (fail-safe freshness).
- Optional `effective_from` (`YYYY-MM-DD` Inkrafttreten) and `section_hint` (e.g. `§ 1`) drive past/current/future status per section.
- Required when a parent has **two or more** rows: every row must include both `effective_from` and `section_hint`.
- Example: `milog` → `milov4` / `milov5` (section-scoped rate Verordnungen, not full MiLoG replacements).
- Example: `sgb11` → `pbav_2025` (Pflege-Beitragssatz under § 55; parent body may still show an older rate).
- **Discovery:** high-confidence links are also auto-discovered from child Ermächtigung + fundstelle during sync. TSV rows **override** discovered rows for the same parent+slug.

Fortschreibung families (`fortschreibung_families.tsv`) map consumer laws (e.g. `sgb2`, `asylblg`, `sgb14`) to a **slug prefix** such as `rbsfv_`. At runtime the latest catalog year matching that prefix (e.g. `rbsfv_2026`) is attached as a seeded linked instrument with section-scoped coverage. Format:

```text
consumer_parent_id<TAB>slug_prefix<TAB>section_hint<TAB>notes
```

`notes` must include a parseable BGBl citation (same rule as `linked_instruments.tsv`).
