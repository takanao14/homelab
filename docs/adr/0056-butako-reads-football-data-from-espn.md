# ADR-0056: Butako reads football data from ESPN's unofficial API

- **Status:** Accepted
- **Date:** 2026-09-25
- **Related:** [ADR-0045](0045-deploy-slack-bot-from-its-own-repository.md),
  [ADR-0055](0055-voice-ui-observes-gpu-switch-without-controlling-it.md)

## Context

Butako should answer questions about Arsenal and Manchester United matches:
recent results, the next fixture, scorers, and league position. An earlier
general web search design (SearXNG through an MCP server) was withdrawn after
Gemma could not produce grounded answers from search results. That design also
sent search terms derived from what the user said to external search engines.

Candidate data sources were compared with live requests on 2026-09-25:

- API-Football's free plan returned no data for the 2026 season ("Free plans do
  not have access to this season, try from 2022 to 2024").
- The Fantasy Premier League API has no key and includes scorers, but covers
  only the Premier League. It omits Arsenal's Champions League and League Cup
  matches, so "the last match" and "the next match" would be wrong.
- football-data.org's free plan is documented but excludes goal scorers.
- ESPN's site API needs no key and returned current Premier League results and
  fixtures, standings, match details with goal minutes and line-ups, and
  Champions League and League Cup matches. It is undocumented and may change
  without notice.

## Decision

The Butako app reads football data from ESPN's site API. A background fetcher
inside the app requests fixed URLs for the configured teams and competitions,
keeps the latest data in memory, and refreshes it every 30 to 60 minutes.
Requests contain only competition slugs, team IDs, and event IDs; no user
speech, transcript, or prompt is sent. The fetcher runs independently of
conversations, so a conversation never waits on ESPN.

For each conversation the app adds a short factual summary to the system
prompt. The app itself converts times to JST, decides win, draw, or loss from
each team's side, and compares league positions, so Gemma does not compute
them. If data is missing, older than the refresh window allows, or does not
parse, the summary states that match information is unavailable and Gemma is
told not to guess.

The data is public, held only in memory, and not written to logs or storage.
No API key or secret is needed.

## Consequences

- Butako can answer match questions across the Premier League, Champions
  League, League Cup, and FA Cup without an external search engine or LLM tool
  calling.
- An ESPN format change or block makes match answers unavailable until the
  fetcher is fixed; conversation otherwise continues. The fetcher is isolated
  behind an interface so another source can replace it.
- Using an undocumented API carries no service guarantee. Request volume is
  kept low (tens of requests per hour at most) and the source is revisited if
  ESPN restricts it.
- Live scores, line-ups, and questions about other clubs are out of scope for
  the first stage and would be added through tool calling after Gemma's tool
  use is evaluated.
