# Style Guide

This guide sets how the site copy reads. It adapts the [Axibase technical writing guide](https://axibase.com/use-cases/workshop/technical-writing.html) for a self-paced course, and adds one rule of its own: state facts concretely, without hedging.

The prose lint in `tests/build/prose.test.mjs` enforces the rules marked **Lint** below on every page under `src/content/docs`, except the `dev/` fixtures. It runs with `npm run test:build` and does not need a build. Run it alone with `node --test tests/build/prose.test.mjs`. The other rules rely on review.

## Concrete Prose

State what happens. Every sentence names an actor, an action and, where the result varies, the condition that decides it. **Lint**

Where real behavior varies, keep the variation and name its condition: an operating system, a version, a setting, a number.

| Bad | Good |
| --- | --- |
| DNS answers may be cached. | On macOS, the operating system caches DNS answers until their TTL runs out. |
| A lookup that hits the cache often costs less than a millisecond. | When your operating system caches DNS, as macOS does, a repeat lookup costs a millisecond or less. |
| TLS usually takes about one round trip. | TLS 1.3 takes one round trip. TLS 1.2 takes two. |
| Let it run for a while. | Let it run against three sites for 30 minutes. |
| The error may be caused by the certificate. | The certificate is expired, issued for another name or not trusted, or the clock on your computer is wrong. |

Concrete does not mean one cause. When several causes produce the same symptom, list each one by name. A list of named causes is the concrete alternative to may be caused by.

Concrete also does not mean stiff. Write the way you would explain it to a colleague at a whiteboard: the rules remove hedges, not natural phrasing. Join cause and effect with because, to, a colon or two sentences, not with therefore.

These words and phrases signal a hedge. Rewrite the sentence to remove them, with a number, a condition or an example:

| Hedge | Hedge | Hedge | Hedge |
| --- | --- | --- | --- |
| may | might | perhaps | maybe |
| possibly | probably | likely | could |
| would | should | generally | usually |
| typically | often | somewhat | fairly |
| quite | rather | arguably | tends to |
| in most cases | it depends | kind of | sort of |
| seems | appears to | sometimes | almost |
| mostly | normally | commonly | frequently |
| occasionally | potentially | ideally | presumably |
| hopefully | in general | | |

Rather than is a comparison, not a hedge, and the lint allows it.

## Voice and Tense

- Use active voice, with a named actor or the imperative. Bad: The certificate is checked. Good: Your side checks the certificate.
- Use present tense. Avoid will, was, were and had. **Lint** Bad: The lookup will fail. Good: The lookup fails.
- Address the reader as you, and give instructions in the imperative. See [Site Adaptations](#site-adaptations).
- Describe a function in present tense without a subject. Bad: This function returns a copy of `t`. Good: `result`: returns a copy of `t`.
- Remove filler such as follow these steps, and the words lets and please. **Lint**

## Word List

**Lint** checks every row. Matching ignores case and stops at word boundaries: should does not match shoulder.

| Avoid | Write |
| --- | --- |
| should | must, or remove |
| could, would | a sentence that states what happens |
| may | can |
| will, was, were, had | present tense |
| abort, kill, terminate | stop, cancel. For a Unix signal, name it: `SIGTERM` asks a process to exit, `SIGKILL` ends it. Do not write stop there, because stopping a process in Unix means `SIGSTOP`, which pauses it. |
| so, a lot | because, to, a colon, or two sentences; many |
| flag | option or setting, except a Go command-line flag, a feature flag or a TCP flag such as the SYN flag |
| lets, please | remove |
| regex | regular expression |
| and/or | and, or, or one of |
| in order to | to |
| make sure | ensure |
| end-point | endpoint |
| click on | click |
| robust | what it withstands, with a number |
| sample | example |
| admin | administrator |
| deselect, uncheck | clear |
| ingest | load, import |
| Epoch time | Unix time |
| datacenter | data center |
| e.g., i.e. | for example |

The lint allows these exact forms:

- May after a lowercase word or before a number: the month, as in in May or May 5. Will after a lowercase word: a name.
- so far and if so.
- rather than.
- Let's Encrypt, the name of a certificate authority.
- flag after Go, command-line, feature, SYN, ACK, FIN, RST or TCP.

Some banned words are the correct technical term in one context: kill or terminate for a Unix process, sample for a metric data point, abort for a Postgres transaction. Name the precise term where one exists, such as `SIGKILL`. Otherwise, add a suppression comment on the line before or on the same line:

```mdx
{/* prose-allow: sample */}
Prometheus stores one sample per scrape.
```

The comment names one or more words, separated by commas, and covers its own line and the next line only.

## Grammar

| Rule | Bad | Good |
| --- | --- | --- |
| Spell out an acronym once, on first use, then abbreviate it. | The CA signs it. | A certificate authority (CA) signs it. |
| Never spell out SQL, API, REST, URL, URI, CSV, JSON, XML, PDF, HTTP, HTTPS, SSL, SSH, ISO, ASCII or UTC. See [Site Adaptations](#site-adaptations) for DNS, TCP and UDP. | HTTP (Hypertext Transfer Protocol) | HTTP |
| Use Title Case for headings. Small words stay lowercase unless first or last: a, an, the, and, but, or, for, nor, on, at, to, by, of, in, with, from, as, vs. **Lint** | Done when | Done When |
| Keep program, file and function names lowercase, even in headings. **Lint** | Trace With Dig | Trace with dig |
| After a colon, a heading starts a new segment, and its first word is capitalized. **Lint** | DNS: from a Name | DNS: From a Name |
| Mixed-case names keep their case in headings. **Lint** | Call GRPC From an IPhone | Call gRPC from an iPhone |
| Do not capitalize the first word after a colon in a list item. | `Certificate`: The server sends it. | `Certificate`: the server sends it. |
| Keep file extensions lowercase and product names in their original case. | `.GO`, MacOS, Github | `.go`, macOS, GitHub |
| Pick one form of each term: Unix, bash, IPv4, IPv6, URL, Go, macOS. Use US spelling. | ipv4, golang, organisation | IPv4, Go, organization |
| Write every contraction out in full. **Lint** | It isn't cached. | It is not cached. |
| Do not join clauses with different actors with a comma. Split them. | The request goes out, the server replies. | The request goes out. The server replies. |
| Begin a list with a colon. Put a colon between a title and its meaning, not a hyphen. | `req.Close` - opens a connection | `req.Close`: opens a connection |
| Hyphenate compound adjectives, but not adverbs ending in -ly. | a three way handshake, a newly-built page | a three-way handshake, a newly built page |
| Spell out numbers one through ten and all ordinals, unless the number has a unit. | 3 failures, 2nd block | three failures, second block |
| Use thousands separators, except for milliseconds and machine output. | 10000 checks, 1,000 ms | 10,000 checks, 1000 ms |
| Put a space before a multi-letter unit, ms included. Write a one-letter unit without a space. | 8GB, 12ms, 10 % | 8 GB, 12 ms, 10% |
| Avoid parentheses, except to spell out an acronym, a character or a number. | (Week 5 covers goroutines.) | Week 5 covers goroutines. |
| Avoid possessives. Prefer a compound noun, such as the resolver cache, over an of-phrase. Use of only when the compound reads badly. | the server's certificate, the certificate of the server | the server certificate |
| Do not use quotation marks to set off a term. Bold the term where it is defined. | a "round trip" | a **round trip** |

## Formatting

- Use backticks for file names, paths, programs, commands, hosts, IP addresses, HTTP methods, status codes, headers, function names, parameters, values, ports and versions in prose. Bad: run curl against example.com on port 443. Good: run `curl` against `example.com` on port `443`.
- Do not put a plain acronym in backticks, and never use backticks in a heading. **Lint** Bad: `DNS`, ## Meet `httptrace`. Good: DNS, ## Step 1: Meet httptrace.
- Single-line machine output goes in backticks. Multi-line output goes in a fenced block with the `txt` language.
- Give every code fence its correct language: `go`, `sh`, `txt`.
- Bold UI elements. Bad: tick Done When. Good: tick **Done When**.
- Write dates with a spelled-out month, no ordinal suffix, a comma after the year, and 24-hour time. Bad: 10/10/26 2:02 PM, October 10th. Good: October 10, 2026, at 14:02.
- Describe what a command does instead of announcing it. Bad: The following command lists the files. Good: List the files:
- A heading has no trailing period and no punctuation other than colons and the commas of a list that ends in and or or. Number steps as Step 1:, not 1. **Lint**
- Link text describes the target. Never write here or click here. **Lint** Bad: read more [here](#). Good: read the [Time and NTP track](#).

## Example Values

| Kind | Use |
| --- | --- |
| IPv4 addresses | `192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24` |
| IPv6 addresses | `2001:db8::/32` |
| Domains | `example.com`, `example.org`, or any name under `.example` for a name that must not exist |

Real names and addresses are allowed where the exercise needs that exact service: public resolvers such as `1.1.1.1`, loopback `127.0.0.1`, and test sites such as `expired.badssl.com`.

## Site Adaptations

| Adaptation | Reason |
| --- | --- |
| Address the reader as you, and use the imperative. | The site is a course. The reader acts on every page, and the imperative names the actor. |
| Explain every term on first use, on the page that uses it. | The reader knows TypeScript, React and testing, but is new to Go, Linux, networking and infrastructure. |
| Spell out DNS, TCP, UDP and IP on the page that teaches them. Axibase never spells out DNS, TCP or UDP. | The reader meets these terms for the first time, and the expansion is part of the explanation. The rest of the never-spell-out list stands, because a frontend engineer knows HTTP, URL, JSON and API. |
| Quoted machine output inside code blocks is exempt from every prose rule. | Output must match what the reader sees in the terminal. The lint skips code blocks and inline code. |
| Allow flag for a Go command-line flag, a feature flag and a TCP flag. | Each is the standard term: the Go `flag` package, feature flag tooling, and the TCP header. |
| Allow kill, terminate, sample and abort through a `prose-allow` comment where they are the technical term. | Unix signals, Prometheus samples and Postgres aborted transactions have no other name. |
| Keep week numbers, step numbers, versions and ports as digits. | They are names, not counts: Week 4, Step 3, TLS 1.3, port `443`. |
| Allow commas in a heading that lists items and ends in and or or. | A list heading such as Go Packages, Interfaces and Testing has no clear rewrite without them. |
| Write the time range in the week header as 6–8 hours, not 6–8 h. | The unit rule gives 6–8h, which reads as a typo. The full word avoids the question. |
| Use US spelling. | Axibase writes US English. One spelling keeps search and the lint simple. |
| Leave out the Axibase sections on Interface Elements, Issue Subjects, Product Names, Variables and Configuration. | They apply only to Axibase products and its issue tracker. |

## What the Lint Checks

| Check | Where |
| --- | --- |
| Hedges and the word list | prose, callout text, frontmatter `title`, `description` and `done[].text`, including multi-line values, and the visible `title`, `caption`, `text` and `description` props of components |
| Contractions, including it's, that's, you're, let's and how's | same as above |
| e.g., i.e. and click here | same as above |
| here as the whole text of a link | prose |
| Title Case, backticks and punctuation in headings | markdown headings, the frontmatter `title`, and the week, phase and track titles in `src/data` |

The lint hides frontmatter values that are not prose, fenced code blocks, inline code, import lines, URLs, and component tags with their props. It keeps the children of components, which means callout text is checked. A `prose-allow` comment suppresses named words, as described under [Word List](#word-list). To let a program name stay lowercase in a heading, add it to `CODE_WORDS` in `tests/build/prose.test.mjs`.

The lint cannot judge passive voice, possessives, comma splices, parentheses, number style, backticks around hosts and values, or whether a term is explained on first use. Review those by hand.
