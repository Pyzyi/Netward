# 09 · Version 0.2

Stand: 26.10.2026 · Status: Plan, geprüft (Review 7, Entscheidungen im letzten Abschnitt)

Sechs Funktionen, **einzeln** umgesetzt: jede mit eigenen Tests, eigenem Commit und grüner CI, bevor die nächste beginnt. Reihenfolge nach Größe und Abhängigkeit.

## F1 · Zweiter DHCP-Server und geänderte Router-MAC

**Nutzen:** Ein zweiter DHCP-Server (falsch konfigurierter Pi-hole, Reiserouter, Angreifer) verteilt falsche Router- und DNS-Adressen; das führt zu „Internet geht manchmal nicht“ oder zu umgeleitetem Verkehr. Eine plötzlich andere MAC für die Router-Adresse ist ein typisches Zeichen für ARP-Spoofing – oder schlicht ein neuer Router.

**Erkennung (rein beobachtend bzw. Standardanfragen):**
- Passiv: DHCP-Antworten (OFFER/ACK), die ohnehin am Host vorbeikommen. Aus Option 54 (Server-ID), Absender-MAC, Option 3 (Router) und Option 6 (DNS) wird je Server ein Eintrag.
- Aktiv, nur im Sicherheitscheck: ein DHCPDISCOVER mit Broadcast-Flag von einer zufälligen, lokal verwalteten MAC; alle Server antworten per Broadcast. Kein REQUEST, also keine vergebene Adresse (das Angebot verfällt nach Sekunden).
- Router-MAC: Für jede Gateway-Adresse wird die MAC gemerkt, die sie über eine Woche stabil hatte. Antwortet die Adresse plötzlich mit einer anderen MAC (oder mit zwei MACs zugleich), entsteht ein Befund.

**Bewertung:** Der erste beobachtete Server, dessen Server-ID das Gateway oder ein bestätigtes Gerät ist, gilt als „der“ DHCP-Server. Jeder weitere → Befund `dhcp_rogue` (Warnung; kritisch, wenn er einen anderen Router oder DNS als der bekannte verteilt). Router-MAC-Wechsel → Befund `gateway_mac` (kritisch bei zwei MACs gleichzeitig, sonst Warnung). Beides lässt sich bestätigen („Das ist mein neuer Router / mein Pi-hole“); dann gilt der neue Zustand als normal.

## F2 · Verhaltens-Baseline pro Gerät

Für **bekannte** Geräte (bestätigt, älter als die Lernphase):
- **Neue offene Ports:** Ein Port, der bei einem früheren Scan desselben Umfangs geschlossen war → Ereignis und Befund `new_port` (Warnung bei riskanten Diensten, sonst Hinweis). Vergleich nur innerhalb der jeweils gescannten Ports.
- **Hostnamen-Wechsel:** Der vom Gerät selbst gemeldete DHCP-Name ändert sich → Ereignis (kein Befund).
- **Identität passt nicht mehr:** Die aus DHCP abgeleitete Systemfamilie wechselt (z. B. iOS → Windows) oder die Herstellerangabe aus mDNS/UPnP widerspricht der bisherigen → Befund `identity_changed` (Warnung): „Ein anderes Gerät könnte die MAC-Adresse übernommen haben.“ Reine Fingerprint-Änderungen innerhalb derselben Familie (Systemupdate) erzeugen nur ein Ereignis. Private (zufällige) MACs sind ausgenommen.

## F3 · WLAN-Diagnose aus der FRITZ!Box

Quelle: TR-064 `Hosts:1 X_AVM-DE_GetMeshListPath` (JSON der Mesh-Topologie, derselbe Benutzer wie bisher). Je WLAN-Gerät: Zugangspunkt (FRITZ!Box oder welcher Repeater), Band (aus Kanal/Schnittstelle), Signalstärke (dBm), aktuelle und maximale Datenrate; für Repeater zusätzlich die Verbindung zur FRITZ!Box (Backhaul).

Hinweise (regelbasiert, nur wenn eindeutig):
- „Schwaches Signal“ (< −75 dBm), „sehr schwach“ (< −82 dBm)
- „Hängt im 2,4-GHz-Band, obwohl es 5 GHz kann“ (Gerät meldet 5-GHz-fähige PHY-Modi, ist aber im 2,4-GHz-Band)
- „Repeater mit schwacher Anbindung“ (Backhaul < 100 Mbit/s oder < −75 dBm) – betrifft alle Geräte an diesem Repeater
- „Niedrige Datenrate“ (aktuell < 20 % der maximalen)

Anzeige im Gerätedetail (Abschnitt „WLAN“) und eine Übersicht „WLAN“ mit allen Zugangspunkten und ihren Geräten. Abfrage alle 5 Minuten, nur aktueller Stand (kein Verlauf in 0.2).

## F4 · Personen statt Geräte

- Personen anlegen (Name), Geräte zuordnen (ein Gerät gehört höchstens einer Person). Pro Gerät wählbar, ob es für die Anwesenheit zählt (Standard: Handy und Uhr).
- **Zu Hause**, wenn mindestens ein Anwesenheitsgerät online ist; „seit …“.
- **Zeit im Heimnetz** je Tag (letzte 7 Tage): Vereinigung der Online-Zeiten der Anwesenheitsgeräte. Netward sieht nur, *dass* ein Gerät verbunden ist – nicht, was darauf passiert; die Oberfläche sagt das so.
- Home Assistant: pro Person ein `device_tracker` (home/not_home) per MQTT Discovery, abschaltbar.
- Hinweis in der Oberfläche: Alle Personen im Haushalt sollten wissen, dass ihre Anwesenheit erfasst wird.

## F5 · Nachweis-Bericht für den Provider (PDF)

- **Leitungsdaten sammeln:** alle 15 Minuten von der FRITZ!Box `WANCommonInterfaceConfig:GetCommonLinkProperties` (Leitung bereit, max. Raten; DSL, Kabel, Glasfaser) und bei DSL `WANDSLInterfaceConfig:GetInfo` (Sync-Raten, mögliche Raten). Neue Tabelle, Aufbewahrung wie die übrige Historie.
- **Bericht:** Zeitraum wählen (Standard: letzte 30 Tage), gebuchte Bandbreite eintragen (optional). Inhalt: Verfügbarkeit in %, Zahl, Summe und längster Ausfall, Liste aller Ausfälle (Beginn, Ende, Dauer, Art: Router/Internet/DNS), Latenz je Tag (Median, 95. Perzentil), Sync-Raten (min/Ø/max, Verlauf als Diagramm), Anteil der Zeit unter X % der gebuchten Rate.
- **PDF:** eigener, kleiner PDF-Schreiber (Helvetica, WinAnsi für Umlaute, Linien und Rechtecke für Diagramme) – keine neue Abhängigkeit. Download aus der Oberfläche (Internet → „Bericht erstellen“).
- **Ehrlichkeit:** Hinweis im Bericht und in der Oberfläche: Für eine Minderung nach § 57 TKG verlangt die Bundesnetzagentur das Messprotokoll der „Breitbandmessung“; dieser Bericht ergänzt das (Ausfälle, Sync-Raten), ersetzt es aber nicht. Keine Geräteliste im Bericht.

## F6 · Offline-Abgleich bekannter, aktiv ausgenutzter Lücken

- **Daten:** CISA „Known Exploited Vulnerabilities“ (KEV) plus die betroffenen Versionen aus den CVE-Datensätzen (CVE List v5). Ein Generator (`tools/kevgen`) erzeugt pro Release einen kleinen Schnappschuss `internal/vuln/kev.json` nur für Produkte, die Netward erkennen kann (zunächst Apple: iOS, iPadOS, macOS, tvOS, audioOS, watchOS). Eingebettet, mit Datum; keine Online-Abfrage.
- **Abgleich nur bei genauer Version** (mindestens Haupt- und Nebenversion, z. B. tvOS 26.4 aus AirPlay oder eine genaue manuelle Angabe). Nur die Hauptversion („iOS 18“) reicht nicht – dann kein Urteil.
- **Befund** `known_exploited` (kritisch): „Für tvOS 26.4 sind aktiv ausgenutzte Lücken bekannt (CVE-…), behoben in 26.5.“ Mit Link zum Hersteller-Hinweis aus dem Datensatz (nur `https`-Links von bekannten Herstellerdomains).
- Manuelle Angabe erlaubt künftig auch „18.6.2“.
- Bewusst nicht: Windows (Build ohne Update-Stand sagt nichts über Patches), Linux-Distributionen (Rückportierungen), Geräte ohne genaue Version.

## Sicherheit (neue Bedrohungen)

| ID | Bedrohung | Maßnahme |
|---|---|---|
| T-35 | Aktiver DHCPDISCOVER stört das Netz | Nur im Sicherheitscheck, 1 Paket, kein REQUEST, zufällige lokal verwaltete MAC, Antworten 3 s gesammelt |
| T-36 | Gefälschte DHCP-Antworten oder ARP-Pakete erzeugen Fehlalarme | Bestätigen-Funktion; Router-MAC erst nach einer stabilen Woche als Referenz; doppelte Meldungen höchstens einmal pro Tag |
| T-37 | Feindliche Mesh-JSON/TR-064-Antworten | Größenlimit, strikte Typen, Pfad relativ wie bei der Hostliste |
| T-38 | Anwesenheitsdaten sind personenbezogen | Nur im angemeldeten Bereich, Export nur bewusst, Hinweis zur Information der Haushaltsmitglieder, MQTT je Person abschaltbar |
| T-39 | PDF-Erzeugung mit Nutzertexten (Netzwerkname) | Nur WinAnsi-Zeichen, Escape von `()\\`, Längenbegrenzung; Fuzz-Test |
| T-40 | Manipulierter KEV-Schnappschuss | Wird nur per Generator aus den offiziellen Quellen erzeugt, im Repo versioniert und im Review sichtbar; zur Laufzeit keine Abfrage |

## Datenmodell (Migrationen 4–7, je Funktion eine)

- 4: `dhcp_servers` (server_id, mac, router, dns, first_seen, last_seen, accepted), `gateway_refs` (ip, mac, since, accepted)
- 5: `persons` (id, name, created_at), `person_devices` (device_id PRIMARY KEY, person_id, presence)
- 6: `line_samples` (ts, link_up, down_kbps, up_kbps, down_max_kbps, up_max_kbps)
- 7: keine Tabelle für F2/F3/F6 nötig (Befunde, Ereignisse, Live-Abfrage)

## Abnahme je Funktion

Unit-Tests (Parser mit echten Beispielen, Fake-FRITZ!Box für Mesh und Leitung), API-Test, UI-Test in der Demo, Demo-Daten für jede Funktion, CI grün. Am Ende Code- und Security-Review über alles, dann Release 0.2.0 über `VERSION`.

## Entscheidungen nach dem Plan-Review (Review 7)

Die Abschnitte oben bleiben als ursprünglicher Plan stehen; wo sie abweichen, gilt diese Liste.

**Reihenfolge:** F1a (Router-MAC, DHCP passiv) → F1b (aktive DHCP-Anfrage) → F3 → F5 → F6 → F4 → F2. Jede Funktion bringt ihre Migration (fortlaufend nummeriert in dieser Reihenfolge), deutsche und englische Texte, Schalter, Benachrichtigungsart und Demo-Daten selbst mit.

**F1**
- Router-MAC über die vorhandene Konfliktlogik (`detectConflicts`): Repeater mit Proxy-ARP/MAC-NAT bleiben ausgenommen, zwei Sweeps in Folge sind nötig. Zwei MACs gleichzeitig auf der Router-Adresse werden als *ein* verschärfter Befund gemeldet, nicht zusätzlich zum IP-Konflikt.
- MAC-Adressen sind nie Teil der Befund-Identität (keine Meldungsflut bei wechselnden Fälschungen).
- Hersteller zählt: AVM → AVM und alte MAC verschwunden = „Neuer Router?“ (Hinweis); anderer Hersteller oder alte MAC weiterhin aktiv = Warnung bzw. kritisch.
- Gibt es keinen eindeutigen Referenz-Server (z. B. nur ein Pi-hole), fragt Netward „Welcher Server ist deiner?“ statt zu alarmieren.
- Passive Erkennung sieht wenig (DHCP-Antworten gehen meist gezielt an den Client); die aktive Anfrage (F1b) ist der verlässliche Weg. Sie fragt Router, DNS und Server-ID ab (Option 55), prüft die Transaktions-ID, nutzt **eine feste MAC pro Installation** (nicht jedes Mal eine neue) und erzeugt nie ein Gerät. Abschaltbar. Nur IPv4.

**F3**
- Signal aus RCPI umrechnen (dBm = RCPI/2 − 110, 255 = unbekannt); Testdaten mit `schema_version`.
- Regel „niedrige Datenrate“ entfällt. 5 GHz gilt nur bei `a`/`ac` in den Fähigkeiten als möglich.
- Hinweise erst nach drei Abfragen in Folge, keine Benachrichtigungen.
- Fehlende Rechte für die Mesh-Liste betreffen nur diese Funktion, nicht die ganze FRITZ!Box-Anbindung. Ersatzweise die WLAN-Geräteliste ohne Repeater-Zuordnung.

**F5**
- Neue Tabellen für Latenz (je 5 Minuten: Median, Maximum, Verlust) und Art des Ausfalls (Router, Internet, DNS). Router-Ausfälle und Messlücken von Netward zählen im Bericht nicht gegen den Provider.
- Leitungsdaten jede Minute lesen, nur bei Änderung speichern; Resyncs über die Verbindungsdauer erkennen. Sync-Raten-Diagramm nur bei DSL; der Bericht funktioniert auch ohne FRITZ!Box.
- Vertragswerte: maximal, normalerweise verfügbar, minimal („Was steht in deinem Vertrag?“). Hinweis: Sync-Rate ist nicht Durchsatz.
- PDF: Texte als Hex-Strings, Prüfung mit `qpdf --check` im CI, Rohdaten als CSV mit SHA-256 im Anhang. Optional Name/Kundennummer/Anschrift nur für den Bericht, nicht gespeichert. Kein WLAN-Name.

**F6**
- Abgleich je Versionszweig (ältere Zweige haben eigene „behoben in“-Angaben); „17.1“ steht für 17.1.x.
- Produktnamen normalisieren (iOS and iPadOS, macOS-Namen, visionOS).
- Manuelle Angaben: 90-Tage-Regel; statt eines kritischen Befunds die Bitte, die Version zu bestätigen.
- `kev.json` enthält KEV-`catalogVersion` und den festen Commit des CVE-Repos; die CI erzeugt die Datei neu und vergleicht. Datum in der Oberfläche, Warnung ab 90 Tagen; nie „keine Lücken bekannt“.

**F4**
- Standard: nur „zu Hause seit …“. „Zeit im Heimnetz je Tag“ pro Person einschaltbar, mit Bestätigung, dass die Person Bescheid weiß.
- Keine Kommen/Gehen-Benachrichtigungen per Voreinstellung. MQTT-IDs aus der Personen-ID, nie aus dem Namen; Löschen entfernt alles.
- Hinweis zu iOS-„rotierenden“ privaten WLAN-Adressen (für Anwesenheit „fest“ wählen) und dass Home Assistant den Verlauf selbst speichert.

**F2 (zuletzt, verkleinert)**
- Keine Hersteller-Abgleiche aus mDNS/UPnP (Docker-, NAS- und Medien-Apps melden fremde Hersteller).
- Identität: nur Wechsel der DHCP-Systemfamilie bei bekannten Geräten, die mindestens 7 Tage bekannt sind und zweimal per DHCP gesehen wurden; Buttons „neu eingerichtet“ / „kenne ich nicht“.
- Neue Ports nur mit gespeichertem Scan-Umfang und erst, wenn zwei Scans übereinstimmen; sonst nur Ereignisse.

## Umsetzung (Stand Release 0.2.0)

Reihenfolge wie beschlossen, jede Funktion einzeln mit eigenen Tests, Commit und grüner CI: F1a → F1b → F3 → F5a/b/c → F6a/b → F4 → F2.

Abweichungen vom Plan, bewusst:

- **Datenmodell:** Router-Referenzen, DHCP-Server, Probe-MAC und Systemfamilien sind kleine JSON-Einstellungen statt eigener Tabellen (wenige Einträge, keine Abfragen darüber). Die Migrationen sind daher: 4 Latenz-Abschnitte und Ausfallart, 5 Leitungsdaten, 6 Personen.
- **F1b:** Absender-MAC des DISCOVER ist die eigene Schnittstelle, nur `chaddr` ist die feste Probe-MAC. So funktioniert die Prüfung auch über WLAN und hinterlässt keine fremde MAC in Switch-Tabellen.
- **F3:** Abfrage alle 5 Minuten, Hinweise nach drei Abfragen in Folge; ohne Recht für die Mesh-Liste die WLAN-Gerätelisten je Funkband (Signal in Prozent, keine Repeater, daher keine Signal-Hinweise).
- **F5:** Der Bericht umfasst nur abgeschlossene Tage; dadurch enthalten PDF und die getrennt geladene CSV dieselben Daten und die Prüfsumme im PDF passt. Fiel der Router aus, zählt das nicht gegen den Anbieter; maßgeblich ist die Art beim Beginn des Ausfalls.
- **F6:** Abgleich für iOS, iPadOS, macOS und tvOS. „17.1“ ohne Patch-Stand ergibt „unsicher“, wenn die Lücke erst in 17.1.x geschlossen wurde. Neben „kritisch“ (erkannte Version) gibt es „Warnung“ (eigene Angabe, bitte bestätigen) und „Hinweis“ (unsicher).
- **F2:** Neue Ports erscheinen nur als Ereignis (zwei übereinstimmende Scans desselben Umfangs), ohne Befund. Der Systemwechsel bleibt bei Familien (Apple, Windows, Android, Linux); iPhone → Mac ist kein Wechsel.
- **Nebenbei behoben:** MQTT-Verbindung brach direkt nach dem Start einmal ab; CI hat Zeitlimits je Job.
