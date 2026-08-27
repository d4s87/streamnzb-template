## V3

### Changed
- Refactored release-group tiers into reusable `Define` rules.
- T1/T2/T3 scoring rules now reference shared group definitions with `matched()`.
- Updated smart 4K filtering rules to reference the same reusable definitions.
- Removed duplicated release-group regex logic from conditional filters.
- Preserved existing V2 ranking/filtering behaviour while improving maintainability.

### Define Library
- Moved Vidhin-backed release-group definitions out of `profile.txt` into a shared StreamNZB Define Library.
- The library is maintained in `generated/streamnzb-defines.txt`.
- Added 33 shared Define rules covering Movie, Show and Anime release-group classifications, including Movie and Show LQ groups.
- Release-group definitions are synchronized with Vidhin05/Releases-Regex through GitHub Actions.
- Upstream changes are reviewed through pull requests before being published to the library.
- `profile.txt` retains the scoring and filtering policy and references library definitions through `matched()`.
- Linked Define Library updates can be reviewed and applied using StreamNZB's **Refresh** action.

### Notes
- `Define` rules do not score, reject, limit, or appear in `.MatchedRules`.
- V3 requires the shared Define Library to be imported before using `profile.txt`.
- This version requires a StreamNZB build with shared Define Library and `matched()` support.
