# 04 · Qualitätsmodell nach ISO/IEC 25010:2023

> Rolle: QA Lead · Status: **überarbeitet nach Plan-Review 1** (siehe [07](07-review-protokoll.md))
>
> Grundlage ist das Produktqualitätsmodell der ISO/IEC 25010:2023 mit 9 Merkmalen und deren Teilmerkmalen.
> Für jedes Teilmerkmal sind ein **konkretes Ziel**, eine **Maßnahme** und eine **Verifikation** festgelegt.
> Der Abschnitt „Nachweis“ wird im [Review-Protokoll](07-review-protokoll.md) ausgefüllt.

## 1. Funktionale Eignung (Functional suitability)

| Teilmerkmal | Ziel | Maßnahme | Verifikation |
|---|---|---|---|
| Vollständigkeit | Alle Must- und Should-Anforderungen aus [01](01-anforderungen.md) umgesetzt | Rückverfolgbarkeit F-ID → Code → Test | Review-Checkliste |
| Korrektheit | Erkennung, Presence und Klassifizierung arbeiten korrekt | Unit-Tests für Parser, Zustandsautomat und Klassifizierer; E2E-Scan gegen echte Container im Docker-Netz | `go test`, `e2e/scan` |
| Angemessenheit | Funktionen ohne Umwege erreichbar | Häufige Aufgaben (Gerät umbenennen, als bekannt markieren) mit ≤ 2 Klicks | UX-Review |

## 2. Leistungseffizienz (Performance efficiency)

| Teilmerkmal | Ziel | Maßnahme | Verifikation |
|---|---|---|---|
| Zeitverhalten | /24-Sweep < 10 s; API p95 < 50 ms bei 500 Geräten; erste Anzeige < 1 s im LAN | Indizes, gebündelte Schreibzugriffe, gzip, Cache für statische Assets | Integrations-Test (Sweep-Dauer), Benchmark |
| Ressourcennutzung | RAM ≤ 60 MB im Leerlauf (≤ 90 MB beim Login), CPU < 1 % Leerlauf, Image ≤ 40 MB | Statisches Binary, BPF im Kernel, `GOMEMLIMIT`, Argon2id 19 MiB mit 1 Slot | `docker stats`, Image-Größe in CI |
| Kapazität | ≥ 1 000 Geräte, ≥ 1 Jahr Ereignisse bei Standard-Aufbewahrung | Paginierung, Aufbewahrungs-Job, `VACUUM`-freie Strategie | Last-Test mit Demo-Daten |

## 3. Kompatibilität (Compatibility)

| Teilmerkmal | Ziel | Maßnahme | Verifikation |
|---|---|---|---|
| Koexistenz | Keine Konflikte mit anderen ZimaOS-Apps | Unüblicher Port (8840, konfigurierbar); kein Binden von Ports 53/67/5353; passives Mithören ohne Port-Bindung | Code-Review |
| Interoperabilität | Offene Formate und Protokolle | REST/JSON, SSE, CSV/JSON-Export, MQTT 3.1.1 + Home-Assistant-Discovery, Standard-Webhooks | Integrationstests |

## 4. Interaktionsfähigkeit (Interaction capability)

| Teilmerkmal | Ziel | Maßnahme | Verifikation |
|---|---|---|---|
| Erkennbarkeit der Eignung | Nutzen auf den ersten Blick | Übersicht beantwortet: Wie viele Geräte, was ist neu, ist Internet da? | UX-Review |
| Erlernbarkeit | Einrichtung ohne Doku in < 2 min | Setup-Assistent mit 2 Schritten, sinnvolle Standardwerte, Auto-Erkennung der Interfaces | E2E-Test (Funktion) + zeitgemessener Nutzertest |
| Bedienbarkeit | Tastatur, Touch und Maus gleichwertig | `/` fokussiert die Suche, `Esc` schließt Panels; Touch-Ziele ≥ 44 px | E2E + manueller Test |
| Schutz vor Fehlbedienung | Keine versehentlichen Datenverluste | Bestätigung für „Gerät vergessen“, Rückgängig-Hinweis; Validierung mit klaren Meldungen | E2E |
| Nutzerbindung | Ruhige, hochwertige Optik | Eigenes Designsystem, keine generischen Effekte; Hell/Dunkel | UX-Review |
| Inklusivität | WCAG 2.2 AA als Ziel | Kontrast ≥ 4,5:1 (Text) bzw. ≥ 3:1 (Grafik); Fokusringe; `aria-*`; Status nie nur über Farbe | Kontrast-Skript in CI, manuelle Tastaturprüfung |
| Nutzerunterstützung | Hilfe im Kontext | Erklärtexte bei Sicherheitsbefunden, leere Zustände mit nächstem Schritt | UX-Review |
| Selbstbeschreibungsfähigkeit | Begriffe ohne Fachjargon | Deutsche Sprache als Erstsprache, Fachbegriffe mit Tooltip | UX-Review |

## 5. Zuverlässigkeit (Reliability)

| Teilmerkmal | Ziel | Maßnahme | Verifikation |
|---|---|---|---|
| Fehlerfreiheit | 0 offene Defekte der Schwere hoch/kritisch; Parser-Fuzzing ohne Absturz | Tests, Race-Detector (`go test -race`), statische Analyse (`go vet`, `staticcheck`), Fuzzing | CI |
| Verfügbarkeit | Automatischer Neustart ≤ 60 s nach Hänger | Interner Watchdog (ADR-013) + `restart: unless-stopped`; `HEALTHCHECK` für Sichtbarkeit | Unit-Test Watchdog |
| Fehlertoleranz | Ausfall eines Moduls stoppt nicht das System | Isolierte Goroutinen mit `recover`; Fallback ohne `NET_RAW`; Benachrichtigungen mit Retry/Backoff | Unit-Test |
| Wiederherstellbarkeit | Kein Datenverlust bei Absturz/Update | SQLite-WAL, Transaktionen, Migrationen idempotent und versioniert; JSON-Export | Migrationstest |

## 6. Sicherheit (Security)

Vollständig im [Bedrohungsmodell](03-bedrohungsmodell.md) beschrieben, hier zugeordnet:

| Teilmerkmal | Maßnahmen (Referenz) |
|---|---|
| Vertraulichkeit | Pflicht-Login, write-only Geheimnisse, keine Telemetrie, Datei-Rechte (T-01…T-03, T-12…T-14) |
| Integrität | CSRF-Schutz, Eingabevalidierung, Normalisierung von Netzwerkdaten, CSP (T-04…T-09) |
| Nichtabstreitbarkeit | **Eingeschränkt**: Bei einem Admin-Konto und lokal änderbarer Datenbank ist echte Nichtabstreitbarkeit nicht erreichbar. Ereignisprotokoll als Nachvollziehbarkeit (T-11) |
| Verantwortlichkeit | Sitzungsliste mit IP/User-Agent/Zeit (T-03, T-11) |
| Authentizität | Argon2id, Sitzungstoken, Setup-Schutz (T-01…T-03) |
| Widerstandsfähigkeit | Drosselung, Limits, Fuzzing, minimale Rechte, Egress-Sperre, Host-Allowlist (T-02, T-06, T-16…T-20, T-24…T-26) |

## 7. Wartbarkeit (Maintainability)

| Teilmerkmal | Ziel | Maßnahme | Verifikation |
|---|---|---|---|
| Modularität | Klare Paketgrenzen, keine Zyklen | Schnittstellen zwischen Scan → Monitor → Store; `internal/`-Pakete | `go vet`, Review |
| Wiederverwendbarkeit | Protokoll-Parser unabhängig nutzbar | Parser ohne Seiteneffekte, reine Funktionen | Review |
| Analysierbarkeit | Fehler schnell eingrenzbar | Strukturierte Logs (`slog`), `/api/v1/system` mit Diagnose, Debug-Level | Review |
| Änderbarkeit | Neue Kanäle/Regeln ohne Kernänderung | Interface `notify.Sender`, Klassifizierungsregeln als Tabellen | Review |
| Testbarkeit | Kernlogik ohne Netzwerk testbar | Uhr und Netzwerk injizierbar; Demo-Modus mit Testdaten | Coverage ≥ 60 % für Kernpakete |

## 8. Flexibilität (Flexibility)

| Teilmerkmal | Ziel | Maßnahme | Verifikation |
|---|---|---|---|
| Anpassbarkeit | amd64 + arm64, beliebige Linux-Hosts | Statisches Binary, Multi-Arch-Image | CI-Build |
| Skalierbarkeit | Mehrere Interfaces/Subnetze | Pro Interface eigener Scanner; Subnetzgrenze konfigurierbar | Unit-Test |
| Installierbarkeit | Ein Compose-Block, keine Zusatzdateien | `docker-compose.yml` mit ZimaOS-Metadaten (`x-casaos`) | Kaltstart-Test in CI + manueller Import in ZimaOS |
| Austauschbarkeit | Daten portabel | CSV/JSON-Export und JSON-Import; SQLite-Datei | E2E |

## 9. Betriebssicherheit (Safety)

| Teilmerkmal | Ziel | Maßnahme | Verifikation |
|---|---|---|---|
| Betriebliche Einschränkung | Scans gefährden das Netz nicht | Ratenbegrenzung, keine aktiven Angriffe (kein ARP-Spoofing), Portscan nur TCP-Connect | Review |
| Risikoerkennung | Gefahren sichtbar machen | Sicherheitscheck mit Schweregraden und Empfehlungen | E2E |
| Ausfallsicherheit (fail safe) | Fehler führen zu sicherem Zustand | Ohne gültige Sitzung keine Daten; Fehler im Scanner → Status „eingeschränkt“, kein Absturz | Unit-Test |
| Gefahrenwarnung | Nutzer wird gewarnt | Hinweis bei HTTP ohne TLS, bei großen Subnetzen, bei fehlender `NET_RAW` | UX-Review |
| Sichere Integration | Sicher im Host-Netz | Minimal-Rechte-Container (T-20), keine gebundenen Systemports | CI-Test |
