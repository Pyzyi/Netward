# 07 · Review-Protokoll

Dieses Dokument hält alle Reviews fest: wer geprüft hat, was gefunden wurde und wie entschieden wurde.
Befunde werden nicht gelöscht, sondern mit Entscheidung und Nachweis versehen.

## Review 1 · Plan-Review (vor Implementierungsbeginn)

- **Gegenstand:** Dokumente 01–06 in der Erstfassung
- **Reviewer:** unabhängiger Senior-Engineer/Security-Architekt (eigener Agent, ohne Kenntnis der Entwurfsdiskussion)
- **Ergebnis:** 7 blockierende, 16 mittlere, 6 niedrige Befunde. Die Planung wurde überarbeitet, alle Dokumente sind angepasst.

| # | Schwere | Befund (Kurzfassung) | Entscheidung | Umsetzung |
|---|---|---|---|---|
| B1 | hoch, blockierend | DNS-Rebinding: „Origin == Host“ schützt nicht, weil der Angreifer beide kontrolliert. Setup/Login ohne CSRF-Schutz | **Angenommen.** Host-Allowlist (IP-Literale, `localhost`, einteilige Namen, lokale Suffixe, `NETZWART_ALLOWED_HOSTS`) für *jeden* Request, Origin-Prüfung gegen die Allowlist, unsichere Requests ohne `Origin` werden abgelehnt, zusätzlich `http.CrossOriginProtection` | ADR-012, T-24 |
| B2 | hoch, blockierend | SSRF über Webhooks/Benachrichtigungen auf Dienste an `127.0.0.1` des Hosts (Host-Netzwerk!), Kette bis Root auf dem Host | **Angenommen.** Ein zentraler Egress-Dialer verbietet beim Verbindungsaufbau Loopback, Unspecified, Link-Local und Multicast. Gilt auch für Redirects und DNS. Kanaltests liefern nur den Statuscode | ADR-011, T-25 |
| B3 | hoch, blockierend | Setup-Schutz „private IP“ zu grob, Reset entfernt keine Angreifer-Konfiguration | **Angenommen.** Setup nur ≤ 30 min nach Start (danach Neustart nötig), Quelle muss in einem direkt angeschlossenen Subnetz liegen. Dazu `factory-reset` und Reset per Datei-Trigger | ADR-010 |
| B4 | hoch, blockierend | Privilegien-Abgabe nach ADR-004 scheitert an fehlendem `DAC_OVERRIDE`, an Per-Thread-Zustand und an `docker exec` als root | **Angenommen, Design ersetzt:** Raw-Sockets als root öffnen, dann `setresuid` **ohne** KEEPCAPS. Zur Laufzeit gilt CapEff = 0, ohne Re-Exec. Subkommandos wechseln vor jedem Datenzugriff auf PUID | ADR-004 (neu) |
| B5 | hoch, blockierend | MAC als alleinige Identität bricht bei MAC-NAT-Repeatern, Multi-IP-Geräten und mehreren Interfaces | **Angenommen.** Tabelle `device_addresses`; Proxy/Repeater-Erkennung (≥ 3 gleichzeitig aktive IPs), dahinter Identität (MAC, IP). `ip_changed` erst, wenn die alte IP länger als `offline_after` ungesehen ist | Datenmodell |
| B6 | hoch, blockierend | Presence zu naiv: schlafende Handys flackern, Downtime erscheint als „online“ | **Angenommen.** Schwellen je Gerätetyp (mobil 15 min), Weck-Proben vor „offline“ (ARP-Unicast, ICMP, TCP 62078, mDNS), passives mDNS, Heartbeat und Überwachungslücken, Timeout für das Startgate | §4.2 Architektur |
| B7 | hoch, blockierend | Geräte-/Benachrichtigungsflut durch zufällige MACs; rotierende private MACs | **Angenommen.** Aufnahme-Budget (20 neue/min, max. 4 000), Geräte nur aus ARP-Sichtungen im Subnetz, Ablauf unbestätigter privater MACs, Schalter „Bei privaten MACs benachrichtigen“, virtuelle Präfixe als „virtuell“, Geräte zusammenführen | T-26 |
| M1 | mittel | `/proc/net/arp` kennt keinen Nachbarstatus | **Angenommen.** rtnetlink `RTM_GETNEIGH`, nur `REACHABLE`, Fallback max. /24 | ADR-005 |
| M2 | mittel | ZimaOS-/Compose-Details | **Angenommen.** `x-casaos`, `container_name`, Auto-Erkennung nur über das Default-Route-Interface, CA-Bundle und tzdata im Image, macvlan-Einschränkung dokumentiert | Compose, README |
| M3 | mittel | Docker startet „unhealthy“ Container nicht neu | **Angenommen.** Interner Watchdog beendet den Prozess bei hängendem Scanner/DB, die Restart-Policy übernimmt | ADR-013 |
| M4 | mittel | Argon2id 64 MiB × 2 sprengt RAM-Ziel, Login-DoS | **Angenommen.** m = 19 MiB, t = 2, p = 1; 1 Slot mit Warteschlangen-Timeout (503) | T-02 |
| M5 | mittel | Injektion über Gerätenamen in Benachrichtigungen | **Angenommen.** Encoder je Kanal (Telegram Klartext, Discord `allowed_mentions`, Slack-Escaping, ntfy JSON, E-Mail text/plain + RFC 2047) mit Tests | T-27 |
| M6 | mittel | SSE: Verbindungslimit HTTP/1.1, Widerruf, Puffer | **Angenommen.** Stream schließt bei verborgenem Tab, Sitzungsprüfung pro Heartbeat, kein gzip, `X-Accel-Buffering: no` | ADR-007 |
| M7 | mittel | PWA braucht HTTPS; CSP konkretisieren; zxcvbn zu groß | **Angenommen.** F-28 umformuliert, CSP festgelegt, Stärkeanzeige ohne zxcvbn | F-28, T-05 |
| M8 | mittel | Kontrastwerte teils < 4,5:1 | **Angenommen.** Farben korrigiert, Kontrasttest in CI | 05 §2 |
| M9 | mittel | SSDP-Antworten sind spoofbar | **Angenommen.** Abruf über Egress-Dialer, nie an eigene Host-IPs, 1 Abruf/Gerät/6 h, nur GET ohne Cookies | T-07 |
| M10 | mittel | Falsch-Positive bei IP-Konflikten | **Angenommen.** Konflikt nur, wenn zwei MACs auf *denselben* Request antworten, und erst bei Bestätigung im nächsten Zyklus | §4.2 |
| M11 | mittel | Befunde nicht speicher- und quittierbar | **Angenommen.** Tabelle `security_findings` mit „akzeptiert“; Portprüfung nur auf Anforderung oder per Zeitplan (opt-in) | F-21 |
| M12 | mittel | SQLite-Betrieb (Backup, Schreiblast) | **Angenommen.** Nächtliches `VACUUM INTO`, `last_seen` gepuffert, Sitzungs-Touch max. 1×/min, `auto_vacuum=INCREMENTAL`, Hinweis zu NFS/SMB | ADR-002 |
| M13 | mittel | MQTT-Client unterschätzt | **Angenommen.** Vollwertige Komponente (QoS 1, LWT, retained, `homeassistant/status`, TLS, Reconnect) mit Test gegen Mosquitto | F-25 |
| M14 | mittel | Scope zu groß, erwartete Funktionen fehlen | **Teilweise.** Kanäle bleiben (jeder ist klein und einzeln getestet). Import, Latenzhistorie und Netzwerkkarte auf v1.1 verschoben. FRITZ!Box-TR-064 als Should aufgenommen. Zusammenführen und Quittieren aller Geräte aufgenommen; Internet-Ausfälle werden nach Wiederherstellung gemeldet | 01 |
| M15 | mittel | Lizenzen eingebetteter Daten | **Bestätigt unkritisch:** IEEE-Daten über `oui-data` (BSD-2), DHCP-Regeln selbst erstellt, keine Fingerbank-/Wireshark-Daten | NOTICE |
| M16 | mittel | AF_PACKET-Last bei NAS-Durchsatz | **Angenommen.** Getrennte Sockets (`ETH_P_ARP`, `ETH_P_IP` + enger BPF), `PACKET_IGNORE_OUTGOING`, Filter vor `bind` | ADR-005 |
| L1 | niedrig | Cookies sind nicht pro Port isoliert | **Angenommen.** Absolute Sitzungsdauer 30 d, `__Host-`-Präfix bei TLS, dokumentiert | T-03 |
| L2 | niedrig | Zeitzone | **Angenommen.** Browser-Zeitzone wird beim Setup gespeichert | – |
| L3 | niedrig | Reset per `docker exec` ist für Laien schwer | **Angenommen.** Datei `reset-password` im AppData-Ordner → Einmalpasswort im Log | F-14 |
| L4 | niedrig | Multicast-Interface, ICMP | **Angenommen.** `IP_MULTICAST_IF` je Interface, ICMP-Raw-Socket vor dem Drop, Gateway-Prüfung vor WAN-Prüfung | – |
| L5 | niedrig | Einige ISO-Ziele nicht messbar | **Angenommen.** Ziele präzisiert, Nichtabstreitbarkeit als „eingeschränkt“ markiert | 04 |
| L6 | niedrig | Testlücken | **Angenommen.** Testfälle ergänzt | 06 |

**Bestätigt korrekt (keine Änderung):** BPF über `x/net/bpf` ohne cgo, XXE-Sicherheit von `encoding/xml`,
`modernc.org/sqlite` auf amd64/arm64, das Image-Größenziel.

---

## Review 2 · Code-, Security- und UX-Review (nach Implementierung)

**Ablauf:** Drei unabhängige Reviewer (Security, Code-Korrektheit, UX/Barrierefreiheit) haben den Code und eine laufende
Demo-Instanz erhalten, aber keine Einschätzung des Entwicklers. Jeder Befund wurde nachvollzogen (Test, Live-Angriff oder
Codepfad), dann behoben oder begründet abgelehnt. Jede Behebung hat einen Regressionstest, sofern technisch sinnvoll.

### 2.1 Security-Review

| ID | Schwere | Befund | Entscheidung / Behebung | Nachweis |
|---|---|---|---|---|
| S1 | mittel | Gespeicherte Geheimnisse (ntfy/Gotify-Token, Webhook-Header, SMTP-, MQTT-, FRITZ!Box-Passwort) ließen sich an ein **geändertes Ziel** umleiten, indem man das Ziel ändert und das maskierte Geheimnis zurückschickt. Live demonstriert. | **Behoben.** Maskierte/leere Geheimnisse werden nur übernommen, wenn Server, Host, Port, TLS und URL unverändert sind; sonst `secret_required`. Kanalwechsel auf eine andere Art erbt nie Geheimnisse. UI erklärt, was neu einzugeben ist. | `TestSecretsDoNotFollowNewDestination`, `TestChannelSecretsAreWriteOnly` (MQTT-Host-Wechsel) |
| S2 | mittel | Login-Bremse umgehbar: parallele Anfragen passierten alle die Prüfung, IP-Wechsel setzte das Limit zurück; Flut führte zu 503 für den echten Admin. | **Behoben.** Versuche werden vor der Passwortprüfung reserviert; zusätzliches Budget pro Benutzername (max. 60 s Sperre gegen Aussperren); Adressen mit früherem erfolgreichem Login haben Vorrang beim Hashing; `auth_failed` nur beim 1. und jedem 20. Fehlversuch protokolliert. Live nachgemessen: 320 parallele Fehlversuche aus 8 IPs → 10 geprüft, 310 abgewiesen; Admin-Login 200 in 29 ms. | `TestLimiterChargesInFlightAttempts`, `TestLimiterPerUser`, `TestPriorityGetsHashSlot` |
| S3 | mittel | Ersteinrichtung ohne Token ist 30 min lang aus dem LAN möglich. | **Abgelehnt, dokumentiert** (Restrisiko 8). Ein automatisches Token zwingt jeden Nutzer ins Container-Protokoll und bricht das Ziel „Compose einfügen → Passwort setzen“. Schutz: 30-min-Fenster, nur direkt angeschlossene Netze, optional `NETZWART_SETUP_TOKEN`, Werksreset. | ADR-010, Bedrohungsmodell |
| S4 | niedrig | SSDP-`LOCATION`-Prüfung beruht auf fälschbarer UDP-Absenderadresse. | **Akzeptiert, Doku korrigiert** (T-07, Restrisiko 7). Ergebnis ist nur ein GET ohne Zugangsdaten an ein LAN-Gerät, den der Angreifer auch selbst senden kann. | Bedrohungsmodell |
| S5 | niedrig | Egress-Richtlinie sperrt die LAN-IP des eigenen Hosts nicht. | **Abgelehnt, dokumentiert** (Restrisiko 9). Mosquitto, ntfy und Gotify laufen häufig auf demselben ZimaOS; Loopback bleibt gesperrt. | Bedrohungsmodell |
| S6 | niedrig | OUI-Workflow: Schreibrechte für den ganzen Workflow, Token in `.git/config`, Diff nicht prüfbar. | **Behoben.** Rechte nur im Job, `persist-credentials: false`, PR enthält Paketversion, Integritäts-Hash und lesbaren Diff. | `oui-update.yml`, actionlint |
| S7 | niedrig | Belegter Hash-Slot zählte beim Passwortwechsel als Fehlversuch. | **Behoben** (Refund statt Fehlversuch, 503). | Codepfad, `TestLimiter*` |
| S8 | trivial | T-19 nannte 30 s Bündelung, Code nutzt 20 s. | **Behoben** (Doku). | – |

Vom Reviewer als solide bestätigt: Host-/Origin-/CSRF-Prüfung, Sitzungen, SQL-Parametrisierung, Parser und Speichergrenzen,
Kodierung je Benachrichtigungskanal, Egress-Sperren, FRITZ!Box-TLS-Pinning, Maskierung in API und SSE, Rechteabgabe
(CapEff = 0), CSP ohne Inline-Skripte, CI-Supply-Chain (gepinnte SHAs/Digests).

### 2.2 Code-Review (Korrektheit)

| ID | Schwere | Befund | Behebung | Nachweis |
|---|---|---|---|---|
| C1 | hoch | Watchdog startete den Container endlos neu, wenn keine Schnittstelle gescannt wird (Subnetz > /22 oder falscher Name); ein hängender Scan wurde dagegen nie erkannt. | Watchdog nur aktiv, wenn tatsächlich gescannt wird; zusätzlich Erkennung hängender Scans. | Codepfad, Container-Test |
| C2 | mittel | Internet-Ausfall blieb offen, wenn die Prüfung währenddessen abgeschaltet wurde; später falsche „Internet wieder da“-Meldung über 48 h. | Abschalten schließt den Ausfall. | `TestDisableClosesOutage` |
| C3 | mittel | Ausgeblendete Sicherheitsbefunde kamen zurück (mit erneuter Meldung), wenn das Gerät einen Lauf lang offline war. | Bereinigung nur für geprüfte Geräte; ausgeblendete Befunde bleiben 30 Tage; abgebrochene Läufe bereinigen nichts. | `TestEventsAndFindings` |
| C4 | mittel | MQTT: Nach Broker-Wechsel oder Broker ohne Persistenz fehlten die Home-Assistant-Entitäten; nicht mehr beobachtete Geräte blieben nach Neustart als Entität stehen. | Discovery-Konfiguration pro Broker-Sitzung; zurückbehaltene Konfigurationen werden gelesen und veraltete entfernt. | `TestBridgeResyncsAfterBrokerChange` |
| C5 | mittel | Abgebrochener Portscan löschte die bekannten offenen Ports. | Scan läuft unabhängig von der Anfrage mit eigener Frist; unvollständige Ergebnisse werden verworfen. | Codepfad |
| C6 | niedrig | Rückwärts springende Uhr (NTP, Board ohne RTC) hielt Geräte „online“. | `last_seen` wird auf „jetzt“ begrenzt. | `TestClockJumpBack` |
| C7 | niedrig | Beim Herunterfahren abgebrochene Weck-Proben markierten Geräte als offline. | Abgebrochene Probe ist kein Beweis. | Codepfad |
| C8 | niedrig | Nach > 3 min Ausfallzeit meldeten alle beobachteten, weiterhin anwesenden Geräte „online“. | Geräte, die vor der Lücke online waren, lösen keine Meldung aus. | `TestGapRecovery` |

Vom Reviewer als solide bestätigt: Sperrreihenfolge ohne Zyklen, `flush`/`persist` per `MAX(last_seen)`,
keine verschachtelten Transaktionen, Paginierung, Zusammenführen/Löschen, Bündelung und Ratenlimit der Benachrichtigungen.

### 2.3 UX- und Barrierefreiheits-Review

| ID | Schwere | Befund | Behebung |
|---|---|---|---|
| U1 | hoch | Geräteliste auf dem Handy unbenutzbar (Namen auf ~20 px gequetscht). | Raster korrigiert; Regressionstest prüft die Namensbreite. |
| U2 | hoch | Auf dem Handy kein Abmelden. | Abmelden in den Einstellungen und an der aktuellen Sitzung. |
| U3 | hoch | Neue-Geräte-Meldungen an, aber kein Kanal → Meldungen gehen ins Leere; englische Validierungsfehler; keine Hilfe zu Token/Chat-ID. | Hinweis auf Übersicht und in den Einstellungen; zentrale Fehlerübersetzung; Hilfetext je Dienst; „Speichern & Test senden“. |
| U4 | hoch | Tastaturfokus im Detailpanel und Menü. | Fokus ins Panel und zurück zur Zeile; unter 1100 px modaler Dialog mit `inert`-Hintergrund; Escape schließt zuerst das Menü; Pfeiltasten; Fokus nach Dialogen zurück. Regressionstest. |
| U5 | hoch | Kontrast: Sekundärtext 3,9:1, Formularrahmen 1,5:1. | 4,5:1 bzw. 3:1; beide Paare im CI-Kontrastskript. |
| U6 | mittel | Begriffe „Neu / Unbestätigt / Bekannt / Akzeptieren“ uneinheitlich. | Ein Vokabular: „Unbekannt“, „Als bekannt markieren“, „Ausblenden“; „Neu“ nur für seit der Einrichtung aufgetauchte Geräte. |
| U7 | mittel | „Alle als bekannt markieren“ ohne Rückfrage. | Bestätigungsdialog (Übersicht und Einstellungen). |
| U8 | mittel | Umbenennen schwer auffindbar, kein Speicher-Feedback. | Stift-Symbol, „Gespeichert“-Rückmeldung. |
| U9 | mittel | Deutsch: Plural („1 unbestätigte Geräte“), „2 Warnung“, „TELNET offen“, Modellcodes („iPhone16,1“), „Hersteller: Unbekannt“ trotz Apple-Modell. | Pluralregeln im Übersetzer; verständliche Befund-Titel; Apple-Modellnamen; Hersteller aus Modell abgeleitet. |
| U10 | mittel | Offline-Status in Listen nur als Zeitangabe. | „Offline“ + Zeitangabe. |
| U11 | mittel | Zeitleiste mehrdeutig, Ticks springen bei Zeitumstellung. | Legende, Bereich „noch nicht bekannt“, „jetzt“-Marke, Ticks in Wanduhrzeit. |
| U12 | mittel | Deaktivierte Knöpfe ohne Begründung; Wake-on-LAN auch für Online-Geräte. | Hinweis „Im Demo-Modus nicht verfügbar“; WoL nur für Offline-Geräte, mit Erklärung. |
| U13 | mittel | Setup: Fehler doppelt, ohne Feldbezug; doppelter Sprachwähler; kein „Passwort anzeigen“. | Feldfehler mit `aria-describedby`, „Passwort anzeigen“, ein Sprachwähler; Wechsel zur Anmeldung, wenn das Setup anderswo abgeschlossen wurde. |
| U14 | mittel | Doppelte Überschriften in den Einstellungen; aktiver Reiter auf dem Handy außerhalb des Bildes. | Abschnittstitel nur für Screenreader; aktiver Reiter wird eingeblendet. |
| U15 | niedrig | Übersicht: Gerätetypen nehmen auf dem Handy viel Platz; Anmeldungen überladen „Letzte Aktivität“; Filter scrollen ohne Hinweis. | Typen als kompakte Chips; Anmeldungen nur im Aktivitätsprotokoll; Ausblendung am Rand. |

**Bewusst offen (Roadmap):** Gruppierung nach Personen, Ausfallhistorie als eigene Ansicht, Deep-Link zur FRITZ!Box-Kindersicherung,
eingeklappte Details bei Sicherheitsbefunden standardmäßig offen, größere `<summary>`-Klickflächen.
Speedtest bleibt bewusst weg (siehe [01](01-anforderungen.md#bewusst-nicht-übernommen)).

### 2.4 Eigene Befunde während Phase 3

| Befund | Behebung |
|---|---|
| `.box` stand in der Liste lokaler Domains, ist aber eine öffentliche TLD → DNS-Rebinding über `evil.box` möglich. | Entfernt, Test ergänzt. |
| TR-064: UPnP-Fehlercode 401 („Invalid Action“) wurde als Anmeldefehler gewertet, der Fallback für ältere FRITZ!OS lief nie. | Nur 606 gilt als Anmeldefehler; Test mit Fake-FRITZ!Box. |
| CI: lokales `gofmt` stammte aus einer älteren Go-Version. | CI nutzt das `gofmt` der Toolchain und zeigt den Diff als Annotation. |
| Test für das Datenverzeichnis war nur als root gültig. | Test prüft das tatsächliche Verhalten (Reparatur eigener Verzeichnisse). |
| UI-Tests richteten Desktop und Mobil parallel ein. | Serielle Ausführung; Setup-Seite wechselt zur Anmeldung. |

### 2.5 Nachweise zum Qualitätsmodell (ISO/IEC 25010)

Gemessen am 25.10.2026 mit dem Release-Kandidaten (Image `netzwart:local`, amd64).

| Merkmal / Teilmerkmal | Ziel | Gemessen | Status |
|---|---|---|---|
| Funktionale Korrektheit | Kernlogik getestet | 22 Pakete, alle Tests grün mit Race-Detector; Fuzzing der Parser (ARP, DHCP, mDNS, SSDP, NetBIOS, Text, TR-064) | erfüllt |
| Zeitverhalten | /24-Sweep < 10 s | 4,2 s (raw, `192.0.2.0/24`) | erfüllt |
| Zeitverhalten | API p95 < 50 ms bei 500 Geräten | Geräteliste + JSON bei **1000** Geräten: p50 2,2 ms, p95 5,2 ms; `/api/v1/devices` live p95 0,8 ms | erfüllt |
| Ressourcennutzung | RAM ≤ 60 MB, Image ≤ 40 MB | 25 MiB nach Scan (Docker), Image 23 MB | erfüllt |
| Kapazität | ≥ 1000 Geräte | 1000 Geräte laden in 56 ms, 1,4 MiB Heap; Obergrenze 4000 | erfüllt |
| Kompatibilität | amd64 + arm64 | Cross-Compile im Multi-Arch-Build (CI) | erfüllt |
| Erlernbarkeit | Einrichtung < 2 min | 2 Schritte, automatisiert in < 1 s durchlaufen; Nutzertest steht aus | teilweise |
| Bedienbarkeit | Tastatur, Touch, Maus | Fokusführung + `inert`, Regressionstests; Touch-Ziele der Navigation ≥ 44 px | erfüllt |
| Inklusivität | WCAG 2.2 AA | Kontrastskript (68 Paare, hell/dunkel) in CI; Feldfehler mit `aria-describedby` | erfüllt (automatisiert prüfbarer Teil) |
| Schutz vor Fehlbedienung | Bestätigung bei Massenaktionen | „Gerät vergessen“, „Alle als bekannt markieren“ mit Dialog | erfüllt |
| Fehlerfreiheit | 0 offene Defekte hoch/kritisch | alle Befunde aus Review 2 behoben; `staticcheck`, `go vet` sauber | erfüllt |
| Verfügbarkeit | Neustart nach Hänger | Watchdog (C1 korrigiert) + `restart: unless-stopped`, Healthcheck im Container-Test | erfüllt |
| Sicherheit | Bedrohungsmodell umgesetzt | 29 Bedrohungen, Container-Test: UID 10001, CapEff/CapPrm 0, NoNewPrivs, DB 0600; Live-Angriffe S1/S2 nach Fix wirkungslos | erfüllt |
| Testbarkeit | Coverage ≥ 60 % Kernpakete | monitor 63 %, auth 88 %, config 86 %, proto 84 %, egress 83 %, mqtt 72 %, fritzbox 70 %; store 49 %, api 48 %. Hardwarenahe Pakete (`l2`, `scanner`, `privdrop`) werden über den Container-Test abgedeckt | teilweise |
| Installierbarkeit | ein Compose-Block | Container-Test mit Compose-Flags, 7 Szenarien (Bind-Mount root, fremder Eigentümer, ohne Volume, PUID, ohne NET_RAW, `--user`) | erfüllt |

## Review 3 · Design-Review (nach dem Redesign „Leitstand“)

Auslöser: Rückmeldung „sieht aus wie die nächste Vibecoding-App“. Das Frontend wurde mit der Designsprache „Leitstand“ neu gestaltet (siehe [05 · UX-Design](05-ux-design.md), Abschnitt 0). Danach hat ein unabhängiger Reviewer die laufende Demo in drei Runden mit Screenshots bei 1440, 1024 und 390 px in Hell und Dunkel bewertet. Er kannte den Code nicht und hat das Repo nicht verändert.

| Runde | Note (1–10) | Wichtigste Befunde | Behebung |
|---|---|---|---|
| 1 | 6 | Aktivitätskasten leer (die letzten Ereignisse waren nur Anmeldungen), Kopfleiste springt zwischen Seiten, graue deaktivierte Buttons, abgeschnittene Fokusringe, blaue Links, zu viele Schriftgrößen, doppelte Linien, Sitzungen als Karten | Feed lädt gezielt Ereignisarten (`overviewKinds`), feste Kopfleiste, Typo-Skala 11/13/14/16/20/30, Linkfarbe = Textfarbe, Sitzungen als Zeilen |
| 2 | 7 | N1–N12: gestrichelte Buttons, zu viele Unterstreichungen, Einstellungen ohne Satzspiegel, Herzschlag-Semantik, „Online“ in der Kompaktliste, Fokus überdeckt Text, schwacher Dialog-Hintergrund im Dunkelmodus, Login-Ring ungruppiert | Explizite Button-Zustände („Keine Änderungen“, Hinweistext statt deaktiviert), Unterstreichung nur in Fließtext, 680-px-Spalte, Tooltip „Durchgehend online (24 h)“ und 6-h-Marken, gruppierter Login-Ring |
| 3 | 8 | P1 relative Zeiten überlagern den Text im Aktivitäts-Feed; P2 Ausfallmarke der Sparkline; P3 Fokus und Auswahl der Filter-Tabs kaum unterscheidbar; P4 Wiederholungsfeld beim Setup; P5 Schusterjunge im Geräte-Intro; P6 Sicherheitszeile auf dem Handy | siehe unten |

Behebung der Runde 3:

| Befund | Behebung |
|---|---|
| P1 Zeiten im Feed | Spalte zeigt immer `HH:MM`, darunter „Gestern“ oder „Sa., 24.“; feste Breite `8ch`, `nowrap`. |
| P2 Sparkline | Daten geprüft: Der Ausfall lag tatsächlich am Anfang des Fensters, die Marke war korrekt. Zusammenhängende Ausfälle werden jetzt als ein Segment in voller Länge gezeichnet und bleiben innerhalb der Grafik. |
| P3 Fokus vs. Auswahl | Auswahl = Unterstrich, Fokus = 2-px-Umriss mit Abstand. Scrollende Tab-Leisten reservieren 4 px, damit der Umriss nicht abgeschnitten wird. |
| P4 Setup | Nicht reproduzierbar (auch mit sofortigem Ausfüllen beider Felder). Trotzdem gehärtet: Nach dem Absenden gilt ein leeres Wiederholungsfeld als Abweichung und zeigt „Passwörter stimmen nicht überein“, statt stumm nichts zu tun. |
| P5 Geräte-Intro | Text gekürzt: „Die Streifen zeigen die letzten 24 Stunden.“ |
| P6 Handy-Sicherheitszeile | Unter 480 px steht „Geprüft vor …“ in einer eigenen Zeile unter den Zahlen. |

Offen und bewusst zurückgestellt: Sitzungen werden weiterhin einzeln gelistet (mit „Alle anderen abmelden“). Eine Gruppierung nach Gerät folgt, wenn Sitzungen einen stabilen Gerätenamen tragen.

Nachweis: svelte-check 0 Fehler/0 Warnungen, Kontrastskript 68 Paare ok, 14 UI-Tests (Desktop und Mobil) grün, README-Screenshots neu erstellt.

## Review 4 · Code- und Security-Review des Redesigns

Umfang: alle Änderungen seit Review 2 (`f170c01..3ba1553`), also das neue Frontend und der Endpunkt `GET /api/v1/presence/strips`. Zwei unabhängige Reviewer haben parallel geprüft, einer auf Sicherheit, einer auf Korrektheit. Sie haben die Demo-Instanz mit curl und Playwright angegriffen bzw. bedient und das Repo nicht verändert.

### 4.1 Security-Review

Ergebnis: keine kritischen, hohen oder mittleren Befunde. Ohne Befund geprüft:
- Zugriffsschutz des neuen Endpunkts (401 ohne Sitzung, kein CORS, `CORP: same-origin`, `no-store`) und parametrisiertes SQL.
- XSS: keine `{@html}`-/`innerHTML`-Senken; Gerätenamen gelangen nur escaped in SVG-`<title>` und `aria-label`.
- CSP: unverändert; Schriften liegen selbst gehostet unter `/assets`.
- CSRF-Token bei allen ändernden Aufrufen.
- Die erzeugten Icons sind byte-identisch reproduzierbar.

| Befund | Schwere | Behebung |
|---|---|---|
| „Alle anderen abmelden“ löschte Sitzungen einzeln aus einer beim Seitenaufruf geladenen Liste und brach beim ersten 404 stumm ab. Spätere oder neue Sitzungen blieben gültig (schwächt T-03). | niedrig | Neuer Endpunkt `POST /api/v1/auth/sessions/revoke-others`: ein `DELETE` für alle anderen Sitzungen, deren Live-Verbindungen werden geschlossen. Die Seite meldet die Anzahl oder den Fehler. Test `TestRevokeOtherSessions`. |
| Der Abruf der Herzschlag-Streifen lief alle 5 min auch in versteckten Tabs und nach dem Abmelden. Ein vergessener Tab hielt so die Sitzung über das 7-Tage-Leerlauflimit hinaus am Leben. | niedrig | Kein Abruf bei verstecktem Tab oder ohne Anmeldung; Timer und Daten werden bei Abmeldung und 401 gelöscht. |
| Die Streifen-Abfrage durchsuchte die ganze Anwesenheitstabelle. | Info | Migration 2: Index `presence(ended_at)`. Ohne `ORDER BY` in SQL nutzt SQLite ihn per MULTI-INDEX OR (Query-Plan geprüft). |

### 4.2 Code-Review (Korrektheit)

Die Slot-Berechnung im Backend wurde mit einem eigenen Test bestätigt: Fensterränder, Spannen über den Fensteranfang, Ende genau auf einer Slotgrenze, offene Spannen und Lücken.

| Befund | Schwere | Behebung |
|---|---|---|
| Einstellungsseiten verglichen Eingaben mit den Werten beim Seitenaufruf statt mit den gespeicherten. Nach dem Speichern und Zurückändern verschwand „Speichern“, obwohl der Server den anderen Wert hatte. Der Fehler war älter; das Redesign hat ihn sichtbar gemacht. | hoch | Alle fünf Seiten vergleichen mit `$derived(store.settings)`. Neuer UI-Test „saved values become the new baseline“, der ohne die Korrektur fehlschlägt. |
| Die Aktivitätsliste der Übersicht lud nur bei neuen Live-Ereignissen nach. Nach verstecktem Tab oder Neuverbindung fehlten Ereignisse. | hoch | `loadAll()` stößt das Nachladen ebenfalls an (`eventTick`); davon profitieren auch Aktivitätsseite und Gerätedetail. |
| Viele Ereignisse auf einmal lösten parallele Anfragen aus; eine ältere Antwort konnte die neuere überschreiben. | mittel | 300 ms Bündelung und Sequenznummer: nur die jüngste Antwort zählt. |
| Timer der Streifen nach dem Abmelden | mittel | wie 4.1 |
| Die Live-Markierung konnte nach einem Slotwechsel den falschen Slot setzen oder durch eine laufende Abfrage überschrieben werden. | niedrig | Nach Ablauf des Fensters werden die Streifen neu geladen; nach jeder Abfrage wird der Live-Status erneut angewendet; keine parallelen Abfragen. |
| Bei Totalausfall zeigte die Internet-Kurve nichts. | niedrig | Ausfallsegmente werden auch ohne Messwerte gezeichnet. |
| Netzring: Punkte überlappten ab ca. 120 Geräten. | niedrig | Punkt- und Klickradius richten sich nach dem Abstand der Nachbarn. |
| Beschriftung „1 Stunden“ bzw. „0.5 Stunden“ | niedrig | Pluralform und deutsches Zahlenformat („0,5 Stunden“). |
| Zwei direkt aneinandergrenzende Überwachungslücken ließen den gemeinsamen Slot als „offline“ statt „unbekannt“ erscheinen. Dazu kam eine nie wahre Bedingung. | niedrig | Lücken werden vor der Prüfung zusammengeführt; die tote Bedingung ist entfernt. `TestPresenceStripsEdges` deckt alle Randfälle ab und schlägt ohne die Korrektur fehl. |

Nachweis: `go vet` und `go test -race ./...` grün, svelte-check 0/0, 16 UI-Tests (Desktop und Mobil) grün.

## Review 5 · Plan-Review „Software und Support“ (0.1)

Auslöser: Wunsch nach Software-Versionen je Gerät („so wirklich einen Nutzen habe ich daraus nicht“). Bevor programmiert wurde, hat ein unabhängiger Reviewer den Plan [08 · Software und Support](08-software-und-support.md) auf technische Machbarkeit, Richtigkeit der Regeln, Sicherheit, Nutzen und Release-Prozess geprüft. Die Fakten der Wissensbasis wurden am 26.10.2026 per Websuche recherchiert und vom Reviewer stichprobenartig gegengeprüft.

**Urteil:** bereit mit Änderungen. Der Umfang war richtig gewählt: Support-Ende statt CVE-Abgleich, und ehrlich zu iPhones.

Die zwölf Pflichtänderungen und ihre Umsetzung stehen in [08 §11](08-software-und-support.md#11-änderungen-nach-dem-plan-review-review-5). Die wichtigsten:
- Samba/NAS-Platzhalter dürfen nie als „Windows 7“ erscheinen.
- Gemeinsame Build-Nummern von Client, LTSC und Server brauchen eine Ausgabe-Einstellung.
- Die tvOS-Regel wurde korrigiert.
- Die Bewertung läuft unabhängig vom opt-in-Sicherheitscheck.
- Vor jeder Migration entsteht eine Sicherung.
- `latest` steht nur für Releases.
- Der GitHub-Release entsteht im CI.

## Review 6 · Code- und Security-Review von 0.1

Umfang: `22c75ba..63918eb` (Software und Support, Versionsabfrage, Release-Prozess). Zwei unabhängige Reviewer haben parallel geprüft, einer auf Sicherheit, einer auf Korrektheit, mit Wegwerf-Tests in einer Kopie des Repos und gegen die Demo-Instanz.

### 6.1 Security-Review

Ergebnis: nichts Kritisches oder Hohes. Ohne Befund geprüft:
- Probes: nie AUTHENTICATE oder Zugangsdaten, alle Längen und Offsets geprüft, Timeouts.
- RE2-Regex.
- Bereinigung der Rohwerte.
- Whitelist der TXT-Schlüssel.
- Versionsabfrage: aus per Voreinstellung, feste URL, keine Weiterleitungen, Link geprüft.
- FRITZ!Box: keine Seriennummer.
- CSRF auf dem neuen Endpunkt.
- Kein `{@html}`.
- Release-Job mit Tag in Umgebungsvariable.

| Befund | Schwere | Behebung |
|---|---|---|
| Gefälschte passive Angaben im Namen **anderer** Geräte: ein altes iPhone wirkte durch gespoofte AirPlay-Version „aktuell“, eine NAS durch gespoofte SSDP-Kennung „FRITZ!OS 6, veraltet“; wechselnde Produkte erzeugten bei jeder Bewertung neue Befunde | mittel | Plausibilitätsprüfung je Produkt; Apple-Version über der Modellgrenze ⇒ Widerspruch, Modell entscheidet; mDNS-Versionen nur für eigene Dienste (Sleep-Proxy); Befund-Identität nur noch Status; T-30 korrigiert |
| `PUT …/software` speicherte die gültige Hälfte einer teilweise ungültigen Anfrage | niedrig | Erst alles prüfen, dann speichern; Test |
| Sicherung vor dem Update blieb unbegrenzt liegen und überstand das Zurücksetzen auf Werkseinstellungen (enthält alte Geheimnisse) | niedrig | Werksreset löscht alle Sicherungen; `pre-update-schema-*` wird nach 30 Tagen entfernt |
| Fuzzer der neuen Parser liefen nicht im CI | niedrig | Paket in `make fuzz`, zusätzliche Fuzzer für TXT und DHCP |
| mDNS-TXT von Sleep-Proxys wurde dem Proxy zugeordnet | niedrig | SRV-Ziel muss zur Absenderadresse passen; Test |
| FRITZ!OS konnte beim Hostnamen statt IP einem fremden Gateway zugeordnet werden; Versionsabfrage lief auch im Demo-Modus; Release-Checkout behielt das Schreib-Token | Info | Zuordnung nur zu AVM-Routern; Demo ohne Abfrage; `persist-credentials: false`; fail2ban-Hinweis in T-32 |

### 6.2 Code-Review

| Befund | Schwere | Behebung |
|---|---|---|
| Zusammenführen von Geräten verlor Software-Angaben im Speicher (erst nach Neustart sichtbar), Bewertung lief nicht neu | hoch | Zeilen werden übernommen (neuere gewinnt), Neubewertung angestoßen; Test |
| Windows-Ausgabe „Enterprise/LTSC/Server“ war ein Topf: Enterprise 1809/21H2 fälschlich unterstützt, LTSC 2024/Server 2025 und Enterprise 23H2/25H2 mit falschen Enddaten | mittel | Drei Ausgaben (Home/Pro, Enterprise/Education, LTSC/Server) mit eigenen Enddaten je Build |
| Debian 12 blieb nach LTS-Ende „unterstützt“ | mittel | Jedes „unterstützt bis“ wird gegen das aktuelle Datum geprüft |
| Veraltete Beobachtung verdeckte das Urteil aus dem Modell | mittel | Modell entscheidet, „zuletzt erkannt“ bleibt sichtbar |
| Fehlgeschlagene Sicherung vor dem Update blockierte jeden Start; Teilkopie hätte als gültig gegolten | mittel | Schreiben unter Temporärnamen, Umbenennen bei Erfolg; Fehler wird protokolliert, Start geht weiter; Container-Test verlangt die Sicherung, wenn das Schema steigt |
| FRITZ!OS „unterstützt“, wenn die Update-Abfrage scheiterte | niedrig | Ohne Update-Status keine Beobachtung |
| MacBook Air 2018/2019 (max. macOS 14), iMac Pro fehlten | niedrig | Ergänzt |
| Viele Auslöser konnten die Bewertung immer weiter verschieben | niedrig | Frist wird nur vorgezogen, nie verschoben |
| Vorabversionen wären als „neuestes Release“ erschienen | niedrig | `--prerelease` für Tags mit `-` |
| FRITZ!OS-Laborversionen, Ubuntu 26.04 fehlten | niedrig | Ergänzt (OpenSSH 10.2p1 ⇒ 26.04) |

Nachweis: `go vet`, `go test -race ./...`, `svelte-check` 0/0, UI-Tests, Fuzzing der neuen Parser im CI.

## Review 7 · Plan-Review Version 0.2

Ein unabhängiger Reviewer hat den Plan [09 · Version 0.2](09-version-0.2.md) gegen den bestehenden Code geprüft. **Urteil:** freigegeben mit Änderungen.

Die Pflichtänderungen und ihre Umsetzung stehen im Abschnitt „Entscheidungen nach dem Plan-Review“. Die wichtigsten:
- **Bericht:** Für den Provider-Bericht fehlten gespeicherte Latenzwerte und die Art des Ausfalls.
- **Router-MAC:** Die Prüfung nutzt die vorhandene Konfliktlogik, und MACs sind nie Teil der Befund-Identität.
- **WLAN:** Die FRITZ!Box meldet Signal als RCPI. Die Regel „niedrige Datenrate“ hätte dauernd angeschlagen und entfällt.
- **Lücken-Abgleich:** Er läuft je Versionszweig.
- **Personen:** Die Funktion bleibt bei der Anwesenheit; die Zeit pro Tag ist nur nach Zustimmung einschaltbar.
- **Verhaltens-Baseline:** Sie wurde verkleinert, um Fehlalarme zu vermeiden.
