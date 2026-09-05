# GitMake v1.2.9 — The Secret Scan, Measured

v1.2.8 took the publish pipeline apart. This release profiles what it spends its time on, and finds that one stage was responsible for nearly all of it.

Publishing behaviour is unchanged. No config, plan, approval, project-identity or MCP interface moves, and the secret scan produces byte-identical reports to v1.2.8 — that is verified against v1.2.8 directly, not assumed.

## The security gate ran at 2 MB/s

The secret scan reads every byte of everything being published, so its throughput is the floor on how fast a publish can be. It was measured at roughly 2 MB/s. A tree of two thousand ordinary source files — thirty megabytes — took **17.3 seconds** and allocated **2.17 GB**.

Timing the rules one at a time made the cause obvious:

```
private_key        (-----BEGIN)      0.0ms   thousands of MB/s
slack_webhook      (https://hooks)   0.7ms   5712 MB/s
github_token       (\bghp_...)      57.4ms     69 MB/s
connection_string  (\b[a-z]*://)   122.2ms     32 MB/s
```

A leading `\b` stops Go's `regexp` from extracting a required literal prefix, so it falls back to running the full engine over every byte. Fourteen of the nineteen content rules begin with `\b`. The 800x gap between the two groups was an artefact of how the patterns happened to be written, not of what they match.

Separately, a one-megabyte scan buffer was allocated **per file** regardless of the file's size, so scanning a two-hundred-byte file cost a megabyte of allocation and a megabyte of first-touch page faults. That is where the 2.17 GB went.

## What changed

Each rule now declares the literals that every match of it must contain, and a `bytes.Contains` — memchr-accelerated, GB/s — decides whether the regex is worth running at all. The regex still returns every verdict; the literal only gates it. Ordinary source contains none of these strings, so the common case rejects each rule outright.

The scan window is allocated once per worker instead of once per file.

Content scanning runs across up to eight workers. The directory walk stays sequential: it is cheap, and keeping it in one goroutine is what keeps its errors ordered. Work is handed out one file at a time rather than in equal ranges, because file sizes in a source tree differ by orders of magnitude and a fixed range would leave one worker running long after the others had finished.

Measured on the same trees, warmed so neither side pays for a cold cache:

| tree | v1.2.8 | v1.2.9 |
| --- | --- | --- |
| 100 files × 4 KiB | 118.7 ms | **3.4 ms** |
| 500 files × 16 KiB | 3,958 ms | **14.2 ms** |
| 2000 files × 16 KiB | 17,271 ms | **175 ms** |

Allocation for the last of those: 2.17 GB to 11.5 MB.

Eight workers is where the measured return stopped being worth the memory, and the number came from the curve rather than from a guess. The two shapes of tree have different bottlenecks: two thousand small files are bound by per-file open and read and stop improving after two workers, while thirty-two one-megabyte files are bound by rule matching and scale nearly linearly, from 268 MB/s to 1631 MB/s at eight. Sixteen workers reach 2154 MB/s on the second shape, change nothing on the first, and double the memory.

## Making a faster security gate a safe one

Optimising the security gate is the most dangerous place in GitMake to optimise anything. A literal that is too narrow does not fail loudly; it stops detecting a credential, and every existing test stays green while the scanner goes blind. Parallelism fails the same way: a gate whose findings depend on which worker finished first is worse than a slow one.

So the speed work is held down by tests that would catch exactly those failures:

- Every rule runs gated and ungated over samples covering **every branch of every alternation** — all five GitHub token prefixes, both AWS prefixes, all five Slack ones, both Stripe ones, every `-----BEGIN` variant, Discord's canary and ptb hosts, OpenAI's three account prefixes — and the two must return the identical offset. The samples are proven to be real positives first, so the agreement cannot hold trivially.
- A rule that declares no literals runs its regex unchanged. The fallback is deliberately the slow-but-correct direction: a rule added later can cost speed, but it cannot be silently skipped.
- The parallel scan is compared against the sequential scan of the same tree at two, three, four, eight and sixteen workers, twenty runs each, on a fixture with nineteen kinds of secret across sixty files — several with two kinds in one file, some three thousand lines deep, plus binary files, a secret-by-name `.env`, and a large file. The reports must be deeply equal, order included.
- Findings are assembled positionally rather than in completion order, so a worker's timing cannot reach the report. Sorting is stable, so equal keys keep a fixed order instead of depending on the sort's internal choices.

That equivalence test was verified to fail. Collecting results in completion order — the classic version of this bug — makes the scanner report secrets against **the wrong files**, and the test catches it on the first run.

Finally, the whole scanner was compared against v1.2.8 end to end. A 125-file fixture covering all nineteen rules, binary files, `.env`, a documentation placeholder that must not block, a large file, an empty file and an empty directory produced 97 findings on both versions and **byte-identical** report JSON.

## Verification

- `go build`, `go vet`, `go test` — and `go test` again, to prove the suite is repeatable
- Full CI on Linux, Windows and macOS: 8 of 8 green, including both race jobs and the packaging job
- Windows E2E: `e2e_v05`, `e2e_v051`, `e2e_v052`, `e2e_v121`, `e2e_v123`, `e2e_v124`, `e2e_v126`
- `internal/securityscan` coverage: 83.1% to 87.3%

The race detector could not be run on the development machine, which has no C compiler. The CI race jobs on Linux and macOS are what cover it, and both pass.

## Upgrading

`gitmake upgrade` installs this release from v1.2.6 or later.

From v1.2.5 or earlier, self-upgrade cannot move you — staged replacement never ran in those builds. Extract the platform package and install once:

```powershell
.\gitmake.exe install
```

```bash
./gitmake install    # Linux / macOS
```
