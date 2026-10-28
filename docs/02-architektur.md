# 02 · Architektur

> Rolle: Software-Architekt · Status: **überarbeitet nach Plan-Review 1** (siehe [07](07-review-protokoll.md))

## 1. Überblick

```
┌──────────────────────────── Docker-Container (network_mode: host) ───────────────────────────┐
│                                                                                               │
│  /netzwart  (ein statisches Go-Binary, CGO_ENABLED=0, Frontend per go:embed eingebettet)      │
│                                                                                               │
│   ┌───────────────┐   Sichtungen   ┌──────────────┐  Ereignisse  ┌────────────────────────┐   │
│   │ scan/arp      │──────────────▶ │              │────────────▶ │ notify (Dispatcher)    │──▶ ntfy, Gotify, Telegram, …
│   │ (aktiver Sweep)│               │   monitor    │              └────────────────────────┘   │
│   ├───────────────┤                │ (Presence-   │              ┌────────────────────────┐   │
│   │ scan/passive  │──────────────▶ │  Zustands-   │────────────▶ │ mqtt (Home Assistant)  │──▶ Broker
│   │ (ARP + DHCP)  │                │  automat)    │              └────────────────────────┘   │
│   └───────────────┘                │              │              ┌────────────────────────┐   │
│   ┌───────────────┐   Namen/       │              │────────────▶ │ api (REST + SSE)       │◀── Browser
│   │ discovery/*   │   Dienste      │              │              │ auth · csrf · headers  │   │
│   │ mDNS NetBIOS  │──────────────▶ │              │              └────────────────────────┘   │
│   │ SSDP DNS DHCP │                └──────┬───────┘                                           │
│   └───────────────┘                       │                                                   │
│   ┌───────────────┐ ┌───────────────┐     ▼                                                   │
│   │ oui · classify│ │ internet      │  store (SQLite, WAL)  ──▶ /data/netzwart.db            │
│   └───────────────┘ │ security      │                                                         │
│                     └───────────────┘                                                         │
└───────────────────────────────────────────────────────────────────────────────────────────────┘
```

## 2. Bausteine (Go-Pakete)

| Paket | Verantwortung |
|---|---|
| `cmd/netzwart` | Einstieg, Subkommandos `serve` (Standard), `healthcheck`, `reset-password`, `factory-reset`, `version` |
| `internal/privdrop` | Startet als root: `/data` übertragen, Raw-Sockets öffnen, auf UID 10001 wechseln und dabei alle Capabilities verlieren |
| `internal/egress` | Zentraler Dialer/HTTP-Client mit Sperre für Loopback, Link-Local und Multicast |
| `internal/config` | Umgebungsvariablen (Bootstrap) und persistierte Einstellungen |
| `internal/store` | SQLite-Zugriff, Migrationen, Repositories, Aufbewahrung |
| `internal/netinfo` | Interfaces, Subnetze, Default-Gateway (`/proc/net/route`), DNS-Server, ARP-Cache |
| `internal/scan/arpscan` | ARP-Sweep und Proben über einen langlebigen `AF_PACKET`-Socket; Fallback: UDP-Anstoß + rtnetlink |
| `internal/scan/passive` | Mithören von ARP, DHCP und mDNS mit Kernel-BPF-Filter |
| `internal/scan/ports` | TCP-Connect-Portscan mit Ratenbegrenzung |
| `internal/discovery` | mDNS (Unicast- und Multicast-Abfragen), NetBIOS-Node-Status, SSDP/UPnP, Reverse-DNS, DHCP-Fingerprints |
| `internal/oui` | Eingebettete, komprimierte IEEE-OUI-Datenbank (MA-L/MA-M/MA-S) |
| `internal/classify` | Regelbasierte Gerätetyp-Erkennung mit Konfidenz |
| `internal/monitor` | Orchestrierung: Scan-Zyklen, Anreicherung, Presence-Zustandsautomat, Ereignisse |
| `internal/notify` | Kanäle, Vorlagen (DE/EN), Bündelung, Wiederholung |
| `internal/mqtt` | MQTT-3.1.1-Client (QoS 1, LWT, retained, Subscribe, TLS) + Home-Assistant-Discovery |
| `internal/fritzbox` | Optionale FRITZ!Box-Anbindung (TR-064 Host-Liste: Namen und WLAN-Anmeldung, HTTPS mit Zertifikats-Pinning, ADR-014) |
| `internal/internet` | Konnektivitäts- und Latenzmessung, Ausfallerkennung |
| `internal/security` | Sicherheitscheck (riskante Ports, UPnP, IP-Konflikte, unbekannte Geräte) |
| `internal/auth` | Argon2id, Sitzungen, Login-Drosselung |
| `internal/api` | HTTP-Router (Go 1.22+ `ServeMux`), Middleware, Handler, SSE-Hub |
| `internal/web` | Eingebettetes Frontend (Svelte-Build) mit Cache-Headern |
| `web/` | Svelte-5-Frontend (TypeScript, Vite) |

## 3. Datenmodell (SQLite)

```
devices            id, ident (unique: MAC, hinter Repeatern "MAC/IP"), mac, ip (primär), name, hostname,
                   vendor, model, os, type, type_override, location, notes, known, favorite, watch,
                   mac_kind (universal|random|virtual), proxy (Repeater/Proxy-ARP), via_proxy, online,
                   is_gateway, is_self, first_seen, last_seen, interface, dhcp_fingerprint, ports_scanned_at
device_addresses   device_id, ip, interface, first_seen, last_seen     (mehrere IPs je Gerät)
device_names       device_id, source (dhcp|mdns|mdns_instance|netbios|ssdp|dns), name, updated_at
device_services    device_id, source (mdns|ssdp), service, detail, updated_at
device_ports       device_id, port, proto, service, first_seen, last_seen
presence           id, device_id, started_at, ended_at (NULL = offen)
events             id, ts, kind, severity, device_id, data (JSON)
internet_outages   id, started_at, ended_at
monitor_gaps       id, started_at, ended_at          (Zeiten, in denen Netzwart nicht lief)
security_findings  id, check_id, device_id, severity, data, first_seen, last_seen, dismissed
settings           key, value (JSON)
users              id, username, password_hash, created_at, updated_at
sessions           id, token_hash (SHA-256), user_id, csrf_token, created_at, last_seen_at,
                   expires_at, remember, user_agent, ip
channels           id, name, kind, config (JSON, Geheimnisse write-only), events (JSON), enabled
schema_migrations  version
```

Zeitstempel sind Unix-Millisekunden (UTC). Die Darstellung erfolgt in der Zeitzone des Browsers.

## 4. Kernabläufe

### 4.1 Scan-Zyklus (Standard alle 90 s)

1. Für jedes überwachte Interface und Subnetz: ARP-Requests mit ca. 500 pps senden, Antworten sammeln,
   Nicht-Antwortende bis zu zweimal erneut anfragen.
2. Jede Antwort wird als **Sichtung** `(mac, ip, iface, quelle, zeitpunkt)` an den Monitor gegeben.
3. Neue Geräte und Geräte mit veralteter Anreicherung (> 6 h) werden asynchron angereichert:
   Reverse-DNS, mDNS-Unicast-Abfrage, NetBIOS, SSDP-Beschreibung; danach wird der Typ neu klassifiziert.
4. Ein Sweeper markiert Geräte als offline, wenn `last_seen < jetzt − offline_after`.

Geräte entstehen **nur aus ARP-Sichtungen** (aktiv oder passiv) mit Absender-IP im überwachten Subnetz.
DHCP- und mDNS-Daten zu noch unbekannten MACs werden in einem begrenzten Zwischenspeicher gehalten
und erst angewendet, wenn ARP das Gerät bestätigt. Neue Geräte unterliegen einem Aufnahme-Budget
(20 pro Minute, insgesamt 4 000). Darüber hinaus entsteht genau ein Ereignis `device_flood`.

### 4.2 Presence-Zustandsautomat

```
            Sichtung (neu)                    Sichtung
  [unbekannt] ───────────▶ [online] ◀─────────────────────┐
                              │                           │
                              │ keine Sichtung            │
                              │ für offline_after         │
                              ▼                           │
                          [offline] ──────────────────────┘
```

- **Gültige Lebenszeichen**: ARP-*Absender*felder (nie Zielfelder, nie Absender 0.0.0.0), DHCP-Pakete
  (belegen die MAC, nicht die IP), passive mDNS-Pakete und Antworten auf aktive Proben.
- **Schwellen je Typ**: `offline_after` (Standard 5 min), für Telefone, Tablets und Uhren
  `offline_after_mobile` (Standard 15 min), weil diese im Energiesparmodus ARP minutenlang ignorieren.
- **Weck-Proben**: Bevor ein Gerät als offline gilt, sendet Netzwart einen ARP-Unicast-Burst, ICMP-Echo,
  einen TCP-Connect auf Port 62078 (iOS) und eine mDNS-Unicast-Anfrage. Jede Antwort hält das Gerät online.
- Übergang *online*: Presence-Sitzung öffnen; Ereignis `device_online` nur bei beobachteten Geräten.
- Übergang *offline*: Sitzung mit `ended_at = last_seen` schließen.
- **Überwachungslücken**: Netzwart schreibt minütlich einen Heartbeat. Ist er beim Start älter als 3 min,
  werden offene Sitzungen bei `min(last_seen, heartbeat)` geschlossen und die Lücke wird gespeichert.
  Die Zeitleiste zeigt sie schraffiert, der Online-Anteil zählt nur beobachtete Zeit.
- **Startgate**: Der Offline-Sweeper wertet erst nach dem ersten vollständigen Scan aus,
  spätestens jedoch 3 min nach dem Start.
- **Lernphase**: Geräte aus dem allerersten Scan einer Neuinstallation lösen kein „neues Gerät“ aus.
  „Neu“ bedeutet: zuerst gesehen nach der Lernphase und noch nicht als bekannt markiert.
- **Mehrere IPs**: Jede IP wird in `device_addresses` geführt. Primär ist die zuletzt per ARP bestätigte IP.
  `ip_changed` entsteht nur, wenn die alte IP länger als `offline_after` ungesehen ist.
- **Repeater/Proxy-ARP**: Antwortet eine MAC für ≥ 3 verschiedene IPs, wird sie als Proxy markiert.
  Geräte dahinter erhalten die Identität „MAC/IP“, damit sie nicht zu einem Gerät verschmelzen.
- **IP-Konflikt**: Nur wenn zwei verschiedene (nicht-Proxy-)MACs auf *dieselbe* ARP-Anfrage antworten
  und sich das im folgenden Zyklus wiederholt.

### 4.3 Namens-Priorität

`Nutzername > mDNS-Instanzname > SSDP friendlyName > DHCP-Hostname > mDNS-Hostname > NetBIOS > Reverse-DNS (ohne lokale Domain) > „<Hersteller>-Gerät“ > MAC`

Alle Namen aus dem Netz sind **nicht vertrauenswürdig**: Sie werden auf druckbare Unicode-Zeichen
normalisiert, auf 128 Zeichen gekürzt und im Frontend nie als HTML gerendert.

## 5. Architekturentscheidungen (ADRs)

### ADR-001 · Go als Backend-Sprache
- **Kontext:** Raw-Sockets, Nebenläufigkeit, kleines Image, mehrere Architekturen.
- **Entscheidung:** Go mit `CGO_ENABLED=0`. Ergebnis ist ein einziges statisches Binary.
- **Alternativen:** Python (NetAlertX) mit großem Image und vielen Laufzeit-Abhängigkeiten; Rust mit längerer Entwicklungszeit bei ähnlichem Nutzen.
- **Konsequenz:** Cross-Compile für arm64 ist trivial, das Image kann auf `scratch` basieren.

### ADR-002 · SQLite (pure Go, `modernc.org/sqlite`)
- **Entscheidung:** Eine Datei in `/data`, WAL-Modus, `foreign_keys=ON`, `busy_timeout`.
- **Begründung:** Kein zweiter Container, transaktional, eine Datei.
- **Konsequenz:** `_txlock=immediate` gegen Lock-Upgrade-Konflikte; `last_seen` wird im Speicher gepuffert
  und alle 15 s geschrieben, Sitzungen höchstens 1×/min (Schonung von eMMC-Speicher).
  Nächtliches `VACUUM INTO /data/backup/` (3 Generationen) als konsistentes Backup im laufenden Betrieb.
  `/data` darf nicht auf NFS/SMB liegen (WAL braucht lokale Sperren).

### ADR-003 · Eigene, minimale Protokoll-Implementierungen
- **Entscheidung:** ARP, DHCP-Parsing, NetBIOS, SSDP und MQTT-Publish werden selbst implementiert.
  DNS-Nachrichten laufen über `golang.org/x/net/dns/dnsmessage`, BPF über `golang.org/x/net/bpf`.
- **Begründung:** Weniger Abhängigkeiten bedeuten eine kleinere Supply-Chain-Angriffsfläche. Die Protokolle sind klein und gut spezifiziert.
- **Absicherung:** Jeder Parser für Netzwerkdaten bekommt Unit- **und Fuzz-Tests**.
- **Direkte Abhängigkeiten:** `modernc.org/sqlite`, `golang.org/x/crypto`, `golang.org/x/net`, `golang.org/x/sys`.
  Der MQTT-Client ist ein vollwertiger Baustein (QoS 1, LWT, retained, Subscribe, TLS, Reconnect) und wird gegen einen echten Mosquitto getestet.

### ADR-004 · Sockets als root öffnen, dann alle Rechte abgeben
- **Kontext:** Layer-2-Scans brauchen `AF_PACKET`- und ICMP-Raw-Sockets (`CAP_NET_RAW`). Ein Bind-Mount
  (`/DATA/AppData/netzwart`) wird von Docker als root angelegt. Root ohne `DAC_OVERRIDE` kann fremde
  Verzeichnisse aber weder ändern noch betreten.
- **Entscheidung:** Der Container startet als root mit genau `CHOWN, SETUID, SETGID, NET_RAW`. `internal/privdrop`:
  1. setzt `umask 077` und `PR_SET_NO_NEW_PRIVS` selbst (unabhängig von der Compose-Datei),
  2. führt ein **nicht rekursives** `lchown` von `/data` auf `PUID:PGID` aus (Standard 10001). Kein chmod und kein Durchlaufen als root,
  3. öffnet alle Raw-Sockets für die infrage kommenden Interfaces (ARP, IPv4-Filter, ICMP),
  4. ruft `setgroups([])`, `setresgid`, `setresuid` auf (Go wendet das auf alle Threads an). **Ohne KEEPCAPS**
     löscht der Kernel dabei alle Capabilities.
  5. Erst danach wird `/data` als Eigentümer geprüft (`chmod 0700`, Schreibtest) und SQLite geöffnet.
- **Ergebnis:** Laufzeitprozess mit UID 10001 und **CapEff = 0**. Die Raw-Sockets funktionieren weiter, weil der
  Kernel `CAP_NET_RAW` nur beim Anlegen des Sockets prüft. Kein Re-Exec, kein Ambient-Capability-Zustand pro Thread.
- **Konsequenz:** Die Menge der Interfaces ist pro Prozessstart fest. Wird ein Interface entfernt oder neu
  angelegt, beendet sich Netzwart kontrolliert und die Restart-Policy startet neu.
- **Subkommandos** (`reset-password`, `factory-reset`, `healthcheck`) wechseln vor jedem Zugriff auf PUID:PGID.
  So entstehen nie root-eigene WAL-Dateien.
- **Fallback:** Startet der Container bereits als Nicht-root (`user:` in Compose), bekommt der Prozess von Docker keine
  Capabilities. Dann greift der unprivilegierte ARP-Fallback (ADR-005). Die Oberfläche zeigt das an.
- **Abgelehnt:** Ambient-Capabilities mit Re-Exec (Per-Thread-Zustand, fehleranfällig); Shell-Entrypoint mit `setpriv`
  (braucht Shell und Paketmanager im Image); Root-Betrieb (CIS Docker 4.1).

### ADR-005 · ARP über AF_PACKET mit unprivilegiertem Fallback
- **Entscheidung:** Pro Interface gibt es zwei langlebige Sockets:
  - `ETH_P_ARP`: Senden der Sweep-Anfragen und Empfang *aller* ARP-Pakete, also aktiv und passiv in einem,
  - `ETH_P_IP` mit Kernel-BPF-Filter (nur unfragmentiertes UDP an 67/68/5353) und `PACKET_IGNORE_OUTGOING`.
    Bei NAS-Durchsatz ist die Last vernachlässigbar.
  Der Filter wird vor `bind` angehängt, sodass kein ungefilterter Frame in die Warteschlange gelangt.
- **Fallback ohne `NET_RAW`:** Jede Ziel-IP wird per UDP-Datagramm (Port 9) angestoßen, danach wird der
  Nachbarstatus per rtnetlink (`RTM_GETNEIGH`) gelesen. Nur `REACHABLE`-Einträge zählen. Maximal /24,
  um die Nachbartabelle des Hosts nicht zu überlaufen.
- **Konsequenz:** Die Funktion bleibt auch in restriktiven Umgebungen erhalten. Die Oberfläche zeigt den Modus an.

### ADR-006 · Svelte 5 + Vite, ohne UI-Framework
- **Entscheidung:** Svelte 5 (Runes) mit TypeScript und eigenem, kleinem Designsystem (CSS-Variablen).
  Icons: Phosphor (MIT). Schrift: IBM Plex Sans / Mono, selbst gehostet.
- **Begründung:** Kleines Bundle (< 150 kB gz), kein generischer Komponenten-Look, volle Kontrolle über die Optik.
- **Konsequenz:** Das Frontend wird zur Build-Zeit per `go:embed` in das Binary eingebettet.

### ADR-007 · Server-Sent Events statt WebSockets
- **Entscheidung:** `GET /api/v1/stream` (SSE) für Live-Updates.
- **Begründung:** Einweg-Kommunikation reicht. SSE funktioniert über Reverse-Proxies, nutzt die Cookie-Authentifizierung und braucht keine Zusatzbibliothek.
- **Details:** Heartbeat alle 20 s mit erneuter Sitzungsprüfung (Abmelden/Widerruf beendet den Stream).
  Kein gzip, `X-Accel-Buffering: no`, kein globaler `WriteTimeout`. Der Client schließt den Stream bei
  verborgenem Tab und lädt beim Zurückkehren neu. So bleibt das 6-Verbindungen-Limit von HTTP/1.1 frei.

### ADR-008 · Cookie-Sitzungen + CSRF-Token
- **Entscheidung:** Zufälliges 256-bit-Token im Cookie `netzwart_session` (`HttpOnly`, `SameSite=Strict`;
  bei TLS `__Host-netzwart_session` mit `Secure`). In der DB liegt nur der SHA-256-Hash.
  Zustandsändernde Requests brauchen zusätzlich den Header `X-CSRF-Token` und eine Origin aus der
  Host-Allowlist (ADR-012). Leerlauf-Timeout 7 d (bzw. 30 d), absolute Lebensdauer 30 d.
- **Begründung:** Gegen XSS-Diebstahl (HttpOnly) und CSRF (drei unabhängige Schichten).
  Cookies sind nicht pro Port isoliert. Andere Apps auf demselben Host sehen das Cookie,
  deshalb sind Origin-Prüfung und Token die eigentlichen Schutzschichten.

### ADR-009 · Scratch-Image, Multi-Arch, signiert
- **Entscheidung:** Multi-Stage-Build (`node` → `golang` → `scratch`). Images für `amd64` und `arm64` werden in GitHub Actions gebaut,
  nach GHCR veröffentlicht, mit SBOM und Provenance versehen und mit Trivy gescannt.
- **Konsequenz:** Kein Paketmanager und keine Shell im Image. Updates kommen über neue Image-Tags.

### ADR-010 · Ersteinrichtung: lokal, zeitlich begrenzt, rücksetzbar
- **Kontext:** Zwischen Container-Start und Ersteinrichtung existiert kein Konto.
- **Entscheidung:** `POST /api/v1/auth/setup` wird nur akzeptiert, wenn
  1. noch kein Benutzer existiert (atomar in einer Transaktion),
  2. der Prozess vor höchstens 30 min gestartet wurde (danach: „Container neu starten, um einzurichten“),
  3. die TCP-Quelladresse Loopback ist oder in einem direkt angeschlossenen Subnetz liegt. Weitergeleitete Anfragen
     (`X-Forwarded-For`, `Forwarded`) gelten nur von vertrauten Proxys, dann mit der Adresse des Clients,
  4. Host- und Origin-Prüfung bestanden sind (ADR-012),
  5. optional `NETZWART_SETUP_TOKEN` stimmt.
- **Wiederherstellung:** `reset-password` (neues Passwort, alle Sitzungen widerrufen), `factory-reset` (legt die Trigger-Datei an; ausgeführt beim nächsten Start, damit kein laufender Prozess alte Kanäle oder Einstellungen zurückschreibt)
  (löscht Konten, Sitzungen, Kanäle, MQTT und Einstellungen; Geräte bleiben) sowie ein Datei-Trigger:
  Eine Datei `reset-password` im AppData-Ordner erzeugt beim nächsten Start ein Einmalpasswort im Log.
- **Restrisiko:** Ein Angreifer im selben LAN könnte die Einrichtung innerhalb des Zeitfensters vor dem Besitzer
  abschließen. Das fällt sofort auf und lässt sich per `factory-reset` vollständig zurücksetzen.

### ADR-011 · Zentrale Egress-Richtlinie
- **Kontext:** Im Host-Netzwerk ist `127.0.0.1` die Loopback-Schnittstelle des Hosts. Dort lauschen
  ZimaOS/CasaOS-Verwaltungsdienste, die lokale Aufrufe teils ungeprüft vertrauen.
- **Entscheidung:** Alle ausgehenden Verbindungen (Webhooks, ntfy, Gotify, Pushover, Telegram, Discord, Slack, SMTP,
  MQTT, SSDP-Beschreibungen, FRITZ!Box) laufen über einen Dialer. Dessen `Control`-Hook verbietet beim
  Verbindungsaufbau Loopback, Unspecified, Link-Local (inkl. 169.254.169.254) und Multicast.
  Weil die Prüfung beim Connect stattfindet, erfasst sie auch DNS-Auflösung und Redirects. Redirects sind zusätzlich
  auf 3 begrenzt. Kanaltests liefern nur Erfolg/Statuscode, nie den Antwortinhalt.
- **Konsequenz:** Ein ntfy/Gotify/Mosquitto auf demselben Host wird über dessen LAN-IP angesprochen, nicht über `localhost`.
  Die Oberfläche erklärt das bei der Eingabe.

### ADR-012 · Host-Allowlist gegen DNS-Rebinding
- **Entscheidung:** Jeder Request wird nur bedient, wenn der `Host`-Header ein IP-Literal, `localhost`, ein
  einteiliger Name (`zimaos`), ein Name mit lokalem Suffix (`.local`, `.lan`, `.home`, `.home.arpa`,
  `.internal`, `.fritz.box`, …) oder in `NETZWART_ALLOWED_HOSTS` eingetragen ist. Andernfalls antwortet Netzwart mit `421`.
  Unsichere Methoden verlangen zusätzlich einen `Origin`-Header, dessen Host in derselben Allowlist steht.
  Ergänzend läuft Go's `http.CrossOriginProtection`.
- **Begründung:** Beim DNS-Rebinding kontrolliert der Angreifer Host *und* Origin (`evil.example`). Eine Prüfung
  „Origin == Host“ hilft deshalb nicht, nur eine Allowlist hilft.

### ADR-013 · Watchdog statt „unhealthy“
- **Kontext:** Docker (ohne Swarm) startet Container mit Status „unhealthy“ nicht neu.
- **Entscheidung:** Ein interner Watchdog beendet den Prozess mit Exit-Code ≠ 0, wenn der Scanner länger als
  3 Intervalle keinen Zyklus abgeschlossen hat oder Datenbank-Schreibzugriffe dauerhaft scheitern.
  Die Restart-Policy `unless-stopped` startet dann neu.

### ADR-014 · FRITZ!Box über TR-064 mit Zertifikats-Pinning

- **Kontext:** Die FRITZ!Box kennt die vom Nutzer vergebenen Gerätenamen und weiß, welche Handys im WLAN angemeldet sind,
  auch wenn sie im Tiefschlaf kein ARP beantworten. TR-064 ist die offizielle Schnittstelle dafür.
- **Entscheidung:** Ein Poller liest jede Minute die Host-Liste (`X_AVM-DE_GetHostListPath`, Fallback `GetGenericHostEntry`).
  Authentifizierte Aufrufe nur per HTTPS (Port aus `GetSecurityPort`), Zertifikat per SHA-256 gepinnt (TOFU), HTTP-Digest.
  Namen fließen als Quelle `fritzbox` in die Namens-Priorität; eine aktive WLAN-Anmeldung zählt als Anwesenheitsbeweis.
  Geräte werden **nie** aus der Router-Liste angelegt (sie enthält auch längst verschwundene Geräte).
- **Konsequenz:** Optional und standardmäßig aus. Ein getauschter Router erfordert einmal Aus-/Einschalten der Anbindung.
  Bedrohung T-29.

## 6. Konfiguration (Bootstrap per Umgebung)

| Variable | Standard | Zweck |
|---|---|---|
| `NETZWART_PORT` | `8840` | HTTP-Port (host-Netzwerk) |
| `NETZWART_LISTEN` | `0.0.0.0` | Bind-Adresse |
| `NETZWART_DATA` | `/data` | Datenverzeichnis |
| `PUID` / `PGID` | `10001` | Laufzeit-Benutzer |
| `NETZWART_INTERFACES` | automatisch (Interface der Default-Route) | Kommagetrennte Interface-Namen, die zusätzlich bereitgestellt werden |
| `NETZWART_SETUP_TOKEN` | – | Optionales Token für die Ersteinrichtung |
| `NETZWART_ALLOWED_HOSTS` | – | Zusätzliche Hostnamen (z. B. `netzwart.example.com` hinter einem Reverse-Proxy) |
| `NETZWART_TRUSTED_PROXIES` | – | CIDRs, deren `X-Forwarded-*`-Header vertraut wird |
| `NETZWART_TLS_CERT` / `_KEY` | – | Optional direktes HTTPS |
| `NETZWART_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `TZ` | `UTC` | Zeitzone für Logs und Benachrichtigungen |

Alles Weitere wird in der Oberfläche eingestellt und in SQLite gespeichert.

## 7. Schnittstellen (REST, `/api/v1`)

| Methode | Pfad | Zweck |
|---|---|---|
| GET | `/healthz` | Liveness (ohne Auth, ohne Details) |
| GET | `/auth/state` | Setup nötig? Angemeldet? |
| POST | `/auth/setup`, `/auth/login`, `/auth/logout` | Konto und Sitzung |
| POST | `/auth/password` | Passwort ändern (widerruft alle anderen Sitzungen) |
| GET/DELETE | `/auth/sessions[/{id}]` | Sitzungen verwalten |
| GET | `/overview` | Netz, Zähler, Internet, Scanner-Status |
| GET | `/devices` · `/devices/{id}` | Liste / Detail |
| PATCH/DELETE | `/devices/{id}` | Bearbeiten / Vergessen |
| GET | `/devices/{id}/presence?range=` | Anwesenheits-Sitzungen |
| POST | `/devices/{id}/wake` · `/ping` · `/portscan` | Aktionen |
| POST | `/devices/acknowledge` | Alle neuen als bekannt markieren |
| POST | `/devices/{id}/merge` | Gerät mit einem anderen zusammenführen (z. B. nach MAC-Rotation) |
| GET | `/events` | Ereignisse (Cursor-Paginierung) |
| POST | `/scan` | Scan jetzt starten |
| GET/PUT | `/settings` | Einstellungen |
| GET | `/interfaces` | Verfügbare Interfaces/Subnetze |
| CRUD | `/channels[/{id}]`, POST `/channels/{id}/test` | Benachrichtigungen |
| GET/POST | `/security` · `/security/run` | Sicherheitscheck |
| GET | `/internet` | Status und Historie |
| GET | `/export?format=csv\|json` | Datenexport (ohne Geheimnisse) |
| GET | `/stream` | Server-Sent Events |
| GET | `/system` | Version, Modus (raw/fallback), DB-Größe |
