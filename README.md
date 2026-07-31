# go-garmin

Go client for the **unofficial Garmin Connect API** (`connectapi.garmin.com`) —
the API used by the Garmin Connect mobile apps — authenticated with a regular
Garmin account (email / password / 2FA). **No Garmin Connect Developer Program
token required.**

It is a Go port of [python-garminconnect](https://github.com/cyberjunky/python-garminconnect):
the endpoint coverage mirrors it and the **token files are interchangeable**
between the two libraries.

> ⚠️ This is not a Garmin-supported API. Endpoints may change without notice,
> and Garmin aggressively rate-limits datacenter/cloud IPs (expect
> `ErrRateLimited` when calling from cloud providers — log in from a
> residential IP, or inject an `http.Client` going through a proxy).

## Scope

- **Authentication**: SSO login (mobile + portal strategy cascade), 2FA/MFA
  (blocking prompt for CLIs, or a serializable two-phase challenge for web
  apps), "DI" OAuth2 tokens with automatic refresh and refresh-token-rotation
  handling, pluggable `TokenStore` persistence.
- **API domains** (~140 methods, all on `connectapi.garmin.com`):

  | Domain | Service |
  |---|---|
  | Profile & settings | `client.UserProfile` |
  | Daily/weekly summaries, steps, fitness stats | `client.Summaries` |
  | Heart rate, HRV, sleep, stress, Body Battery, SpO2, respiration, hydration | `client.Wellness` |
  | Blood pressure | `client.BloodPressure` |
  | Weight & body composition (FIT upload) | `client.Weight` |
  | VO2max, training readiness/status, endurance/hill scores, race predictions | `client.Metrics` |
  | Lactate threshold, FTP, HR/power zones | `client.Biometrics` |
  | Personal records, badges, challenges | `client.Records` |
  | Goals | `client.Goals` |
  | Devices, solar, alarms | `client.Devices` |
  | Activities (search, details, splits, weather, zones, CRUD) | `client.Activities` |
  | Upload (FIT/GPX/TCX) / download (original ZIP, TCX/GPX/KML/CSV) | `client.Upload` / `client.Download` |
  | Gear | `client.Gear` |
  | Workouts, calendar, training plans | `client.Workouts` |
  | Nutrition | `client.Nutrition` |
  | Women's health | `client.WomensHealth` |
  | Golf | `client.Golf` |
  | GraphQL gateway | `client.GraphQL` |

  Major domains return typed structs; long-tail endpoints return
  `json.RawMessage` (the exact Garmin payload). Any uncovered endpoint can be
  called through the low-level escape hatch `client.Do`.

- **MCP server** (`pkg/mcp` + `garmin mcp`): exposes the client as a
  [Model Context Protocol](https://modelcontextprotocol.io) server so LLM
  agents can read your Garmin data. See [MCP server](#mcp-server) below.

- Zero dependencies (standard library only), Go ≥ 1.26.

## Getting tokens (CLI)

The library's entry point is a set of Garmin "DI" tokens. Mint them once with
the bundled CLI:

```console
$ go run ./cmd/garmin login
Email: you@example.com
Password:
2FA code (sent via email): 123456
Tokens written to /Users/you/.garminconnect/garmin_tokens.json
Logged in as Jane Doe (f2f16b0e-…)
```

The token file (`~/.garminconnect/garmin_tokens.json` by default, or
`$GARMINTOKENS`, or `--tokens <path>`) is the same
`{"di_token","di_refresh_token","di_client_id"}` file python-garminconnect
writes — either library can use the other's file.

Other commands:

```console
$ go run ./cmd/garmin whoami    # verify the stored tokens
$ go run ./cmd/garmin refresh   # force a refresh (persists the rotated refresh token)
$ go run ./cmd/garmin mcp       # run the MCP server over stdio (see below)
```

> Garmin **rotates the refresh token on every refresh**: the file is rewritten
> automatically. If you copy tokens elsewhere (e.g. a CI secret), refresh
> invalidates the copied refresh token eventually — re-mint when that happens.

## Using the library

```go
import "github.com/ndeloof/go-garmin/pkg/garmin"

store := garmin.NewFileTokenStore("") // $GARMINTOKENS or ~/.garminconnect/garmin_tokens.json
client, err := garmin.NewClientFromStore(ctx, store)
if err != nil { /* run `garmin login` first */ }

// Typed calls
profile, _ := client.UserProfile.SocialProfile(ctx)
sleep, _ := client.Wellness.Sleep(ctx, garmin.Today().AddDays(-1))

// Transparent pagination (break to stop early)
for act, err := range client.Activities.All(ctx, nil) {
    if err != nil { return err }
    fmt.Println(act.ActivityName)
}

// Raw escape hatch for anything not covered
var raw json.RawMessage
_ = client.Do(ctx, http.MethodGet, "/some-service/endpoint", nil, nil, &raw)
```

Tokens are the construction parameter of the client — in CI or servers,
inject them directly:

```go
creds, _ := garmin.CredentialsFromEnv() // $GARMINTOKENS: file path OR inline JSON
client := garmin.NewClient(creds)
```

See [examples/activities](examples/activities/main.go) and the package
documentation for more.

## MCP server

`pkg/mcp` exposes the Garmin client as a [Model Context Protocol](https://modelcontextprotocol.io)
server over stdio (JSON-RPC 2.0, standard library only — no MCP SDK
dependency), inspired by [taxuspt/garmin_mcp](https://github.com/taxuspt/garmin_mcp).
It offers ~29 read tools: `get_profile`, `list_activities`, `get_activity`,
`get_activity_details`, `get_daily_summary`, `get_sleep`, `get_heart_rate`,
`get_body_battery`, `get_stress`, `get_hrv`, `get_training_readiness`,
`get_vo2max`, `get_weight`, `get_devices`, `get_personal_records`, and more.

It reuses the same token file as the library. Get one first with
`garmin login` (see above).

### Run locally

```console
$ GARMINTOKENS=~/.garminconnect/garmin_tokens.json go run ./cmd/garmin mcp
```

Configure an MCP client (e.g. Claude Desktop) to launch it:

```json
{
  "mcpServers": {
    "garmin": {
      "command": "garmin",
      "args": ["mcp"],
      "env": { "GARMINTOKENS": "/Users/you/.garminconnect/garmin_tokens.json" }
    }
  }
}
```

### Run in Docker

Build the image and run the server, sharing your local token directory
(mounted read-write so rotated refresh tokens persist back to the host):

```console
$ docker compose run --rm garmin-mcp
```

or, as a standalone MCP client command:

```console
$ docker run -i --rm \
    -v ~/.garminconnect:/tokens \
    ndeloof/go-garmin mcp
```

The corresponding MCP client config:

```json
{
  "mcpServers": {
    "garmin": {
      "command": "docker",
      "args": ["run", "-i", "--rm",
               "-v", "/Users/you/.garminconnect:/tokens",
               "ndeloof/go-garmin", "mcp"]
    }
  }
}
```

The [`Dockerfile`](Dockerfile) builds a static binary into a distroless image;
[`compose.yaml`](compose.yaml) wires the token mount and stdio for you.

## Integration tests

Correct behavior against the real Garmin API is validated by the integration
suite, driven by a token provided through the environment (never committed):

```console
$ go run ./cmd/garmin login    # once, to mint the token file
$ GARMINTOKENS=~/.garminconnect/garmin_tokens.json go test -tags integration ./integration/ -v
```

- Tests are **read-only** by default (profile, activities, sleep, heart rate,
  devices…). Reversible write tests (hydration log + revert, activity rename +
  restore) only run with `GARMIN_INTEGRATION_WRITE=1`.
- On GitHub CI, the [Integration workflow](.github/workflows/integration.yml)
  runs on manual dispatch and nightly — never on pull requests — with the
  token JSON injected from the `GARMINTOKENS` repository **secret**:

  ```console
  $ gh secret set GARMINTOKENS < ~/.garminconnect/garmin_tokens.json
  ```

Unit tests (`go test ./...`) run entirely against local `httptest` servers and
need no credentials.

## License

Apache-2.0. Not affiliated with, endorsed by, or supported by Garmin.
