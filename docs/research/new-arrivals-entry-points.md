# New arrivals entry points

Verified 2026-11-01 through public unauthenticated HTML and gateway GET requests. No application changes or private account files.

## Findings

The official [Books page](https://chpl.org/books/) and [New Titles page](https://cincinnatilibrary.bibliocommons.com/explore/recent_arrivals) expose catalog acquisition searches, not physical shelf selectors. The inspected links use acquisition age, on-order state, classification/genre queries and format/audience/language facets. None contains a branch-specific physical stack selector. This is a scoped negative finding, not proof that the catalog cannot search stacks elsewhere.

Do not label these searches "Physical New Arrivals shelf". A matching title does not establish that any copy is available or occupies a New Arrivals stack. Copy-level availability and location metadata must answer that separately. The unfiltered gateway response includes digital FORMAT facets such as EBOOK, EAUDIOBOOK, VIDEO_ONLINE and MUSIC_ONLINE, further distinguishing it from a physical display.

## Just Arrived and View All New Titles

- Default New Titles: <https://cincinnatilibrary.bibliocommons.com/explore/recent_arrivals>
- Just Arrived: <https://cincinnatilibrary.bibliocommons.com/explore/recent_arrivals?f_on_order=false>
- On Order: <https://cincinnatilibrary.bibliocommons.com/explore/recent_arrivals?f_on_order=true>

Just Arrived is active by default. Its View All New Titles anchor uses `custom_query=nw:[0 TO 180]`, `f_on_order=false`, and legacy descending NEWLY_ACQUIRED sort parameters. Fetching that link with spaces percent-encoded redirects to this verified URL:

<https://cincinnatilibrary.bibliocommons.com/v2/search?f_ON_ORDER=false&origin=core-catalog-explore&query=nw%3A%5B0%20TO%20180%5D&searchType=bl&sort=NEWLY_ACQUIRED&suppress=true&title_key=all_newly_acquired>

The On Order page's View All anchor uses the same `nw:[0 TO 180]` query with `f_on_order=true`. On-order false is not an available-now condition. The 180-day acquisition window differs from the Books page's 30-day carousels.

### Verified gateway execution

<https://gateway.bibliocommons.com/v2/libraries/cincinnatilibrary/bibs/search?query=nw%3A%5B0+TO+180%5D&searchType=bl&sort=newly_acquired&f_ON_ORDER=false&locale=en-US>

This public request succeeded with JSON keys `catalogSearch`, `entities`, and `rssURL`. `catalogSearch` echoed query `nw:[0 TO 180]`, search type `bl`, applied sort `newly_acquired`, and applied ON_ORDER facet value `false`. Pagination reported 40,529 results at research time, with limit 25. This is a transient observation. The website also exposes a gateway `/rss/search` link with equivalent query/filter values.

Shelfcheck can use the existing bibs/search gateway for this acquisition-discovery mode. This is not a documented stable upstream contract, and no physical-stack query was verified in this investigation.

## Page filters

The page exposes `formats`, `fictype`, `audience`, `lang`, and `f_on_order`. Verified combined page:

<https://cincinnatilibrary.bibliocommons.com/explore/recent_arrivals?formats=BK&fictype=FICTION&audience=adult&lang=eng&f_on_order=false>

Its View All anchor preserves all of those values and redirects to:

<https://cincinnatilibrary.bibliocommons.com/v2/search?f_AUDIENCE=adult&f_FICTION_TYPE=FICTION&f_FORMAT=BK&f_LANGUAGE=eng&f_ON_ORDER=false&origin=core-catalog-explore&query=nw%3A%5B0%20TO%20180%5D&searchType=bl&sort=NEWLY_ACQUIRED&suppress=true&title_key=all_newly_acquired>

This verifies the translations `formats` to `f_FORMAT`, `fictype` to `f_FICTION_TYPE`, `audience` to `f_AUDIENCE`, `lang` to `f_LANGUAGE`, and `f_on_order` to `f_ON_ORDER`.

The Mystery category anchor was identical on default, combined-filter, and On Order pages. It did not inherit the selected format, language, audience or on-order state. Category presets are separate searches, not guaranteed compositions with page filters.

## Books page carousels

Source: embedded `carouselTitleLink` configuration in <https://chpl.org/books/>, under "New Titles in the Past 30 Days". The exact configured links and decoded queries follow. These are verified source configurations; their destinations were not independently executed.

### Fiction

<https://cincinnatilibrary.bibliocommons.com/v2/search?custom_edit=true&f_FORMAT=BK&f_NEWLY_ACQUIRED=PAST_30_DAYS&query=contentclass%3A%22FICTION%22%20AND%20oo%3A%28false%29&searchType=bl&suppress=true&title=New%20Titles%20in%20the%20Past%2030%20Days%20%7C%20Fiction>

Query: `contentclass:"FICTION" AND oo:(false)`

Other search parameters: `f_FORMAT=BK`, `f_NEWLY_ACQUIRED=PAST_30_DAYS`, `searchType=bl`.

### Non-Fiction

<https://cincinnatilibrary.bibliocommons.com/v2/search?custom_edit=true&f_FORMAT=BK&f_NEWLY_ACQUIRED=PAST_30_DAYS&query=contentclass%3A%22NONFICTION%22%20oo%3A%28false%29&searchType=bl&suppress=true&title=New%20Titles%20in%20the%20Past%2030%20Days%20%7C%20Non-Fiction>

Query: `contentclass:"NONFICTION" oo:(false)`

Other search parameters: `f_FORMAT=BK`, `f_NEWLY_ACQUIRED=PAST_30_DAYS`, `searchType=bl`.

### Large Print

<https://cincinnatilibrary.bibliocommons.com/v2/search?custom_edit=true&f_FORMAT=LPRINT&f_NEWLY_ACQUIRED=PAST_30_DAYS&query=oo%3A%28false%29&searchType=bl&suppress=true&title=New%20Titles%20in%20the%20Past%2030%20Days%20%7C%20Large%20Print>

Query: `oo:(false)`

Other search parameters: `f_FORMAT=LPRINT`, `f_NEWLY_ACQUIRED=PAST_30_DAYS`, `searchType=bl`.

### On Order

<https://cincinnatilibrary.bibliocommons.com/v2/search?custom_edit=true&query=formatcode%3A%28BK%29%20oo%3Atrue%20pubyear%3A%5B2011%20TO%202021%5D&searchType=bl&suppress=true&title=New%20Titles%20in%20the%20Past%2030%20Days%20%7C%20On%20Order>

Query: `formatcode:(BK) oo:true pubyear:[2011 TO 2021]`

Other search parameters: `searchType=bl`.

The On Order carousel has a fixed `pubyear:[2011 TO 2021]` condition and no PAST_30_DAYS facet. Record that as a first-party configuration anomaly, not a recommended new-release definition. Non-Fiction uses whitespace rather than explicit AND before `oo:(false)`; the exact source query is preserved above.

## Category preset inventory

Source: exact first-party anchors in <https://cincinnatilibrary.bibliocommons.com/explore/recent_arrivals>. Desktop/mobile duplicate label-and-URL pairs were deduplicated programmatically. There are 39 category presets plus View All. The URLs below resolve the original hrefs against the catalog host and percent-encode literal spaces only. Query spacing and aliases are preserved.

All presets include legacy descending NEWLY_ACQUIRED sort fields. The Mystery destination was fetched and verified to redirect to:

<https://cincinnatilibrary.bibliocommons.com/v2/search?f_FICTION_TYPE=FICTION&origin=core-catalog-explore&query=gh%3Amystery%20audience%3A%22adult%22%20AND%20nw%3A%5B0%20TO%20180%5D&searchType=bl&sort=NEWLY_ACQUIRED&suppress=true&title=Mystery>

Other category URLs below were verified as source anchors, not individually executed. They use `ca:`, `gh:`, `sh:`, `an:`, and `ge:` classification/genre/subject-style criteria, not stack IDs. Some use `nw:180` rather than a range. Music uses the smart keyword query `new music`. Do not silently normalize those differences or assert equivalent behavior without further testing.

### Architecture

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=ca:72*%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Architecture>

Query: `ca:72* AND nw:[0 TO 180]`

### Arts & Entertainment

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=(ca:7*%20%20-ca:79*)%20%20%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Arts%20%26%20Entertainment>

Query: `(ca:7*  -ca:79*)   AND nw:[0 TO 180]`

### Biography

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=gh:biography%20%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Biography>

Query: `gh:biography  AND nw:[0 TO 180]`

### Business

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=sh:business%20AND%20nw:%5B0%20TO%20180%5D%20+-ft:FICTION&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Business>

Query: `sh:business AND nw:[0 TO 180]  -ft:FICTION`

### Computers

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=(ca:004*%20OR%20ca:005*%20OR%20ca:006*)%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Computers>

Query: `(ca:004* OR ca:005* OR ca:006*) AND nw:[0 TO 180]`

### Cooking

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=(ca:641*%20OR%20ca:642*)%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Cooking>

Query: `(ca:641* OR ca:642*) AND nw:[0 TO 180]`

### Crafts & Hobbies

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=(an:crafts%20OR%20an:hobbies)%20AND%20nw:%5B0%20TO%20180%5D%20+-ft:FICTION&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Crafts%20%26%20Hobbies>

Query: `(an:crafts OR an:hobbies) AND nw:[0 TO 180]  -ft:FICTION`

### Economics

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=ca:33*%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Economics>

Query: `ca:33* AND nw:[0 TO 180]`

### Education

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=ca:37*%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Education>

Query: `ca:37* AND nw:[0 TO 180]`

### Gardening

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=ca:635*%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Gardening>

Query: `ca:635* AND nw:[0 TO 180]`

### General Information & Media

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=ca:0*%20-(ca:004*%20OR%20ca:005*%20OR%20ca:006*)%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=General%20Information%20%26%20Media>

Query: `ca:0* -(ca:004* OR ca:005* OR ca:006*) AND nw:[0 TO 180]`

### Health & Fitness

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=ca:61*%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Health%20%26%20Fitness>

Query: `ca:61* AND nw:[0 TO 180]`

### History

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=ca:9*%20-ca:91*%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=History>

Query: `ca:9* -ca:91* AND nw:[0 TO 180]`

### Interior Design

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=(ca:747*%20OR%20ca:749*)%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Interior%20Design>

Query: `(ca:747* OR ca:749*) AND nw:[0 TO 180]`

### Language

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=ca:4*%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Language>

Query: `ca:4* AND nw:[0 TO 180]`

### Philosophy

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=(ca:1*%20-ca:15*%20-ca:13*)%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Philosophy>

Query: `(ca:1* -ca:15* -ca:13*) AND nw:[0 TO 180]`

### Political Science & Law

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=(ca:32*%20OR%20ca:34*)%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Political%20Science%20%26%20Law>

Query: `(ca:32* OR ca:34*) AND nw:[0 TO 180]`

### Psychology

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=(ca:15*%20OR%20ca:13*)%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Psychology>

Query: `(ca:15* OR ca:13*) AND nw:[0 TO 180]`

### Religion

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=ca:2*%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Religion>

Query: `ca:2* nw:[0 TO 180]`

### Science & Mathematics

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=ca:5*%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Science%20%26%20Mathematics>

Query: `ca:5* nw:[0 TO 180]`

### Social sciences

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=ca:3*%20nw:180&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Social%20sciences>

Query: `ca:3* nw:180`

### Sports

<https://cincinnatilibrary.bibliocommons.com/search?suppress=true&custom_query=ca:79*%20%20-ca:791*%20-ca:792*%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&title=Sports>

Query: `ca:79*  -ca:791* -ca:792* AND nw:[0 TO 180]`

### Technology

<https://cincinnatilibrary.bibliocommons.com/search?suppress=true&custom_query=ca:6*%20nw:180&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&title=Technology>

Query: `ca:6* nw:180`

### Travel & Geography

<https://cincinnatilibrary.bibliocommons.com/search?suppress=true&custom_query=ca:91*%20nw:180&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&title=Travel%20%26%20Geography>

Query: `ca:91* nw:180`

### Movies

<https://cincinnatilibrary.bibliocommons.com/search?suppress=true&custom_query=formatcode:%22DVD%22%20AND%20nw:%5B0%20TO%20180%5D&audience=adult&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&title=Movies>

Query: `formatcode:"DVD" AND nw:[0 TO 180]`

Additional parameters: `audience=adult`.

### Music

<https://cincinnatilibrary.bibliocommons.com/search?t=smart&search_category=keyword&q=new%20music&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Music>

Query: `new music`

Additional parameters: `t=smart`, `search_category=keyword`.

### Fiction

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=audience:%22adult%22%20AND%20nw:%5B0%20TO%20180%5D&f_fiction_type=FICTION&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Fiction>

Query: `audience:"adult" AND nw:[0 TO 180]`

Additional parameters: `f_fiction_type=FICTION`.

### Mystery

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=gh:mystery%20audience:%22adult%22%20AND%20nw:%5B0%20TO%20180%5D&f_fiction_type=FICTION&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Mystery>

Query: `gh:mystery audience:"adult" AND nw:[0 TO 180]`

Additional parameters: `f_fiction_type=FICTION`.

### Fantasy

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=gh:fantasy%20audience:%22adult%22%20AND%20nw:%5B0%20TO%20180%5D&f_fiction_type=FICTION&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Fantasy>

Query: `gh:fantasy audience:"adult" AND nw:[0 TO 180]`

Additional parameters: `f_fiction_type=FICTION`.

### Large Print

<https://cincinnatilibrary.bibliocommons.com/search?suppress=true&custom_query=formatcode:%22LPRINT%22%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&title=Large%20Print>

Query: `formatcode:"LPRINT" AND nw:[0 TO 180]`

### Graphic Novels

<https://cincinnatilibrary.bibliocommons.com/search?suppress=true&custom_query=gh:%22graphic%20novels%22%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&title=Graphic%20Novels>

Query: `gh:"graphic novels" AND nw:[0 TO 180]`

### Poetry

<https://cincinnatilibrary.bibliocommons.com/search?suppress=true&custom_query=(ca:811*+OR+ca:821*+OR+ca:831*+OR+ca:841*+OR+ca:851*+OR+ca:861*+OR+ca:871*+OR+ca:881*)+AND+nw:%5B0+TO+180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&title=Poetry>

Query: `(ca:811* OR ca:821* OR ca:831* OR ca:841* OR ca:851* OR ca:861* OR ca:871* OR ca:881*) AND nw:[0 TO 180]`

### Drama

<https://cincinnatilibrary.bibliocommons.com/search?suppress=true&custom_query=ca:8*%20AND%20ge:drama%20audience:%22adult%22%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&title=Drama>

Query: `ca:8* AND ge:drama audience:"adult" AND nw:[0 TO 180]`

### Humor

<https://cincinnatilibrary.bibliocommons.com/search?suppress=true&custom_query=(gh:humor%20OR%20gh:humour)%20%20AND%20nw:%5B0%20TO%20180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&title=Humor>

Query: `(gh:humor OR gh:humour)  AND nw:[0 TO 180]`

### Children's Fiction

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=audience:%22juvenile%22+-fo:dvd++AND+nw:%5B0+TO+180%5D&f_fiction_type=FICTION&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Children's%20Fiction>

Query: `audience:"juvenile" -fo:dvd  AND nw:[0 TO 180]`

Additional parameters: `f_fiction_type=FICTION`.

### Children's Non-fiction

<https://cincinnatilibrary.bibliocommons.com/search?custom_query=audience:%22juvenile%22+-fo:dvd++AND+nw:%5B0+TO+180%5D&f_fiction_type=NONFICTION&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&suppress=true&title=Children's%20Non-fiction>

Query: `audience:"juvenile" -fo:dvd  AND nw:[0 TO 180]`

Additional parameters: `f_fiction_type=NONFICTION`.

### Children's Movies

<https://cincinnatilibrary.bibliocommons.com/search?suppress=true&custom_query=audience:%22juvenile%22+formatcode:%22DVD%22+nw:%5B0+TO+180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&title=Children's%20Movies>

Query: `audience:"juvenile" formatcode:"DVD" nw:[0 TO 180]`

### Children's Music

<https://cincinnatilibrary.bibliocommons.com/search?suppress=true&custom_query=audience:%22juvenile%22+formatcode:%22MUSIC_CD%22+nw:%5B0+TO+180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&title=Children's%20Music>

Query: `audience:"juvenile" formatcode:"MUSIC_CD" nw:[0 TO 180]`

### Teen Material

<https://cincinnatilibrary.bibliocommons.com/search?suppress=true&custom_query=audience:%22teen%22+AND+nw:%5B0+TO+180%5D&sort%5Bfield%5D=NEWLY_ACQUIRED&sort%5Btype%5D=BIB_FIELDS&sort%5Bdirection%5D=descending&origin=core-catalog-explore&title=Teen%20Material>

Query: `audience:"teen" AND nw:[0 TO 180]`

## Scope and access limits

Keep acquisition age, publication year, on-order state, available-now status, branch, and physical copy stack separate. No server-side physical-location query was established from these entry points. The main investigation's copy-metadata findings are separate evidence and are not claimed as independently replicated here.

The text extractor omitted the links and most dynamic content. Direct HTML GET requests exposed legacy catalog anchors and embedded Books carousel configuration. Browser automation could not start its daemon, so no interactive browser behavior is claimed. BeautifulSoup was unavailable; extraction used the Python standard library instead. No dependencies were installed and no login was used.
