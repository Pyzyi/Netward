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
| T-01 | S | Fremder übernimmt die Ersteinrichtung | Setup nur ohne bestehenden Benutzer (atomar), nur ≤ 30 min nach Start, nur aus direkt angeschlossenem Subnetz (über einen Reverse-Proxy nur, wenn er in `NETZWART_TRUSTED_PROXIES` steht, und dann mit der Adresse des eigentlichen Clients), Host-/Origin-Allowlist; optionales `NETZWART_SETUP_TOKEN`; `factory-reset` entfernt jede Angreifer-Konfiguration | API-Test |
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
| T-30 | S | **Gefälschte Versionsangaben** (mDNS, SSDP, DHCP und SSH sind spoofbar, auch im Namen eines *anderen* Geräts) führen zu falscher Entwarnung oder falschem Alarm | Plausibilitätsprüfung je Produkt (FRITZ!OS nur für AVM-Geräte oder das Gateway, DSM nur für Synology, Apple-Systeme nur für Apple- oder private MACs, Android nie für Apple/AVM/Synology); eine Apple-Version über dem, was das Modell installieren kann, gilt als Widerspruch und das Modell entscheidet; mDNS-Versionen nur für eigene Dienste des Absenders (kein Sleep-Proxy); aktive Proben und TR-064 gehen passiven Ankündigungen vor; widersprechende Quellen werden angezeigt; Angaben älter als 90 Tage entscheiden nichts; Befund-Identität ohne Produkt und Version, damit wechselnde Fälschungen keine Benachrichtigungsflut auslösen; Quelle und Rohwert sind immer sichtbar; Netzwart handelt nie automatisch | Unit-Test |
| T-31 | T/D | Parser-Fehler bei feindlichen Eingaben (SSH-Zeile, NTLM-Antwort, TXT, SERVER-Header, DHCP-Option 60) | Reine Funktionen mit Längengrenzen und verankerten Regex (RE2, kein Backtracking); SMB-Antwort max. 4 KiB, NTLM-Feld mit Bereichsprüfung; SSH max. 1 KiB; 3 s Timeout; Fuzz-Tests | Fuzzing in CI |
| T-32 | D | Aktive Proben stören Geräte oder lösen Sperren aus | SSH: es wird nichts gesendet; SMB: nur NEGOTIATE und die erste SESSION_SETUP-Nachricht ohne Benutzer, die Challenge wird nie beantwortet (keine Anmeldung, kein Sperrzähler); nur im Sicherheitscheck (auf Anforderung oder opt-in-Zeitplan), 4 Geräte parallel; bekannte Nicht-Windows-Geräte werden ausgelassen. Hinweis: sshd protokolliert die Probe als Verbindung ohne Kennung („Did not receive identification string“); fail2ban reagiert darauf nur im Modus `aggressive`/`ddos` | Unit-Test (Fake-SMB) |
| T-33 | I | Sensible Daten in TXT-Records oder TR-064-Antworten (Seriennummer, Geräte-ID, Ereignisprotokoll) | Positivliste der übernommenen Schlüssel je Dienst; von TR-064 `GetInfo` nur `NewSoftwareVersion`; Rohwerte bereinigt und auf 120 Zeichen gekürzt | Unit-Test |
| T-34 | I | Versionsabfrage verrät die Nutzung an GitHub | Standardmäßig aus (N-04); fester Ziel-URL, Egress-Client, keine Kennung/Cookies; nur `tag_name` und `html_url` übernommen, Link muss mit `https://github.com/Pyzyi/netzwart/releases/` beginnen; Antwort max. 64 KiB | Unit-Test |
| T-35 | D | Die **aktive DHCP-Prüfung** (0.2) stört das Netz oder belegt Adressen | Nur im Sicherheitscheck und abschaltbar (`dhcpCheck`); ein einziger DHCPDISCOVER mit Broadcast-Flag, nie ein REQUEST (es wird keine Adresse vergeben); eine feste, lokal verwaltete Client-MAC pro Installation, die nie zu einem Gerät wird; Absender-MAC ist die eigene Schnittstelle; Antworten 3 s lang gesammelt, nur mit passender Transaktions-ID und Client-MAC, höchstens 16; ein Probe je Schnittstelle gleichzeitig | Unit-Test (Pipe statt Socket) |
| T-36 | S | **Gefälschte DHCP-Angebote oder ARP-Antworten** erzeugen Fehlalarme oder verdecken einen echten Angriff | Router-Konflikt erst nach zwei Sweeps in Folge, Proxy-MACs (Repeater) ausgenommen; neue Router-MAC erst nach zwei Sweeps, in den ersten 24 h ohne Meldung übernommen; MAC-Adressen sind nie Teil der Befund-Identität (keine Meldungsflut bei wechselnden Fälschungen); Bestätigen („Das ist mein neuer Router“, „Diesen Server kenne ich“) wird im Aktivitätsprotokoll festgehalten; Server-Liste auf 16 begrenzt, der Server des Routers wird nie verdrängt, Platz macht nur ein Eintrag, der eine Woche nicht gesehen wurde; steigt die Schwere eines Befunds, wird gemeldet (Review 8) | Unit-/API-Test |
| T-37 | T/D | Feindliche **Mesh-Liste/TR-064-Antworten** der FRITZ!Box (oder eines Geräts, das sich als FRITZ!Box ausgibt) | Nur über die gepinnte HTTPS-Verbindung (T-29); Dateipfade nur relativ auf demselben Router; Antwort max. 4 MiB; strikte Typen, Namen bereinigt und gekürzt, höchstens 1 000 WLAN-Geräte; fehlende Rechte betreffen nur WLAN bzw. Leitungsdaten, nie die Hostliste | Fuzz-Test (Mesh-Parser), Unit-Test |
| T-38 | I | **Anwesenheit ist personenbezogen** (Personen, Zeit im Heimnetz) | Standard nur „zu Hause seit …“; Zeit je Tag pro Person nur nach ausdrücklicher Bestätigung, dass die Person Bescheid weiß (Zeitpunkt gespeichert, Abschalten widerruft); keine Kommen/Gehen-Benachrichtigungen; Home Assistant je Person standardmäßig aus und nur mit derselben Bestätigung (es speichert den Verlauf selbst); Zeit je Tag erst ab der Bestätigung; MQTT-IDs aus der Personennummer, nie aus dem Namen, Löschen entfernt die Entität; Werkseinstellung löscht Personen; Hinweis zur Information des Haushalts in der Oberfläche | API-/Bridge-Test |
| T-39 | T/I | **Nachweis-Bericht** (PDF/CSV) mit Nutzertexten und persönlichen Angaben | Texte im PDF als Hex-Strings in WinAnsi (kein Ausbruch aus Strings möglich), Längen begrenzt und bereinigt; Name, Kundennummer und Anschrift werden nur in das PDF geschrieben, nie gespeichert oder protokolliert; `Cache-Control: no-store`; Zeitraum höchstens ein Jahr, nur abgeschlossene Tage; CSV enthält nur Zeiten, Zahlen und feste Kennungen; das PDF prüft CI mit `qpdf --check` | Unit-/API-Test, CI |
| T-40 | T | **Manipulierter Schnappschuss** bekannter ausgenutzter Lücken (falsche Entwarnung oder Fehlalarm) | Erzeugt nur von `tools/kevgen` aus CISA KEV und CVE List v5 an festen Commits; die CI erzeugt die Datei neu und vergleicht; eingebettet, zur Laufzeit keine Abfrage; Links nur `https://support.apple.com/…`; die Oberfläche zeigt den Datenstand, warnt ab 90 Tagen und sagt nie „keine Lücken“ | CI (`kevgen -verify`), Unit-Test |
| T-41 | I/D | **DNS-Prüfung** (0.3.2) verrät etwas über den Nutzer oder belastet DNS-Server | Nur im Sicherheitscheck und abschaltbar (`dnsCheck`); höchstens vier Server aus DHCP-Angaben bzw. Gateway, nur IPv4; feste öffentliche Namen und ein zufälliger Name unter `example.com` (reserviert, nie registrierbar); 4 s je Frage; Antworten nur verglichen, nie aufgelöst oder verwendet | Unit-Test (Fake-DNS) |
| T-42 | E/D | **Köder-Dienste des Stolperdrahts** (0.3.2) als Angriffsfläche | Kein Protokoll implementiert: nur eine feste Begrüßung, danach höchstens 4 KB lesen und verwerfen, 10 s Frist, höchstens 32 gleichzeitige Verbindungen (weitere werden sofort geschlossen); Inhalt wird nie gespeichert oder protokolliert; Ports < 1024 nur vor der Rechteabgabe geöffnet, danach CapEff = 0; standardmäßig aus | Unit-Test (lokale Listener, `-race`) |
| T-43 | S/D | **Fehlalarme oder Meldungsflut** über den Stolperdraht (gefälschte Absender, Port-Scan, Scan von außen) | Nur TCP mit vollständigem Handshake zählt (Absender nicht frei fälschbar); nur Heimnetz-Adressen (privat, ULA, link-local, 100.64/10), nie Loopback, eigene Adressen oder Internet-Adressen; ein Befund je Gerät (unbekannte IPv6-Quellen je /64, ab 16 unbekannten Quellen ein gemeinsamer Befund), Benachrichtigung nur beim ersten Mal; Ereignisse höchstens eines je Quelle und Port in 10 Minuten und 20 je Minute insgesamt, Merkliste hart auf 4 096 begrenzt, Meldung über eine begrenzte Warteschlange außerhalb des Annahme-Pfads; Wiederholungen nicht im Geräte-Lebenslauf; Port der Weboberfläche nie als Köder; Ausnahmen per IP/Netz; Befund verschwindet nach 30 Tagen ohne Treffer | Unit-/Security-Test |
| T-44 | T/I | **Hochgeladene Grundrisse** (0.3.2): feindliche Bilddateien, gespeicherte Inhalte | Nur `image/png` und `image/jpeg`, Format aus dem Dateikopf muss zum Content-Type passen; höchstens 8 MB und 6000 px je Seite; ausgeliefert mit geprüftem Typ, `nosniff` und CSP, nur an angemeldete Nutzer; höchstens 10 Grundrisse mit je 200 Punkten; der Browser kodiert Fotos vorher neu (entfernt Standortdaten) | API-Test |
| T-45 | D | **Messendpunkte der WLAN-Karte** als Lastquelle | Nur angemeldet; höchstens zwei Messungen gleichzeitig (sonst 429); Download/Upload je Anfrage höchstens 64 MiB; Raumnamen bereinigt (40 Zeichen); Messungen vom Netzwart-Rechner selbst werden abgelehnt | API-Test |

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
10. **Versionsangaben im LAN** (T-30): Ein Angreifer im Heimnetz kann Versionen fälschen – für sein eigenes Gerät
   und, im Rahmen der Plausibilitätsprüfung, auch für fremde Adressen (z. B. ein Android-Gerät „Android 9“ melden
   lassen). Folge ist ein falscher Befund oder, bei Geräten, die nur passive Angaben machen, eine falsche Entwarnung.
   Quelle und Rohwert sind sichtbar, aktive Quellen haben Vorrang, Wiederholungen alarmieren nicht erneut. Ein
   Angreifer, der das kann, hat das Netz bereits; das Risiko ist akzeptiert.
11. **Abdeckung**: Viele Geräte verraten keine Version (iPhones, Fernseher, Kameras). „Unbekannt“ ist keine Entwarnung;
   die Oberfläche sagt das ausdrücklich.

## 5. Sicherheitsrelevante Standards

- OWASP ASVS 4.0.3 Level 1 (Auth, Session, Access Control, Validation, Config) als Checkliste im Review
- CIS Docker Benchmark Abschnitt 4/5 (Image, Container-Laufzeit)
- OWASP Password Storage Cheat Sheet (Argon2id-Parameter)
