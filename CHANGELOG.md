## V3

### Changed
- Refactored release-group tiers into reusable `Define` rules.
- T1/T2/T3 scoring rules now reference shared group definitions with `matched()`.
- Updated smart 4K filtering rules to reference the same reusable definitions.
- Removed duplicated release-group regex logic from conditional filters.
- Preserved existing V2 ranking/filtering behaviour while improving maintainability.

### Notes
- `Define` rules do not score, reject, limit, or appear in `.MatchedRules`.
- This version requires a StreamNZB build with `RuleActionDefine` support.
