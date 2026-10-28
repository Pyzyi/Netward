# 10 · Version 0.3.2

Stand: 28.10.2026 · Status: umgesetzt (Review 10 im [Review-Protokoll](07-review-protokoll.md))

Vier Funktionen, **einzeln** umgesetzt, jede mit eigenen Tests, Demo-Daten, deutschen und englischen Texten und
eigenem Commit. Alles bleibt defensiv: Netzwart beobachtet, fragt und stellt Köder auf, greift aber nie an und
liest keinen fremden Datenverkehr mit.

## F1 · Geräte-Lebenslauf und Inventar

**Nutzen:** Wann kam das Gerät ins Netz, wann bekam es ein Update, seit wann ist ein Port offen, wann fiel es aus?
Für kleine Büros dazu Kaufdatum, Garantie und Seriennummer, also eine einfache Inventarliste.

- **Chronik** (`device_log`, Migration 7): erstes Auftauchen, Wechsel der Systemfamilie, neue offene Ports,
  Versionswechsel (Software-Erkennung), Ausfälle. Sie überdauert die Aufbewahrungsfrist der Ereignisse, ist aber je
  Gerät auf 500 Einträge begrenzt. Beim Zusammenführen zweier Geräte wandert die Chronik mit.
- **Ausfälle** nur für Geräte, die durchlaufen sollten (Router, Zugangspunkt, Switch, Server, NAS, Kamera,
  Smart Home, Steckdose), ab 30 Minuten und nicht, wenn Netzwart selbst in dieser Zeit nicht lief.
- **Inventar:** Kaufdatum, Garantie bis (1980–2100) und Seriennummer (max. 64 Zeichen) im Gerätedetail; der
  CSV-Export enthält die Spalten. „Garantie endet in 24 Tagen“ bzw. „abgelaufen“ steht im Gerätedetail.
- API: `GET /api/v1/devices/{id}/history`, Felder in `PATCH /api/v1/devices/{id}`.

## F2 · Angriffserkennung im LAN

Netzwart erkannte schon ARP-Spoofing der Router-Adresse und zweite DHCP-Server (0.2). Neu ist die
**DNS-Prüfung** und eine gemeinsame Übersicht „Angriffserkennung im LAN“ auf der Sicherheitsseite.

- Im Sicherheitscheck fragt Netzwart jeden DNS-Server im Netz (vom DHCP-Server verteilt, sonst das Gateway,
  höchstens vier) nach drei Namen mit fester, öffentlich bekannter Adresse (`one.one.one.one`, `dns.google`,
  `dns.quad9.net`) und nach einem zufälligen Namen unter `example.com`, den es nicht gibt.
- **Bewertung:** falsche Adresse für einen bekannten Namen → Befund `dns_manipulated` (kritisch). Adresse für den
  Namen, den es nicht gibt → `dns_redirect` (Warnung, typisch für „Navigationshilfen“ von Anbietern).
  `0.0.0.0` oder Loopback gilt als Sperre (Pi-hole, AdGuard) und ist kein Befund.
- Abschaltbar (`dnsCheck`, Einstellungen → Scannen). Befunde verschwinden beim nächsten Lauf, wenn der Server
  wieder richtig antwortet oder die Prüfung aus ist.

## F3 · Stolperdraht (Köder-Dienste)

**Nutzen:** Kein normales Gerät spricht Telnet, Remotedesktop oder ADB auf einem Rechner an, der diese Dienste nie
angeboten hat. Eine Verbindung dorthin ist ein starkes Zeichen für Schadsoftware, die das Netz absucht, oder für
jemanden, der sich umsieht. Falsche Alarme sind fast ausgeschlossen.

- Standardmäßig **aus**. Ports 2323 (Telnet), 3389 (RDP), 5555 (ADB), 5900 (VNC), 8081 (Kamera-Login),
  in den Einstellungen 1024–65535 frei wählbar, höchstens 16.
- **Ports unter 1024** (z. B. 23, 445) öffnet nur der Start über `NETZWART_TRIPWIRE_PORTS`, bevor die Rechte
  abgegeben werden; das braucht `cap_add: NET_BIND_SERVICE`. Danach hat der Prozess wie bisher keine
  Capabilities. Ausschalten schließt auch diese Ports; wieder öffnen geht erst nach einem Neustart.
- **Verhalten des Köders:** eine Begrüßung wie der echte Dienst (z. B. `login:` oder eine HTTP-401-Seite mit
  „IP Camera“), höchstens 4 KB lesen, 10 Sekunden, dann schließen. Der Inhalt wird nie gespeichert oder
  protokolliert. Höchstens 32 Verbindungen gleichzeitig.
- **Wer zählt:** nur Adressen aus dem Heimnetz (privat, ULA, link-local, 100.64/10 für VPNs wie Tailscale).
  Nicht: Loopback, eigene Adressen des Hosts, Internet-Adressen (etwa über eine Portweiterleitung), Ausnahmen
  (IP oder Netz, z. B. ein eigener Scanner).
- **Meldung:** ein kritischer Befund `tripwire` je Gerät (unbekannte Geräte je Adresse), Benachrichtigung nur beim
  ersten Mal; jede Verbindung als Ereignis, höchstens eines je Adresse und Port in 10 Minuten. Ohne neuen Treffer
  verschwindet der Befund nach 30 Tagen.
- Status je Port (offen, belegt, erst nach Neustart, Fehler) und letzter Treffer in `/api/v1/network` und in der
  Angriffserkennung.

## F4 · WLAN-Karte

**Nutzen:** „Im Kinderzimmer nur 40 Mbit/s, ein Verstärker im Flur würde helfen.“ Ohne Router-Zugang, nur mit
dem Handy und einem Grundriss.

- **Grundriss:** Foto, Skizze oder Screenshot. Der Browser verkleinert auf höchstens 2400 px und kodiert neu
  (PNG bleibt PNG, sonst JPEG). Das macht HEIC vom iPhone lesbar und entfernt Standortdaten aus Fotos. Der Server
  nimmt nur PNG/JPEG bis 8 MB und 6000 px je Seite an (Kopf wird gelesen, Format muss zum Content-Type passen),
  höchstens 10 Grundrisse mit je 200 Messpunkten (Migration 8).
- **Messung im Browser** zwischen Handy und Netzwart: Reaktionszeit (zehn kleine Anfragen nach einer zum
  Aufwärmen, Median und Schwankung), Download (Datenstrom zufälliger Bytes, etwa drei Sekunden, die erste halbe
  Sekunde zählt nicht), Upload (wachsende Blöcke bis etwa drei Sekunden). Höchstens zwei Messungen gleichzeitig.
  Gemessen wird bewusst nicht das Internet (siehe [01](01-anforderungen.md), kein Speedtest).
- **Gerät und Funk:** Netzwart erkennt das messende Gerät an der Adresse; mit FRITZ!Box kommen Band, Signal und
  Zugangspunkt dazu. Messungen vom Netzwart-Rechner selbst werden abgelehnt (außer im Demo-Modus).
- **Karte:** inverse Distanzgewichtung der Werte im Vergleich zur besten Stelle, zum Rand der Messpunkte hin
  ausgeblendet. Ist die schnellste Stelle mehr als doppelt so schnell wie die zweitbeste (z. B. am Kabel
  gemessen), dient die zweitbeste als Maßstab.
- **Hinweise** je Raum (schlechteste Stelle des Raums): schwach unter 40 % der besten Stelle oder unter
  10 Mbit/s; dazu der Raum für einen Verstärker, wenn eine gute gemessene Stelle nahe der Mitte zwischen bester
  und schwacher Stelle liegt; 2,4-GHz-Hinweis mit FRITZ!Box; „schwankt“ ab 50 ms Reaktionszeit oder 30 ms
  Schwankung. Ohne Schwachstelle: „Überall gut versorgt“.
- Seite `/wlan/map`, erreichbar über WLAN (auch ohne FRITZ!Box).

## Sicherheit (neue Bedrohungen)

Ausführlich in [03](03-bedrohungsmodell.md), T-41 bis T-45.

| ID | Bedrohung | Maßnahme |
|---|---|---|
| T-41 | DNS-Prüfung verrät Nutzung oder stört | Nur im Sicherheitscheck, abschaltbar, höchstens vier Server, 4 s je Frage, nur Namen ohne Bezug zum Nutzer |
| T-42 | Köder-Dienste als Angriffsfläche | Kein Protokoll implementiert, nur Begrüßung; 4 KB, 10 s, 32 Verbindungen; Inhalt nie gespeichert |
| T-43 | Fehlalarm- oder Meldungsflut über den Stolperdraht | Ein Befund je Gerät, Benachrichtigung nur beim ersten Mal, Ereignisse begrenzt, nur Heimnetz-Adressen |
| T-44 | Hochgeladene Bilder (Grundriss) | Nur PNG/JPEG, Format geprüft, Größen- und Pixelgrenzen, `nosniff`, ausgeliefert nur an angemeldete Nutzer |
| T-45 | Messendpunkte als Lastquelle | Nur angemeldet, höchstens zwei gleichzeitig, Größe begrenzt |

## Datenmodell

- 7: `devices.purchase_date`, `warranty_until`, `serial`; Tabelle `device_log`
- 8: Tabellen `floor_plans`, `wifi_points`

DNS-Ergebnis und Stolperdraht-Status brauchen keine Tabelle (Einstellung bzw. Laufzeit).

## Abnahme

Unit-Tests (DNS mit Fake-Server, Köder mit lokalen Listenern, Hinweise der WLAN-Karte), API-Tests (Upload,
Messpunkte, Ablehnung lokaler Messungen), e2e-Tests für jede Funktion auf Desktop und Handy, Demo-Daten für jede
Funktion. Danach Code- und Security-Review über alles (Review 10), dann Release 0.3.2 über `VERSION`.
