# ChillCheck BLE gateway

A small Go agent that runs on-site (Raspberry Pi or any Linux mini-PC), listens
to cheap Bluetooth temperature sensors broadcasting nearby, and forwards their
readings to the ChillCheck API. It is the piece that turns manual
logging into automatic monitoring.

Why a gateway at all: BLE sensors broadcast a few meters and have no internet
connection of their own. The gateway is the bridge — it scans, decodes, samples,
and ships readings, and it **buffers to disk during outages** so a dropped
connection never leaves a gap in the compliance record.

```
[BLE sensors] --broadcast--> [gateway: scan -> decode -> sample -> spool/send] --HTTPS--> [ChillCheck API]
```

## What it does

- **Scans** continuously for BLE advertisements (`tinygo.org/x/bluetooth`, BlueZ on Linux).
- **Decodes** known sensor formats — Xiaomi LYWSD03MMC on ATC1441/pvvx firmware
  (service `0x181A`) and the open BTHome v2 standard (service `0xFCD2`). Converts
  Celsius to Fahrenheit at the edge.
- **Samples** down to one reading per sensor per `sample_interval` (default 5 min),
  so you don't store a reading every two seconds.
- **Stores and forwards**: if the API is unreachable, readings go to a local
  JSON-lines spool and are retried on the next cycle. The spool is trimmed to
  `spool_max` records so it can't fill the disk.
- Preserves each reading's real timestamp, so buffered data lands in the log at
  the time it was measured, not the time it was finally delivered.

## Build toolchain: standard Go, not TinyGo

Despite the import path, this builds with the **standard Go toolchain** (`go build`,
`go run`) — there is no TinyGo here. `tinygo.org/x/bluetooth` ("Go Bluetooth") is a
normal Go module that just happens to live under the TinyGo project's namespace. It
targets two worlds: standard Go on Linux/macOS/Windows, and TinyGo on bare-metal
microcontrollers. We use the former.

On a Raspberry Pi (full Linux) it talks to **BlueZ over D-Bus** in pure Go — no CGo —
which is why the `CGO_ENABLED=0` cross-compile below works. The agent also relies on
`net/http`, the filesystem spool, and `os/signal`, so it is an OS-class program by
design (Pi, mini-PC, NUC). You would only reach for the actual TinyGo compiler to run
bare-metal on an ESP32/nRF with no OS — a different build, since there is no filesystem
or `net/http` there.

## Bind sensors to units (one-time setup)

1. In the ChillCheck app, open a location and create a **gateway** — copy the key
   it shows you (shown once).
2. For each unit, set its **sensor MAC** to the sensor sitting in that fridge/freezer.
   (API: `PUT /api/units/{id}/sensor` with `{"mac":"A4:C1:38:..."}`.)
3. Put the key in the gateway config and start it. The gateway sends every sensor
   it sees; the server records readings only for MACs bound to a unit and ignores
   the rest.

Until there's UI for this, it's a couple of authenticated calls. Grab a user
token by logging in, then:

```bash
TOKEN=$(curl -s localhost:8080/api/auth/login \
  -d '{"email":"demo@chillcheck.app","password":"chillcheck123"}' | jq -r .token)

# create a gateway for a location (returns the key once)
curl -s localhost:8080/api/locations/$LOCATION_ID/gateways \
  -H "Authorization: Bearer $TOKEN" -d '{"name":"Kitchen Pi"}'

# bind a sensor MAC to a unit
curl -s -X PUT localhost:8080/api/units/$UNIT_ID/sensor \
  -H "Authorization: Bearer $TOKEN" -d '{"mac":"A4:C1:38:00:00:01"}'
```

## Run it (simulation — no hardware)

Test the whole pipeline against your local backend with fake sensors:

```bash
cd gateway
cp config.example.yaml config.yaml
# set gateway_key (or export CHILLCHECK_GATEWAY_KEY), set simulate: true
go mod tidy
go run . --simulate
```

Bind the three simulated MACs (`A4:C1:38:00:00:01..03`) to units and watch the
status board update.

## Run it (real sensors, on a Pi)

```bash
cd gateway
cp config.example.yaml config.yaml   # set api_url + gateway_key, simulate: false
go build -o chillcheck-gateway .
sudo ./chillcheck-gateway --config config.yaml
```

BLE scanning needs Bluetooth privileges — run with `sudo`, or grant the binary
`CAP_NET_ADMIN`/`CAP_NET_RAW` (the included systemd unit does this).

### Cross-compile for a Raspberry Pi

```bash
# 64-bit Pi OS
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o chillcheck-gateway-arm64 .

# 32-bit Pi OS
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -o chillcheck-gateway-armv7 .
```

Copy the binary + `config.yaml` to `/opt/chillcheck-gateway/`, install
`systemd/chillcheck-gateway.service`, then `systemctl enable --now chillcheck-gateway`.

## Add a new sensor type

Implement the `Decoder` interface in `internal/ble/decode.go` (return the
temperature in Celsius and `true` if you recognize the advertisement) and add it
to `Decoders()`. The Xiaomi and BTHome decoders are worked examples; Govee and
SensorPush use manufacturer-data formats and are good next additions.

## Layout

```
gateway/
  main.go                      # wiring, run loop, store-and-forward delivery
  config.example.yaml
  systemd/chillcheck-gateway.service
  internal/
    config/   # YAML + env config
    ble/      # decoders (decode.go) + scanner & simulator (scan.go)
    sampler/  # newest-per-sensor, emit one batch per interval
    spool/    # disk buffer for offline readings
    client/   # HTTPS client for /api/ingest/readings
    reading/  # the shared Reading type
```

## Note

The gateway compiles and `go vet`s clean with the standard Go toolchain (`go.mod`/`go.sum`
are committed). For the real BLE path, build on a Linux box — it needs BlueZ over D-Bus
(`CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build .` cross-compiles to a Pi). Simulation mode
(`go run . --simulate`) needs neither Bluetooth nor privileges.
