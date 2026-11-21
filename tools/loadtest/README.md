# Load test

Simulates many people using Hiveloop at once and measures response times, errors and server
resources. It runs against a **test copy** of Hiveloop on your server, with its own empty
database: your real instance and its data are not touched.

Each simulated user downloads the app, signs in, stays connected to the live stream
(notifications and chat) and uses the calendar, chats, groups, documents and attendance with
3–10 second pauses. The AI assistant is left out (every request costs money).

## Parts

| File | Runs on | What it does |
|---|---|---|
| `main.go` | your computer | the user simulator |
| `relay/` | server | forwards traffic from a network address to the test copy (like a reverse proxy would) |
| `mon.sh` | server | records CPU, memory, swap, load and network every 2 s |
| `server.sh` | server | starts, summarises, stops and deletes everything |

## How to run it

From the project folder (replace `SERVER` with your server's SSH address and `ADDR` with an
address of the server your computer can reach, e.g. its LAN or VPN address):

```sh
# 1. build the simulator (your computer) and the relay (server; use GOARCH=amd64/arm64/386 as needed)
go build -o /tmp/loadtest ./tools/loadtest
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags=-s -o /tmp/relay ./tools/loadtest/relay

# 2. copy the tools to the server
ssh SERVER 'mkdir -p ~/hiveloop-load'
scp /tmp/relay tools/loadtest/mon.sh tools/loadtest/server.sh SERVER:hiveloop-load/

# 3. start the test copy (pick any password for its super admin)
ssh SERVER 'sh ~/hiveloop-load/server.sh start TEST-PASSWORD ADDR'
#    it prints the address to use, e.g. http://ADDR:18092

# 4. run the test: 50 users join within a minute and stay active for 5 minutes
/tmp/loadtest -url http://ADDR:18092 -users 50 -ramp 60s -dur 5m \
  -adminpw TEST-PASSWORD -probe https://your-hiveloop.example.org/api/version

# 5. server resources during the test
ssh SERVER 'sh ~/hiveloop-load/server.sh report'

# 6. stop everything and delete the test data
ssh SERVER 'sh ~/hiveloop-load/server.sh stop'
```

`server.sh start` expects the Hiveloop binary at `~/hiveloop/hiveloop` (edit the path at the
top of the script if yours is elsewhere). To run the test again without recreating users,
groups and events, add `-setup=false` (before `stop`).

## Simulator options

| Option | Default | Meaning |
|---|---|---|
| `-url` | `http://127.0.0.1:18091` | address of the test copy |
| `-users` | 50 | simulated users |
| `-ramp` | 60s | time to bring them all online |
| `-dur` | 5m | duration once everyone is online |
| `-adminpw` | | password of the test copy's super admin |
| `-probe` | | address of your real instance, checked every 5 s to see whether it slows down |
| `-setup` | true | create users, 5 groups and 40 events first |

## What to look at

- **Errors**: the `status` column should only show `200`. `-1` means no answer within 30 s.
- **Times**: `p50` is the typical time, `p95` the time 95% of requests stay under.
- **Server** (`report`): Hiveloop's CPU (100% = one full core), free memory and swap, which
  should stay at 0.

Reference (Intel Atom N2600, 1 GB RAM, 50 very active users): no errors, reads 21 ms typical
(68 ms at p95), Hiveloop at 38% of one core on average, 36 MB of RAM, no swap.
