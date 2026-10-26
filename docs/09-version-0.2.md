# 09 · Version 0.2

Stand: 26.10.2026 · Status: Plan, zur Prüfung

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
- **Zeit im Heimnetz** je Tag (letzte 7 Tage): Vereinigung der Online-Zeiten der Anwesenheitsgeräte. Netzwart sieht nur, *dass* ein Gerät verbunden ist – nicht, was darauf passiert; die Oberfläche sagt das so.
- Home Assistant: pro Person ein `device_tracker` (home/not_home) per MQTT Discovery, abschaltbar.
- Hinweis in der Oberfläche: Alle Personen im Haushalt sollten wissen, dass ihre Anwesenheit erfasst wird.

## F5 · Nachweis-Bericht für den Provider (PDF)

- **Leitungsdaten sammeln:** alle 15 Minuten von der FRITZ!Box `WANCommonInterfaceConfig:GetCommonLinkProperties` (Leitung bereit, max. Raten; DSL, Kabel, Glasfaser) und bei DSL `WANDSLInterfaceConfig:GetInfo` (Sync-Raten, mögliche Raten). Neue Tabelle, Aufbewahrung wie die übrige Historie.
- **Bericht:** Zeitraum wählen (Standard: letzte 30 Tage), gebuchte Bandbreite eintragen (optional). Inhalt: Verfügbarkeit in %, Zahl, Summe und längster Ausfall, Liste aller Ausfälle (Beginn, Ende, Dauer, Art: Router/Internet/DNS), Latenz je Tag (Median, 95. Perzentil), Sync-Raten (min/Ø/max, Verlauf als Diagramm), Anteil der Zeit unter X % der gebuchten Rate.
- **PDF:** eigener, kleiner PDF-Schreiber (Helvetica, WinAnsi für Umlaute, Linien und Rechtecke für Diagramme) – keine neue Abhängigkeit. Download aus der Oberfläche (Internet → „Bericht erstellen“).
- **Ehrlichkeit:** Hinweis im Bericht und in der Oberfläche: Für eine Minderung nach § 57 TKG verlangt die Bundesnetzagentur das Messprotokoll der „Breitbandmessung“; dieser Bericht ergänzt das (Ausfälle, Sync-Raten), ersetzt es aber nicht. Keine Geräteliste im Bericht.

## F6 · Offline-Abgleich bekannter, aktiv ausgenutzter Lücken

- **Daten:** CISA „Known Exploited Vulnerabilities“ (KEV) plus die betroffenen Versionen aus den CVE-Datensätzen (CVE List v5). Ein Generator (`tools/kevgen`) erzeugt pro Release einen kleinen Schnappschuss `internal/vuln/kev.json` nur für Produkte, die Netzwart erkennen kann (zunächst Apple: iOS, iPadOS, macOS, tvOS, audioOS, watchOS). Eingebettet, mit Datum; keine Online-Abfrage.
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
