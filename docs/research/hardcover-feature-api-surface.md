# Hardcover feature/API surface for Shelfcheck

**Scope:** documented, first-party API evidence checked 2026-10-12. Hardcover’s supported integration surface is authenticated GraphQL `POST https://api.hardcover.app/v1/graphql`; OAuth discovery also publishes authorization, token, revocation, and device-authorization endpoints.[1][3] The official API is in beta, so this is a build shortlist rather than a stability guarantee.[4]

## Evidence standard

| Label | Meaning |
| --- | --- |
| **Verified** | Official documentation names the GraphQL root field/mutation and its scope, or provides a concrete request example. |
| **Documented data only** | Official schema documents readable fields, but not the exact write payload needed for the proposed UX. |
| **UI-only / do not infer** | A feature seen on the website without an identified documented operation. It is excluded from implementation commitments. |

## Build candidates with verified API operations

All rows use the GraphQL endpoint above with an OAuth Bearer token or PAT. OAuth is the appropriate user-facing path; official docs specifically say not to ask users to paste PATs into an app.[1][4]

| Priority | Shelfcheck feature | Verified endpoint / GraphQL root field | Request type and minimum scope | Confidence | Constraints / risks |
| --- | --- | --- | --- | --- | --- |
| 1 | **Connect a Hardcover account** | OAuth discovery: `https://api.hardcover.app/.well-known/oauth-authorization-server`; `https://hardcover.app/oauth2/authorize`; `https://api.hardcover.app/oauth2/token`; GraphQL `me` | OAuth authorization-code + PKCE, or device authorization for CLI/headless clients; `read:me:content` is the official example scope.[1][2][3] | High | Store refresh/access tokens securely and revoke on disconnect. PATs are private credentials and not suitable for a multi-user connection flow.[4] |
| 1 | **Read and filter the user’s shelf** (Want to Read, Reading, Read, Paused, DNF, Ignored) | `me { user_books(...) }`, or `user_books`, `user_books_by_pk`, `user_books_aggregate`; fields include status, rating, dates, ownership, notes, and read count. Status IDs are documented (1–6). | Query; `read:library` (or public/following/private variants).[5][8] | High | Paginate and request only needed fields; private data must only be used on behalf of the authorizing user.[4] |
| 1 | **Update shelf state, rating, review, ownership, and reading dates** | `insert_user_book`, `update_user_book`, `delete_user_book` | Mutation; `write:library`. The schema documents `status_id`, `rating` (0–5 halves), `review_markdown`, spoiler flag, `owned`, first/last read dates, and edition fields.[5][8] | High | Treat mutations as destructive/account-changing; confirm intent and read back after writes. The public action list verifies operation names/scopes, but exact input shapes should be validated in the current GraphQL explorer before implementation. |
| 1 | **Currently-reading progress and read-through history** | `user_book_reads`, `user_book_reads_by_pk`; `insert_user_book_read`, `update_user_book_read`, `delete_user_book_read`, `upsert_user_book_reads`. Official guide queries `me → user_books(status=2) → user_book_reads { progress_pages }`. | Query + mutation; reads use `read:library`, writes `write:library`.[8][9] | High | The guide proves `progress_pages`; confirm the desired percent/location/date fields and upsert semantics against the live schema before exposing a progress editor. |
| 2 | **Reading journal / quotes / notes timeline** | `reading_journals`, `reading_journals_by_pk`, `reading_journals_summary`; `insert_reading_journal`, `update_reading_journal`, `delete_reading_journal` | Query uses `read:journal`; mutation names are documented under `write:library`.[8][10] | High for read; medium for editor | Journal schema supports events such as note, quote, status, rating, review, and progress update, plus `entry`, `metadata`, date, and privacy fields.[10] Exact write input/event constraints are not shown in the schema page. |
| 2 | **Personal reading goals dashboard and editor** | `goals`, `goals_by_pk`; `insert_goal`, `update_goal`, `update_goal_progress`, `delete_goal` | Query `read:goals`; mutations `write:goals`.[7][8] | High | Supported fields include target, metric (books/pages), progress, dates, state, conditions, and privacy.[7] Exact `conditions` JSON and lifecycle behavior need schema/explorer validation. |
| 2 | **Create/manage reading lists; add/reorder books; follow public lists** | `lists`, `list_books`, `followed_lists`; `insert_list`, `update_list`, `delete_list`, `insert_list_book`, `delete_list_book`, `update_list_books`, `upsert_followed_list`, `delete_followed_list` | Query `read:lists`; mutations `write:lists`.[6][8] | High | Official examples show create/add/update/follow calls and list-book `position`; users can modify only their own lists, and deletion is irreversible.[6] |
| 2 | **Browse and answer community prompts** | `prompts`, `prompts_by_pk`, `prompt_answers`, `prompt_books_summary`; `insert_prompt`, `update_prompt`, `insert_prompt_answer`, `update_prompt_answers`, `delete_prompt_answer`, follow/unfollow roots | Query `read:prompts`; mutation `write:prompts`.[8][11] | High for discovery/read; medium for authoring | Prompt fields and answer/book relationships are documented, but the prompt schema page does not publish concrete mutation payloads. Validate input/ownership rules first. |
| 3 | **Recommendation feeds and custom “vibes”** | `vibes`, `vibes_by_pk` plus `books`; `insert_vibe`, `update_vibe`, `delete_vibe` | Query `read:vibes`; mutation `write:vibes`.[8][12] | High | Results are ordered `cached_book_ids` and require a second `books` query. Only custom vibes are fully editable; creation and signal/dynamic-signal changes require Supporter status. Signal inputs are write-only, and cached results may be stale until regeneration.[12] |
| 3 | **Public-library/list discovery and catalog enrichment** | `books`, `books_by_pk`, `authors`, `search`, `lists`, `list_books` | Query; `read:catalog`/`read:catalog:search` and `read:lists`.[6][8] | High | Use exact/browse-based list discovery: `_ilike` and related text operators are disabled, and search has a two-second maximum timeout.[4][6] |

## Documented data but defer as UI-only implementation work

| Website-visible concept | What is actually verified | Why not commit it yet |
| --- | --- | --- |
| **Discover / “Because you read…” rows** | Vibes documentation identifies system-managed recommendation, dynamic, and top-picks vibe types, but only custom vibes are generally editable and result refresh is asynchronous/cached.[12] | Shelfcheck can display a selected vibe’s results, but should not promise parity with Hardcover’s entire Discover UI or its system-managed ranking behavior. |
| **Social activity feed, likes, follows** | `activities`, `likes`, and follow-related roots and scopes are documented.[8] | API availability is verified, but a Shelfcheck social UX, filtering rules, privacy expectations, and write semantics are not yet specified. Treat as a later design decision, not an inferred integration. |
| **Notifications and account/import management** | Notification, account, and collection-import root fields are listed in the action/scope reference.[8] | No Shelfcheck use case or payload-level research was established here; do not expose these merely because the web app has them. |

## Cross-cutting implementation notes

- **Use least privilege.** OAuth discovery lists granular `read:*`/`write:*` scopes, and the actions reference maps every listed operation to a scope.[3][8]
- **Plan for change and quotas.** Hardcover calls the API beta, warns it can break/reset tokens, sets a 30-second query maximum, and documents Free limits of 5,000 daily requests, 10 burst, and 60/minute.[4]
- **Keep user data user-scoped.** The API documentation says commercial/public products may access user-owned library, journal, prompt, list, vibe, goal, network, and activity data only on behalf of a user who allowed access.[4]
- **No anonymous schema assumption.** An unauthenticated GraphQL introspection request returned HTTP 400 `No Authorization header` during this research.[unverified] Use a scoped token and the documented explorer/schema for integration-time validation.[1]

## Recommended sequence

1. OAuth connection + read-only shelf/status/progress dashboard.[1][5][9]
2. Opt-in shelf status/rating/review and progress writes, each with post-write verification.[5][8]
3. Lists and goals, then journals.[6][7][10]
4. Prompts and vibes after product/privacy design; keep social/notification surfaces deferred.[8][11][12]

## Sources

[1] https://docs.hardcover.app/api/oauth/getting-started-server
[2] https://docs.hardcover.app/api/oauth/getting-started-device
[3] https://api.hardcover.app/.well-known/oauth-authorization-server
[4] https://docs.hardcover.app/api/getting-started
[5] https://docs.hardcover.app/api/graphql/schemas/userbooks
[6] https://docs.hardcover.app/api/graphql/schemas/lists
[7] https://docs.hardcover.app/api/graphql/schemas/goals
[8] https://docs.hardcover.app/api/graphql/actions
[9] https://docs.hardcover.app/api/guides/gettingbooksprogress
[10] https://docs.hardcover.app/api/graphql/schemas/readingjournals
[11] https://docs.hardcover.app/api/graphql/schemas/prompts
[12] https://docs.hardcover.app/api/graphql/schemas/vibes
