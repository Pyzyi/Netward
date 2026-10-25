# 03 · Bedrohungsmodell (STRIDE)

> Rolle: Security Engineer · Status: **überarbeitet nach Plan-Review 1** (siehe [07](07-review-protokoll.md))

## 1. Schutzgüter

| Schutzgut | Warum schützenswert |
|---|---|
| Geräteinventar und Anwesenheitsdaten | Verrät, wer wann zuhause ist (Einbruchsrisiko, Privatsphäre) |
| Zugangsdaten (Admin-Passwort, Sitzungen) | Vollzugriff auf die Anwendung |
| Geheimnisse der Benachrichtigungskanäle | Bot-Tokens, SMTP-Passwörter, MQTT-Zugang |
| Host-System | Der Container läuft im Host-Netzwerk |
| Netzwerkbetrieb | Scans dürfen das Netz nicht stören |

## 2. Vertrauensgrenzen

```
 [Internet] ─ ─ ─ (Router/NAT) ─ ─ ─ [LAN: Geräte, ggf. feindlich] ─ ─ [Host ZimaOS] ─ ─ [Container: Netzwart]
                                              │                                                │
                                              └── Pakete (ARP/DHCP/mDNS/SSDP/NetBIOS) ────────▶│  TG-1: untrusted network input
 [Browser des Admins] ───────────── HTTP(S) ──────────────────────────────────────────────────▶│  TG-2: Web-API
 [Benachrichtigungsdienste, MQTT] ◀───────────────────────────────────────────────────────────│  TG-3: ausgehend
```

Annahme: **Jedes Gerät im LAN kann bösartig sein**. Es kontrolliert seine Hostnamen, mDNS-, SSDP- und
NetBIOS-Antworten und kann beliebige Pakete senden.

## 3. Bedrohungen und Maßnahmen

| # | STRIDE | Bedrohung | Maßnahme | Verifikation |
|---|---|---|---|---|
| T-01 | S | Fremder übernimmt die Ersteinrichtung | Setup nur ohne bestehenden Benutzer (atomar), nur ≤ 30 min nach Start, nur aus direkt angeschlossenem Subnetz, Host-/Origin-Allowlist; optionales `NETZWART_SETUP_TOKEN`; `factory-reset` entfernt jede Angreifer-Konfiguration | API-Test |
| T-02 | S | Brute-Force auf Login | Argon2id (19 MiB, t=2, p=1, OWASP); exponentieller Backoff pro IP bis 15 min; genau 1 Hash-Slot mit 3-s-Warteschlange (sonst `503`); gleiche Arbeit bei unbekanntem Benutzer (Dummy-Hash) | Unit-Test Limiter |
| T-03 | S | Sitzungsdiebstahl | 256-bit-Zufallstoken, nur Hash in DB, `HttpOnly`, `SameSite=Strict`, `__Host-`/`Secure` bei TLS, Leerlauf 7 d (30 d mit „angemeldet bleiben“), absolute Lebensdauer 30 d, Widerruf einzeln; Passwortänderung widerruft andere Sitzungen | API-Test |
| T-04 | T | CSRF auf zustandsändernde Endpunkte | `SameSite=Strict` + `X-CSRF-Token` (pro Sitzung, konstante Vergleichszeit) + Pflicht-`Origin` aus Allowlist + `http.CrossOriginProtection`. Auch Setup und Login sind geschützt | API-Test |
| T-05 | T | **Stored XSS über Gerätenamen** (DHCP-Hostname, mDNS, SSDP friendlyName) | Normalisierung beim Einlesen (Steuerzeichen raus, Länge ≤ 128); Svelte escapt standardmäßig, `{@html}` ist verboten (CI-Grep); CSP: `default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; manifest-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'` | Unit-Test + CI-Grep + E2E-CSP-Test |
| T-06 | T | Manipulierte Netzwerkpakete bringen Parser zum Absturz (DoS) oder lesen über Speichergrenzen | Go ist speichersicher; alle Parser prüfen Längen und laufen durch **Fuzz-Tests**; `recover` pro Paket im Listener | Fuzzing in CI |
| T-07 | T/I | **SSRF über SSDP `LOCATION`** (Antworten sind spoofbar) | Host muss IP-Literal = Absender-IP im Subnetz sein, nie eine eigene Host-IP (die Absender-IP ist per UDP fälschbar: ein LAN-Angreifer kann damit einen GET an einen beliebigen Port eines LAN-Geräts auslösen; akzeptiert, siehe Restrisiko 7); nur `http` + GET ohne Cookies/Auth; keine Redirects; 64 KiB; 3 s; max. 1 Abruf/Gerät/6 h; Egress-Dialer (T-25) | Unit-Test |
| T-08 | T | XXE / XML-Bomben in UPnP-Beschreibung | Go `encoding/xml` löst keine externen Entities auf; Größenlimit 64 KiB | Unit-Test |
| T-09 | T | CSV-Formel-Injektion beim Export (`=HYPERLINK(...)` als Hostname) | Felder, die mit `= + - @ \t \r` beginnen, werden mit `'` präfixiert | Unit-Test |
| T-10 | T | Log-Injektion über Hostnamen | Strukturiertes Logging (`slog`), Steuerzeichen werden entfernt | Code-Review |
| T-11 | R | Abstreitbarkeit von Admin-Aktionen | Ereignisprotokoll für Login (Erfolg/Fehlschlag), Passwortänderung, Einstellungen, Kanaländerungen | Code-Review |
| T-12 | I | Geheimnisse der Kanäle werden über die API ausgelesen | Geheimnisfelder sind write-only: Die API liefert nur `"••••"`/„gesetzt“; leeres Feld beim Speichern bedeutet „unverändert“ | API-Test |
| T-13 | I | Datenbank fällt in falsche Hände | `/data` 0700, DB 0600, Laufzeit-UID 10001; Export enthält keine Geheimnisse. Keine Verschlüsselung at-rest, weil der Schlüssel auf demselben Datenträger läge (Scheinsicherheit) | Code-Review |
| T-14 | I | Informationsabfluss an Dritte | Keine Telemetrie, keine CDN-Ressourcen (Schriften/Icons lokal), OUI-DB offline; Internet-Monitor abschaltbar und mit dokumentierten Zielen | CSP `connect-src 'self'` |
| T-15 | I | Clickjacking | `frame-ancestors 'none'`, `X-Frame-Options: DENY` | E2E-Header-Test |
| T-16 | D | Scan-Flut stört das Netz | ARP ≤ 500 pps; Portscan max. 32 parallele Verbindungen, 0,8 s Timeout, nur TCP-Connect ohne Nutzdaten; Subnetze > /22 nur nach expliziter Freigabe; Fallback-Sweep max. /24 (Nachbartabelle des Hosts) | Unit-Test |
| T-17 | D | Paketflut auf passivem Listener | Getrennte Sockets, Kernel-BPF-Filter (ARP; UDP 67/68/5353), `PACKET_IGNORE_OUTGOING`, begrenzte Queue mit Verwerfen, Ratenlimit pro MAC | Unit-Test |
| T-18 | D | Große/viele API-Requests | Body-Limit 1 MiB, `ReadHeaderTimeout`, `ReadTimeout`, `IdleTimeout`; begrenzte SSE-Clients (32) | Code-Review |
| T-19 | D | Benachrichtigungs-Spam (z. B. viele neue Geräte) | Lernphase beim ersten Scan; Bündelung innerhalb 20 s (ab 4 Geräten eine Sammelmeldung); Ratenlimit 30 Meldungen / 10 min pro Kanal | Unit-Test |
| T-20 | E | Container-Ausbruch / Rechteausweitung | Nicht-root (UID 10001), zur Laufzeit **CapEff = 0** (Sockets vor dem Drop geöffnet), `no-new-privileges` (zusätzlich vom Binary selbst gesetzt), read-only Root-FS, `scratch`-Image ohne Shell und Paketmanager | CI-Test prüft `/proc/1/status` |
| T-21 | E | Verwundbare Abhängigkeiten | 4 direkte Go-Abhängigkeiten; `govulncheck`, `npm audit`, Trivy-Scan, Dependabot | CI |
| T-22 | E | Manipuliertes Image (Supply Chain) | Build ausschließlich in GitHub Actions, SBOM + Provenance-Attestierung, gepinnte Actions (Commit-SHA) | CI |
| T-23 | S | Gefälschte `X-Forwarded-For`-Header umgehen IP-Drosselung | Weitergeleitete Header nur von `NETZWART_TRUSTED_PROXIES` | Unit-Test |
| T-24 | T | **DNS-Rebinding** gegen die Web-Oberfläche (Angreifer kontrolliert Host *und* Origin) | Host-Allowlist für jeden Request (`421` sonst), Origin-Allowlist für unsichere Methoden, `NETZWART_ALLOWED_HOSTS` für Reverse-Proxies (ADR-012) | API-Test |
| T-25 | E | **SSRF auf Host-Loopback** über Webhook/Kanaltest (`127.0.0.1` = Host, dort CasaOS/ZimaOS-Dienste, ggf. Docker-API) | Zentraler Egress-Dialer verbietet Loopback, Unspecified, Link-Local, Multicast beim Connect (wirkt auch nach DNS/Redirect); Tests liefern nur Statuscode (ADR-011) | Unit-Test |
| T-26 | D | Geräteflut durch zufällige MACs (macof) oder rotierende private MACs | Geräte nur aus ARP-Sichtungen im Subnetz; Aufnahme-Budget 20/min, Obergrenze 4 000, ein `device_flood`-Ereignis; unbekannte Geräte mit privater MAC verfallen nach 14 Tagen offline | Unit-Test |
| T-27 | T | Injektion über Gerätenamen in Benachrichtigungen (Telegram-Markup, `@everyone`, E-Mail-Header, MQTT-Topics) | Encoder je Kanal: Klartext, `allowed_mentions: {parse: []}`, Slack-Escaping, ntfy-JSON, RFC 2047, MQTT-IDs nur aus MAC | Unit-Test je Kanal |
| T-28 | I | Datenabfluss über vom Angreifer angelegte Kanäle bleibt nach Passwort-Reset bestehen | `factory-reset`; Kanaländerungen im Ereignisprotokoll | Code-Review |
| T-29 | S/I | **FRITZ!Box-Zugangsdaten** werden von einem Gerät im LAN abgegriffen, das sich als FRITZ!Box ausgibt (ARP-/DNS-Spoofing) | Authentifizierte TR-064-Aufrufe nur über HTTPS; Zertifikat wird beim ersten erfolgreichen Login gepinnt (SHA-256), Abweichung bricht vor dem Senden von Zugangsdaten ab; Digest-Auth statt Klartext; über HTTP läuft nur die unauthentifizierte Port-Abfrage; Host-Listen-Pfad muss relativ sein (kein SSRF); Egress-Dialer; eigener FRITZ!Box-Benutzer empfohlen | Unit-Test (Fake-FRITZ!Box mit TLS) |

## 4. Restrisiken (akzeptiert)

1. **Cookies sind nicht pro Port isoliert:** Andere Web-Apps auf demselben Host erhalten das Sitzungs-Cookie.
   Origin-Prüfung und CSRF-Token schützen vor Missbrauch, eine bösartige App auf demselben Host bleibt aber ein Restrisiko.
2. **Unverschlüsseltes HTTP im LAN** (Standard). Begründung: ZimaOS-Apps laufen typischerweise per HTTP im LAN.
   Abhilfe: HTTPS über `NETZWART_TLS_CERT`/`_KEY` oder einen Reverse-Proxy. Die Oberfläche weist darauf hin.
3. **Setup-Race im LAN** innerhalb des 30-min-Fensters (siehe T-01, ADR-010).
4. **Host-Netzwerk** ist für Layer-2-Scans technisch zwingend. Fing Agent und NetAlertX haben dieselbe Anforderung.
5. **MAC-Spoofing**: Ein Angreifer kann die MAC eines bekannten Geräts annehmen. Netzwart erkennt das nur
   indirekt (IP-Konflikt, Hersteller- oder Namenswechsel). Kein Layer-2-Tool kann das vollständig verhindern.
6. **Trust on first use bei der FRITZ!Box** (T-29): Wer beim allerersten Verbindungsaufbau bereits erfolgreich
   die FRITZ!Box imitiert, erhält den Digest-Hash. Die FRITZ!Box bietet kein CA-signiertes Zertifikat im LAN;
   empfohlen wird daher ein eigener FRITZ!Box-Benutzer mit eigenem Passwort.
7. **SSDP-Antworten mit gefälschtem Absender** (T-07): Ein Gerät im LAN kann Netzwart zu einem einfachen GET ohne
   Cookies/Zugangsdaten an ein anderes LAN-Gerät veranlassen. Das kann derselbe Angreifer auch direkt; die Antwort
   wird nur als Gerätebeschreibung (max. 64 KiB, bereinigt) gespeichert.
8. **Ersteinrichtung ohne Token** (T-01): Bewusst kein automatisch erzeugtes Setup-Token, weil der Einrichtungsweg
   „Compose einfügen → Passwort setzen“ sonst den Blick ins Container-Protokoll erfordert. Wer das Risiko nicht
   tragen will, setzt `NETZWART_SETUP_TOKEN`.
9. **Eigene Host-Adressen sind als Ziel erlaubt**: Benachrichtigungen, MQTT und Webhooks dürfen die LAN-IP des Hosts
   erreichen, weil Mosquitto, ntfy oder Gotify häufig auf demselben ZimaOS laufen. Gesperrt bleibt Loopback.

## 5. Sicherheitsrelevante Standards

- OWASP ASVS 4.0.3 Level 1 (Auth, Session, Access Control, Validation, Config) als Checkliste im Review
- CIS Docker Benchmark Abschnitt 4/5 (Image, Container-Laufzeit)
- OWASP Password Storage Cheat Sheet (Argon2id-Parameter)
