# Filtering physical New Arrivals shelves

## Finding

A physical shelf subfilter is feasible without scraping HTML. The existing public BiblioCommons per-record availability endpoint returns the actual collection/stack name for each copy, and Shelfcheck already preserves it. No working server-side collection search filter was established in this investigation.

## Live evidence

The following public requests were exercised against the Cincinnati catalog. These are snapshots, not promises of current availability.

- [The Wump availability](https://gateway.bibliocommons.com/v2/libraries/cincinnatilibrary/bibs/S170C4029776/availability?locale=en-US) returned an AVAILABLE Main Library copy with `collection` equal to `Main - 1st Floor South - Juvenile New Arrivals`.
- [Adult book search at Main, sorted by acquisition](https://gateway.bibliocommons.com/v2/libraries/cincinnatilibrary/bibs/search?locale=en-US&query=fiction&searchType=keyword&f_STATUS=1&f_FORMAT=BK&f_AUDIENCE=adult&sort=newly_acquired) returned normal search results and facets, but no collection/shelf facet. The first 15 returned titles were checked individually for availability. Ten had an AVAILABLE copy at the exact collection `Main - 1st Floor South - New Arrivals`.
- Matching examples were [The Names of the New World](https://cincinnatilibrary.bibliocommons.com/v2/record/S170C4034626), [A Tiny Little Favor](https://cincinnatilibrary.bibliocommons.com/v2/record/S170C4046648), and [Ghost Barn](https://cincinnatilibrary.bibliocommons.com/v2/record/S170C4032331).
- The same sample included `Main - 2nd Floor South - New Non Fiction`, ordinary Sci-Fi and Horror Fiction locations, and `Technical Services New Arrivals`. A generic substring match for "new arrivals" is therefore insufficient to identify the browsable adult section.
- Adding exploratory `f_COLLECTION=Main - 1st Floor South - New Arrivals` to the working search caused HTTP 500. An invented collection value also caused HTTP 500. As a control, a nonexistent `f_STATUS` branch returned zero matches successfully. These results rule out that particular raw collection-filter approach; they do not prove no alternative upstream search syntax exists.

Raw public-copy evidence and filter-probe results were saved in the agent workspace as `shelfcheck-arrivals-evidence.json` and `shelfcheck-arrivals-filter-probes.json`. They contain no account data. The sample was intentionally bounded and is not a shelf inventory.

## Existing code

- `internal/bibliocommons/api.go:154` implements availability fetching using `bibs/<id>/availability`.
- `internal/bibliocommons/api.go:340` reads `rawItem.Collection`; `toCopy` preserves it as `Copy.Collection`.
- `internal/bibliocommons/api.go:42` sends raw search facet filters as `f_<FIELD>`.
- `internal/tui/filters.go:29` defines the current filter categories. There is no physical shelf category.
- The JSON book command already exposes collection names as each copy's `shelf` field.

## Recommended implementation

Add an explicitly local `Shelf / section` subfilter, initially branch-specific. Keep ordinary search facets upstream, then fetch availability for candidate records and admit a title only when the SAME copy satisfies branch, selected shelf, and availability requirements. Do not combine availability from one branch with a matching collection on another copy.

For Main, useful selectable labels include Adult New Arrivals, Juvenile New Arrivals, and New Nonfiction, backed by the exact observed upstream labels. Preserve raw labels for display. Exclude Technical Services and on-order copies from an available-to-browse view.

Load candidate pages incrementally until enough matching rows are found or a clearly declared scan budget is reached. Deduplicate by bib ID, reuse the existing availability cache and request throttle, preserve cancellation, and report how many candidates were checked. An upstream total or facet count is not the filtered shelf count. Fetch failures must remain unknown/incomplete, not silently become nonmatches.

Sorting by recently acquired is a useful candidate-ordering optimization for a dedicated New Arrivals browser. Do not impose a 30-day acquisition cutoff as the meaning of the physical shelf filter: a book may remain in that section longer. Shelf filtering should continue to work with an ordinary user search and its genre/audience/format filters.

This adds per-record requests, so it is more expensive than a server-side facet. Bounded incremental loading is a better first implementation than crawling the whole catalog or claiming a complete inventory from the first page.

## Scope

Research only. No application code changed and no accounts were written. A separate entry-point investigation covers the library's public New Titles pages, which must not be assumed equivalent to physical New Arrivals shelving.
