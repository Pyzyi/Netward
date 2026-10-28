# Contributing to Netward

Thanks for helping! Bug reports, device fingerprints, translations and code are all welcome.
Issues and pull requests can be written in **English or German**.

## Reporting bugs

Open an [issue](https://github.com/Pyzyi/Netward/issues/new/choose) and include:

- Netward version (*Settings → About*) and platform (ZimaOS, Unraid, TrueNAS, plain Docker …)
- what you expected and what happened
- relevant log lines (`docker logs netward`); remove anything private first

Security problems go to [private vulnerability reporting](https://github.com/Pyzyi/Netward/security/advisories/new),
not to public issues. See [SECURITY.md](SECURITY.md).

## A device is recognised wrongly

This is the most useful kind of report. Open the device in Netward, look at *Identified as* (hostname, mDNS services,
DHCP fingerprint) and paste those details into a "Device recognition" issue. Please leave out MAC addresses.

## Development

Requirements: Go 1.27 and Node 22.

```bash
make build      # web UI + binary → bin/netward
make test       # go vet + tests with the race detector
make demo       # sample data on http://localhost:8840 (NETWARD_DEMO=en for English names)
```

For the web UI with hot reload, start `make demo` with `NETWARD_PORT=18840` and run `npm run dev` in `web/`.
Before opening a pull request, please run:

```bash
make test
cd web && npm run check && npm run contrast
cd e2e && npx playwright test        # against a running demo on :18850
```

## Guidelines

- Keep changes small and focused; one topic per pull request.
- Netward is **defensive**: it observes and explains. Features that attack devices (blocking via ARP spoofing,
  brute-forcing, exploiting) will not be merged.
- No telemetry, no new outbound connections without an opt-in setting.
- User-facing text exists in German and English (`web/src/lib/i18n.svelte.ts`). Plain words, no jargon, a concrete
  next step for every problem.
- The design rules are in [docs/05-ux-design.md](docs/05-ux-design.md) (German): calm panels, sentence case, monospace
  only for addresses.

By contributing you agree that your contribution is licensed under the [MIT License](LICENSE).
