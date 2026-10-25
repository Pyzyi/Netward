# 06 · Teststrategie

> Rolle: QA Lead · Status: **überarbeitet nach Plan-Review 1** (siehe [07](07-review-protokoll.md))

## 1. Testpyramide

| Ebene | Werkzeug | Umfang | Wann |
|---|---|---|---|
| Statische Analyse | `go vet`, `staticcheck`, `svelte-check`, `tsc` | gesamter Code | jeder Push |
| Unit-Tests | `go test -race` | Parser, Klassifizierer, Presence-Automat, Auth, Limiter, Export, Store | jeder Push |
| Fuzz-Tests | `go test -fuzz` | ARP, DHCP, NetBIOS, SSDP/UPnP-XML, mDNS-Auswertung | jeder Push (30 s je Ziel), nightly länger |
| API-Tests | `httptest` | Auth-Pflicht, CSRF, Setup-Schutz, Security-Header, write-only Geheimnisse, Validierung | jeder Push |
| Integrations-Test | Docker-Netz | Netzwart-Container scannt Testcontainer per ARP und findet alle | jeder Push |
| Container-Härtungstest | Docker | Prozess-UID ≠ 0, `CapEff` = 0, read-only FS, Healthcheck grün; Matrix: vorhandenes Verzeichnis mit UID 1000, kein Volume, `user:` gesetzt, ohne `NET_RAW`; `reset-password` per `docker exec` im laufenden Betrieb | jeder Push |
| MQTT-Integration | Mosquitto-Container | Discovery, retained State, LWT, Reconnect | jeder Push |
| E2E-UI | Playwright | Setup → Login → Übersicht → Gerät umbenennen → Logout; keine CSP-Verletzungen, keine Konsolenfehler | jeder Push |
| Sicherheits-Scans | `govulncheck`, `npm audit`, Trivy | Abhängigkeiten, Image | jeder Push + wöchentlich |

## 2. Wichtige Testfälle

### Erkennung
- ARP-Frame-Encoding stimmt Byte für Byte mit RFC 826 überein; Antworten mit falscher Länge werden verworfen.
- DHCP: Hostname (Option 12), Vendor-Class (60), Parameter-Request-Liste (55) werden korrekt extrahiert; abgeschnittene Optionen führen nicht zur Panik.
- NetBIOS: Node-Status-Antwort mit 0–n Namen; Workstation-Name wird gewählt.
- SSDP: `LOCATION` mit fremdem Host wird ignoriert (SSRF).
- Normalisierung: Steuerzeichen, RTL-Overrides, > 128 Zeichen.

### Presence
- Gerät online → offline nach `offline_after` → online: zwei Sitzungen, korrekte Grenzen.
- Weck-Probe beantwortet → kein Offline.
- Mobiles Gerät nutzt `offline_after_mobile`.
- Neustart mit frischem Heartbeat: Sitzungen laufen weiter. Neustart nach langer Pause: Sitzungen werden geschlossen, Lücke gespeichert.
- Lernphase: erster Scan erzeugt keine Benachrichtigungen.
- IP-Konflikt: zwei MACs auf dieselbe Anfrage in zwei aufeinanderfolgenden Zyklen → genau ein Ereignis; einmaliges Auftreten → keins.
- Repeater: eine MAC mit ≥ 3 IPs → Proxy, Geräte je IP.
- Flut: 100 zufällige MACs → max. 20 Geräte, ein `device_flood`-Ereignis.

### Sicherheit
- Alle `/api/v1/*`-Endpunkte außer `auth/state`, `auth/setup`, `auth/login` liefern ohne Sitzung `401`.
- POST ohne/mit falschem CSRF-Token → `403`; POST ohne `Origin` → `403`; Origin `http://evil.example` → `403`.
- `Host: evil.example` → `421` (DNS-Rebinding).
- Setup von fremdem Subnetz → `403`; nach Ablauf des Zeitfensters → `403`; zweites Setup → `409`.
- Egress: Webhook auf `127.0.0.1`, `localhost`, `169.254.169.254`, `[::1]` wird beim Connect abgelehnt, auch nach Redirect.
- Benachrichtigungs-Encoder: `@everyone`, `<!channel>`, Telegram-Markup, CRLF im Betreff werden neutralisiert.
- SSE: Abmelden beendet offene Streams.
- 6 Fehlversuche → `429` mit `Retry-After`.
- Kanal-Geheimnisse erscheinen in keiner GET-Antwort.
- CSV-Export präfixiert `=cmd|' /C calc'!A0`.
- Antwort-Header: CSP, `X-Content-Type-Options`, `Referrer-Policy`, `frame-ancestors`.

## 3. Abnahmekriterien für Release

1. Alle Tests und Scans grün, keine kritischen/hohen Befunde von Trivy/govulncheck.
2. Review-Protokoll ohne offene Befunde der Schwere „hoch“.
3. Kaltstart-Test: `docker compose up` mit der README-Compose-Datei → Healthcheck grün ≤ 45 s.
4. Manueller Import der Compose-Datei in ZimaOS (Benutzerdefinierte App) inkl. Bearbeiten/Speichern.
