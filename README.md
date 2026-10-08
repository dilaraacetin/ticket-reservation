# SeatHold

A seat reservation service in Go. Seats are claimed through a short-lived **hold**
rather than sold outright: a hold is created, it expires on its own, and only a
confirmation turns it into a reservation. Everything else here follows from that
one decision — the expiry sweeper, the locking, the waiting list, the live
updates.

Around that sits a catalogue people actually browse: events carry a city, a
category, a poster and house rules, and the list filters on the first two.

It is a learning project, built stage by stage, and the interesting parts are the
trade-offs rather than the feature list. Those are written down below.

## Running it

Nothing to install beyond Go. The server, the API documentation and the browser
interface are a single binary.

```bash
make run          # in-memory stores, no database, http://localhost:8080
```

With Postgres, the API documentation at `/docs`, and the whole thing in
containers:

```bash
echo "AUTH_SECRET=$(openssl rand -base64 32)" >> .env
make up           # postgres, then migrations, then the server
make logs
make down
```

`.env` is read by `make` and by compose, and is git-ignored. Every setting it may
hold is in [Configuration](#configuration).

`make up` refuses to start without `AUTH_SECRET`. That is deliberate; see
[Decisions](#decisions-worth-knowing).

## Trying it

```bash
# an account
curl -X POST localhost:8080/auth/register -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"a-strong-passphrase"}'

TOKEN=$(curl -s -X POST localhost:8080/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"a-strong-passphrase"}' | jq -r .token)

# hold a seat, then confirm it
curl -X POST localhost:8080/events/event-1/seats/A1/hold -H "Authorization: Bearer $TOKEN"
curl -X POST localhost:8080/holds/<holdId>/confirm       -H "Authorization: Bearer $TOKEN"
```

Or open `http://localhost:8080` for the browser interface, and
`http://localhost:8080/docs` for the API reference. `make token USER=user-1`
signs a token without creating an account, for poking at the API by hand.

### Endpoints

| | |
|---|---|
| `POST /auth/register`, `POST /auth/login`, `POST /auth/logout` | accounts |
| `POST /auth/verify`, `POST /auth/verify/resend` | confirming an email address |
| `GET /events`, `GET /events/{id}`, `GET /events/{id}/seats` | the catalogue, one event, its seat map |
| `POST /events/{id}/seats/{seat}/hold` | claim a seat |
| `POST /holds/{id}/confirm`, `DELETE /holds/{id}` | confirm or give up |
| `GET /events/{id}/waiting-list` + `POST`/`DELETE` | queue for a seat |
| `GET /events/{id}/stream` | server-sent events |
| `GET /account` | who the caller is, and what to show them |
| `GET /tickets`, `GET /tickets/{code}/qr.png` | tickets, and the code to scan |
| `GET /notifications`, `POST /notifications/read` | what the caller has not been told yet |
| `GET /push/key`, `POST`/`DELETE /push/subscriptions` | letting a browser be pushed to |
| `POST`/`PATCH`/`DELETE /admin/events[/{id}]` | stocking and withdrawing, administrators only |
| `POST /admin/events/{id}/cancel`, `POST /admin/events/{id}/seats` | |
| `GET /health`, `GET /ready` | liveness and readiness |
| `GET /metrics`, `GET /docs`, `GET /openapi.yaml` | operations |

## How it is put together

```
handler  →  service  →  repository  →  domain
```

Dependencies point one way. The domain knows nothing about storage, the service
knows nothing about HTTP, and interfaces are declared by whoever consumes them
rather than by whoever implements them — so `repository.SeatRepository` lives
next to the service that needs it, and the handler declares its own narrow view
of the service.

The payoff was measured rather than assumed: swapping the in-memory store for
Postgres changed **zero lines** in the service package.

| package | what it owns |
|---|---|
| `internal/domain` | the rules: when a seat may be held, confirmed, released, expired |
| `internal/repository` | storage, with in-memory and two Postgres implementations |
| `internal/service` | the flow, the expiry sweeper, the waiting-list handoff |
| `internal/handler` | HTTP, JSON shapes, the error-to-status table, middleware |
| `internal/auth` | token signing, password hashing |
| `internal/event` | the notice broker behind the live stream |
| `internal/metrics` | the Prometheus registry |
| `internal/config`, `internal/web` | settings, and the embedded browser interface |

## Decisions worth knowing

**A hold is guarded as one operation, not three.** `UpdateSeat` takes a
`mutate func(*domain.Seat) error` closure and runs it while the row is locked, so
checking whether a seat is free and taking it cannot be split apart by another
caller. In Postgres that closure maps onto `BEGIN; SELECT … FOR UPDATE; UPDATE;
COMMIT`. Holding a lock inside each repository method instead would leave the
check-then-act race wide open — and the race detector stays silent about it,
which is the trap.

**Categories are a closed set; cities are not.** A category is validated against
seven values in the domain and by a `check` constraint in the schema, for the
reason a filter exists at all: one venue typing "Stand-up" and another "stand up"
would split one category into two that each find half the events, and nothing
would say so. Cities stay free text because there is no list to close them
against, which is also why the two are offered differently — categories as a row
of pills, cities as a select, since a catalogue grows to more cities than a thumb
can scroll past.

**A PATCH that leaves a field out keeps it; one that sends it empty clears it.**
The descriptive fields of an event are `*string` in the request body, so those
two cases are distinguishable. With plain strings, editing only the start time
would either erase the description or make a description impossible to clear.
The merge runs inside the same locked read-decide-write as everything else, so
nothing is read in one step and written in another.

**Posters cost a hole in the content security policy, and it is a deliberate
one.** `img-src` is `'self' data: https:` rather than `'self'`, because posters
live wherever the venue keeps them. The cost is real and bounded: the URL is set
by an administrator, and a remote image lets that host see the IP of everyone who
browses the catalogue. Everything else in the policy stays shut, including
`script-src 'self'` with no `unsafe-inline`.

**Expiry is defined by the clock, not by the sweeper.** `Seat.CanHold` treats an
expired hold as gone whether or not the sweeper has noticed. The sweeper exists
only so seat maps do not lie.

**Both locking strategies are implemented, and the choice is measured.** From
`go test -bench BenchmarkUpdateSeat`, six runs, time per *successful* update:

| | one seat, every caller | twenty seats |
|---|---|---|
| pessimistic (`FOR UPDATE`) | **392 µs**, 0 turned away | 196 µs |
| optimistic (version + retry) | 526 µs, **36% turned away** | **124 µs** |

Optimistic locking wins when callers rarely collide and loses badly when they
always do — and its lower cost per *attempt* is misleading, because giving up is
cheap. Counting only `ns/op` is how you reach the wrong conclusion here.

**`AUTH_SECRET` has no fallback.** The process refuses to start without one and
says how to generate one. A default compiled into the binary is a default
everyone with the source already has, and a deployment that silently starts on it
is one where anyone can forge a token for any user. The development secret lives
in the `Makefile`, which never ships.

**The waiting list hands out an ordinary hold.** When a seat comes free — from a
release or from a hold running out — whoever has waited longest is given a normal
hold on it and told over the stream. Normal, because a turn that is not taken
expires like any other hold and the seat moves on. Postgres takes the front of
the queue with `FOR UPDATE SKIP LOCKED`, so two workers handling two freed seats
get two different people instead of one blocking behind the other.

**The browser reads the stream with `fetch`, not `EventSource`.** `EventSource`
cannot send an `Authorization` header, and the notice saying your turn came is
addressed to one person. A page built on `EventSource` hears every public notice
and never the one meant for it.

**Metrics are labelled by matched route, never by path.** One time series per
hold id is how a metrics store is brought down by its own data.

**The interface is one binary's worth of files, not a build.** Four ES modules
and a stylesheet, embedded with `go:embed` and served from the same origin as the
API. No bundler, no `node_modules`, no second deployment — and because it is
same-origin there is no CORS to get wrong. The cost is that every screen is
written by hand; the gain is that `make run` puts the whole product on screen two
seconds later.

A path with no file behind it is served the page rather than a 404, because the
interface routes itself and a confirmation link lands on `/verify`. Anything with
an extension still 404s: a typo in a script tag should say so rather than quietly
loading HTML as a script.

**The ticket code is minted at confirmation.** It is not derived from the seat or
the account — somebody who knows a row and a number must not be able to work out
what will be scanned at the door. The QR is rendered by the server: encoding one
is error correction and bit placement, and a page that gets it subtly wrong makes
a code that scans everywhere except at the gate.

**Taking a seat needs a confirmed address.** Holding, confirming and queueing
all go through one rule, asked in the service rather than in a middleware —
because the waiting-list handoff calls the service directly and a check that
only existed over HTTP would not apply to it.

That same handoff is why queueing is guarded too, and the reason is not
obvious: a waiter who can never be given a seat would reach the front of the
queue, be refused, be put back, and block everybody behind them for good. Both
halves are needed. The handoff also drops such a waiter rather than returning
them, for the cases where one is already in a queue.

Confirming is checked as well as holding, so a hold made before the rule applied
does not still turn into a sale. The cost is one lookup by primary key on each
of those paths.

Reading stays open: an unconfirmed account can sign in and browse the seat map.

**Nothing is emailed to an address nobody has confirmed.** Anybody can register
with somebody else's address, and sending to it would make this service a way to
post mail to strangers. Until a link sent to the address has been followed, an
email delivery is recorded as given up rather than retried — waiting will not
change it, and five failed attempts per unverified account is noise rather than
information.

The confirmation link is the one message that has to go to an unconfirmed
address, so it has its own small queue rather than the notification outbox. The
token is also kept out of the notifications table, where the notifications
endpoint would have handed it over.

Two things about that token are worth stating. It is stored as it was sent, not
hashed: hashing would stop a read-only leak from yielding a working link, but it
would also make the token unrecoverable at send time and so force the sending
into the request that registered the account. What keeps the exposure small
instead is that the row is **deleted** the moment the link is followed — that
deletion is what makes it single use — and swept once it expires, so the table
holds only live, unused tokens for at most a day. And verifying checks the
account and the address together: a link sent to an address the account has since
moved away from proves nothing about the new one.

It is a `POST`, not a `GET` on the link itself, because a mailbox scanner or a
link preview would spend a single-use token before the person ever clicked it.

**Sending happens in a worker, not in the request.** Reaching a mail server
inside the request that cancelled an event would make the cancellation as slow
and as unreliable as that server. So notices go into an outbox and a worker
drains it, backing off after each failure and giving up after five attempts.

Two consequences are worth stating. Claiming a delivery takes a short lease
(`FOR UPDATE SKIP LOCKED` plus a pushed-out due time), so two workers never send
the same message — but a worker that dies after sending and before recording it
will send again when the lease runs out. For a notification, arriving twice is
the better of the two mistakes. And the channels are independent: a push that
cannot be delivered does not stop the email, which is visible in the outbox as
one row sent and one still trying.

**Email is SMTP through the standard library.** Every provider speaks it, so the
same code works against Mailpit on a laptop and a real service in a deployment,
and the project gains no dependency for it. Header values are both replaced and
Q-encoded; measured, either one alone stops a newline in a subject from becoming
a `Bcc`, and both are kept because stopping it should not rest on a side effect
of something done for another reason.

**Push is Web Push with VAPID.** The encryption is a library's job: the payload
is sealed to each browser's own keys, and hand-rolling that is how a message ends
up readable by the service carrying it. One subscription per browser, not per
person. A push service that says an endpoint is gone is believed and the
subscription forgotten. Unsubscribing checks ownership, because an endpoint is
unguessable but it is not a secret.

**A notice is stored, not only pushed.** Withdrawing an event tells everyone
with a stake in it — whoever holds a seat, whoever owns one, and whoever is
still queued for one — and the telling is kept until they have read it. A notice
that only reaches people who happen to have the page open is not a notification:
the point of being told is that you were not looking. Each notice carries the
event's name, because the event may be gone by the time anyone reads it.

The queue goes with the event, since a queue for something that is not happening
keeps people waiting for a turn that cannot come. If the telling fails it is
logged rather than returned: the event is already withdrawn by then, and
reporting an error would say the cancellation did not work when it did.

**An event is withdrawn, not deleted.** Cancelling marks it and leaves it in the
catalogue, because somebody holding a ticket to it has to be able to see what
happened. Deleting is kept for mistakes: it is refused outright while any seat is
held or sold, in one statement so a seat that stops being available in between
cannot slip through.

Cancelling stops new claims but **not** confirmations. Somebody already holding a
seat was part way through a purchase when the event was pulled, and letting them
finish is the kinder answer. The cost is one extra read on the busiest path:
holding a seat now looks the event up first, which is what makes a withdrawn
event stop selling.

**Roles are read from storage, not from the token.** An administrative request
looks the account up and checks its role every time. Putting the role in the
token would save a query and outlive being taken away: a token is signed once
and believed until it expires. These routes are rare, so the lookup costs
nothing that matters, and taking someone's role away takes effect at once.

**There is no endpoint that makes an administrator.** The first one is made with
the `admin` command, which needs database access:

```bash
make promote EMAIL=you@example.com     # against the compose database
```

An endpoint that can create an administrator is an endpoint that can be talked
into creating one. Whoever already has the database is already trusted.

**Tokens are not JWTs.** They are HMAC-SHA256 signed, two parts rather than
three, deliberately hand-rolled to keep the signing story small and explicit.

**Signing out is a denylist, not a session store.** A token carries an id of its
own, and signing out records that one id until the token would have expired
anyway. The table holds the exceptions rather than the valid sessions, because a
token is already trusted by its signature — so it stays small and clears itself.
Each sign-in gets its own id, which is what makes signing out of one device
possible without ending the other.

Two consequences worth knowing. The check runs on every authenticated request,
so it is one lookup by primary key; and it **fails closed** — a check that cannot
run refuses, because the tokens on that list are the entire reason it exists. In
memory, the list is per process, so two servers without a shared database would
not see each other's sign-outs.

## Configuration

Everything is read from the environment at startup and validated there, so a bad
value is a refusal rather than a surprise later. `.env` is read by `make` and is
git-ignored.

| | default | |
|---|---|---|
| `AUTH_SECRET` | *required* | at least 32 bytes |
| `ADDR` | `:8080` | |
| `DATABASE_URL` | unset | unset means the in-memory stores |
| `HOLD_TTL` | `5m` | how long a seat stays claimed |
| `SWEEP_INTERVAL` | `30s` | how often expired holds are cleaned up |
| `TOKEN_TTL` | `1h` | |
| `IDEMPOTENCY_TTL` | `24h` | how long a retried request is recognised |
| `HANDOFF_WORKERS` / `HANDOFF_BUFFER` | `16` / `256` | waiting-list handoff; both set by load testing |
| `RATE_LIMIT_BURST` / `RATE_LIMIT_EVERY` | `30` / `1s` | per caller |
| `RATE_LIMIT_AUTH_BURST` / `RATE_LIMIT_AUTH_EVERY` | `5` / `6s` | sign-in is slower on purpose |
| `RATE_LIMIT_TTL` | `10m` | how long an idle allowance is remembered |
| `ARGON2_MEMORY_KIB` / `_ITERATIONS` / `_PARALLELISM` | `19456` / `2` / `1` | memory is the setting that matters |
| `REQUEST_BODY_LIMIT` | `8192` | bytes |
| `READ_HEADER_TIMEOUT` / `SHUTDOWN_TIMEOUT` | `10s` / `10s` | |
| `SEED_DEMO_DATA` | `false` | writes a demo event and its seats |
| `SMTP_ADDR` | unset | host:port. Unset means notices stay in the application |
| `SMTP_FROM` | `tickets@localhost` | required once `SMTP_ADDR` is set |
| `SMTP_USERNAME` / `SMTP_PASSWORD` | unset | left empty for a local catcher |
| `VAPID_PUBLIC_KEY` / `VAPID_PRIVATE_KEY` | unset | both or neither; generate with `go run ./cmd/vapid` |
| `VAPID_SUBJECT` | unset | required with the keys; a `mailto:` or `https:` URL |
| `PUBLIC_URL` | unset | required once `SMTP_ADDR` is set, so links in email are absolute |
| `VERIFICATION_TTL` | `24h` | how long a confirmation link works for |
| `PPROF_ADDR` | unset | see below |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

The startup log reports every setting with the secret and the database password
redacted, so "which settings is this running on" is answerable without reaching
for the deployment.

## Testing

```bash
make test     # go test -race ./...
make cover
make ci       # what CI runs: fmt-check, vet, lint, test
```

One contract suite runs against all three seat stores — in-memory, pessimistic
Postgres, optimistic Postgres — so an implementation that disagrees with the
others fails rather than drifting. The Postgres cases use testcontainers and skip
cleanly when Docker is not running.

A few guards are worth knowing about, because they fail for reasons that are not
obvious: the OpenAPI document is parsed and checked against the code, so an error
code added without being documented fails the build; the embedded migration
filesystem is checked, because `go:embed` keeps the directory prefix and goose
reads from the root.

Load testing lives in `cmd/loadtest`, in the repository rather than in a k6
script, because the interesting scenario is a sequence — hold a seat, then give
it up — run by many callers at once with their own tokens.

## Operating it

`GET /metrics` serves the Prometheus registry: requests by route and status,
failures by error code, stream subscribers, dropped notices, and the waiting-list
handoff. `api_failures_total{code="seat_not_available"}` is the contention
signal.

`PPROF_ADDR=127.0.0.1:6060` turns on the profiler, on its own server. It is off
by default and takes an address rather than a flag on purpose: pprof will hand
anyone who can reach it a heap dump, which is every secret the process is
holding.

`/health` and `/ready` answer different questions on purpose. `/health` does not
touch the database, because an orchestrator restarts the process over it and
restarting a working process during a database outage helps nobody. `/ready`
does, with a short deadline, because a load balancer takes an instance out of
rotation over it.

Every answer carries a strict `Content-Security-Policy`, `nosniff`,
`frame-ancestors 'none'` and `no-referrer`. There is no `Strict-Transport-Security`:
nothing here terminates TLS, and that header is a promise this deployment cannot
keep. `/docs` is the one exception to the policy, because it loads Swagger UI
from a CDN — its inline script is allowed by a per-request nonce rather than
`unsafe-inline`, and vendoring those assets is what would remove both the
exception and the third party.

`/metrics`, `/health` and `/ready` are open, like the rest of the public reads. A
real deployment would put them on an address the internet cannot reach.

## Not done yet

There is no payment step, and no way to sign out everywhere at once — only the
token in hand.

Nothing tells somebody mid-session that their address has just been confirmed —
the page finds out when it next asks.

The interface uses a system font stack rather than the Bricolage Grotesque and
Geist the design names. Loading them would mean either a third party on every
page or roughly 200 KB of font files in the binary, and neither is a decision to
make quietly. There is no CORS policy, which only starts
to matter once something other than the bundled page calls the API.
