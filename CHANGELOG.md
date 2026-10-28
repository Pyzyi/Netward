# Änderungen

Alle nennenswerten Änderungen an Netward. Versionen folgen [Semantic Versioning](https://semver.org/lang/de/);
vor 1.0 können sich Einzelheiten noch ändern. Die Release-Notizen auf GitHub stehen in [`docs/releases`](docs/releases).

## 0.4.1 – 2026-10-28

### Behoben
- Der Router sitzt genau in der Mitte des Netzrings (vorher lag das Symbol über der Mitte).

## 0.4.0 – 2026-10-28

### Geändert
- **Neuer Name: Netward** (vorher Netzwart). Repository `Pyzyi/Netward`, Image `ghcr.io/pyzyi/netward`, Befehl
  `/netward`.
- Umgebungsvariablen heißen `NETWARD_*`; die alten `NETZWART_*` gelten weiter.
- Die Datenbank `netzwart.db` wird beim ersten Start als `netward.db` übernommen.
- Home Assistant: Entitäten heißen `netward_…`, die alten `netzwart_…` werden vom Broker entfernt.
- Sitzungs-Cookie umbenannt (einmal neu anmelden).

## 0.3.3 – 2026-10-28

### Sicherheit
- Ersteinrichtung über einen Reverse-Proxy nur von vertrauten Proxys (`NETWARD_TRUSTED_PROXIES`).
- Login-Bremse gegen Fluten mit vielen Adressen/Namen gehärtet (IPv6 je /64, feste Grenze).
- Benachrichtigungen ohne Weiterleitungen; FRITZ!Box-Sitzungs-ID nicht mehr im Log; Ping nur mit Antworten vom
  Ziel; Live-Strom mit absoluter Sitzungsdauer; Cloud-Metadaten-Dienste gesperrt; Warnung bei nicht vertrautem Proxy.
- `factory-reset` über die Kommandozeile wirkt beim nächsten Start.
- CI: Release nur bei sicher fehlendem Tag und nur von main; wöchentlicher Scan des veröffentlichten Images.

### Behoben
- E-Mail: Zeilen mit Punkt am Anfang kamen mit zwei Punkten an.

### Intern
- Ungenutzter Code, Texte, Icons und CSS entfernt.

## 0.3.2 – 2026-10-28

### Neu
- **WLAN-Karte:** Grundriss hochladen, mit dem Handy durch die Wohnung gehen und antippen, wo man steht. Netward
  misst Reaktionszeit, Download und Upload zwischen Handy und Netward (ohne Router-Zugang) und zeichnet eine Karte
  mit Hinweisen wie „Im Kinderzimmer nur 38 Mbit/s, ein Verstärker im Flur würde helfen“. Mit FRITZ!Box kommen Band
  und Signal dazu. Fotos werden im Browser verkleinert und neu kodiert (ohne Standortdaten).
- **Stolperdraht** (standardmäßig aus): Köder-Ports, die wie Telnet, Remotedesktop, ADB, VNC oder ein Kamera-Login
  aussehen. Jede Verbindung aus dem Heimnetz ist ein kritischer Befund mit Benachrichtigung. Ports unter 1024 über
  `NETWARD_TRIPWIRE_PORTS` (braucht `cap_add: NET_BIND_SERVICE`). Ausnahmen für eigene Scanner.
- **Angriffserkennung im LAN:** neue Übersicht auf der Sicherheitsseite (Router-Adresse, Adressvergabe, DNS,
  Stolperdraht). Neu ist die DNS-Prüfung im Sicherheitscheck: Liefert ein DNS-Server im Netz falsche Adressen für
  bekannte Namen, gibt es einen kritischen Befund; leitet er nicht existierende Namen um, eine Warnung. Sperren durch
  Pi-hole oder AdGuard sind kein Befund. Abschaltbar.
- **Geräte-Lebenslauf:** Chronik je Gerät mit erstem Auftauchen, Systemwechsel, neuen Ports, Updates und Ausfällen.
  Sie bleibt auch nach Ablauf der Aufbewahrungsfrist erhalten.
- **Inventar:** Kaufdatum, Garantie und Seriennummer je Gerät, auch im CSV-Export.
- README auf Englisch als Startseite, Deutsch in `README.de.md`; englische Screenshots.
- `NETWARD_DEMO=en` startet die Demo mit englischen Geräte- und Personennamen.
- Vorbereitete Einträge für die App-Stores von CasaOS/ZimaOS, Unraid und TrueNAS in `deploy/`.
- Beitragsleitfaden (`CONTRIBUTING.md`) und Issue-Vorlagen.

### Datenbank
- Migration 7: Inventarfelder an `devices`, Tabelle `device_log`.
- Migration 8: Tabellen `floor_plans` und `wifi_points`.

## 0.2.1 – 2026-10-27

### Geändert
- Neues Erscheinungsbild „Klartext“ für die ganze Oberfläche: ruhige weiße Flächen auf neutralem Grund, IBM Plex Sans
  (Monospace nur noch für IP, MAC und Ports), Beschriftungen in normaler Schreibweise statt Versalien, Statuschips,
  Segment-Schalter und runde Schalter. Hinweise sind getönte Flächen statt Kästen mit farbigem Randstrich.
- Übersicht: Der Netzring ist größer und hat eine Legende; auf dem Handy wird er jetzt ebenfalls gezeigt. Oben steht
  die Antwort als Satz („26 von 30 Geräten online“) statt einer Riesenzahl.
- Geräteliste, Aktivität, Sicherheit, Personen und Einstellungen stehen in Panels; Personen auf breiten Bildschirmen
  zweispaltig.
- Dunkelmodus mit neutralem Grau statt Nachtblau.

### Intern
- Ungenutzte Abhängigkeit `@lucide/svelte` entfernt; NOTICE nennt die tatsächlich gebündelten Icons (Phosphor) und Schriften.

## 0.2.0 – 2026-10-27

### Behoben
- Home Assistant (MQTT): Nach dem Start brach die erste Verbindung sofort wieder ab und kam erst nach 5 Sekunden zurück.

### Neu
- Zweiter DHCP-Server: Netward merkt sich passiv, welche Geräte Adressen verteilen, und meldet einen weiteren Verteiler – kritisch, wenn er einen anderen Router oder DNS-Server vorgibt. „Diesen Server kenne ich“ bestätigt bewusst betriebene Server (z. B. Pi-hole).
- Router-Adresse: Antworten zwei Geräte auf die Router-Adresse oder gehört sie plötzlich einer anderen Hardware, gibt es einen Sicherheitshinweis (ARP-Spoofing-Erkennung). „Das ist mein neuer Router“ übernimmt einen echten Router-Tausch.
- Aktive DHCP-Prüfung im Sicherheitscheck: ein einzelner DHCPDISCOVER (Broadcast, feste lokale Kennung je Installation, kein REQUEST, nur IPv4) findet auch DHCP-Server, die passiv nicht auffallen. Antworten zählen nur mit passender Transaktions-ID. Abschaltbar unter Einstellungen → Scannen.
- Sicherheitsseite: Übersicht „Router und Adressvergabe“ mit Router-Kennung, bekannten DHCP-Servern und letzter aktiver Prüfung.
- Bestätigungen erscheinen im Aktivitätsprotokoll.
- Internet-Verlauf für den Provider-Nachweis: Latenz wird in 5-Minuten-Abschnitten (Median, Höchstwert, Verluste) so lange wie der übrige Verlauf aufbewahrt, Ausfälle bekommen ihre Art (Router antwortete nicht / Internet / DNS). Fiel der Router selbst aus, zählt das nicht gegen den Provider; DNS-Störungen werden nur notiert, nicht gemeldet.
- Leitungsdaten aus der FRITZ!Box (jede Minute gelesen, bei Änderung und stündlich gespeichert): Zugangsart, Sync-Raten, bei DSL die möglichen Raten, Neueinwahlen. Anzeige unter Einstellungen → Internet.
- Nachweis-Bericht für den Provider (Einstellungen → Internet): PDF mit Verfügbarkeit, allen Störungen nach Art, Latenz je Tag, Sync-Raten gegen die Vertragswerte (maximal/normalerweise/minimal) mit Diagrammen; dazu die Rohdaten als CSV, deren SHA-256 im PDF steht. Name, Kundennummer und Anschrift landen nur im PDF und werden nicht gespeichert. Der Bericht weist darauf hin, dass für eine Minderung nach § 57 TKG die Breitbandmessung der Bundesnetzagentur maßgeblich ist. Eigener kleiner PDF-Schreiber, im CI mit `qpdf --check` geprüft.
- Bekannte, aktiv ausgenutzte Lücken offline abgleichen: Netward bringt einen Schnappschuss des CISA-KEV-Katalogs mit den betroffenen Versionen aus den CVE-Datensätzen mit (iOS, iPadOS, macOS, tvOS; Stand im Gerätedetail, Warnung ab 90 Tagen). Bei genauer Version (z. B. tvOS 26.4 per AirPlay oder eine genaue eigene Angabe wie 18.6.2) gibt es einen kritischen Hinweis mit den CVEs und der Version, die sie behebt; bei eigenen Angaben stattdessen die Frage, ob die Version noch stimmt. Netward sagt nie „keine Lücken“.
- Eigene Systemangabe: optional die genaue Version.
- Personen statt Geräte: Personen anlegen, Geräte zuordnen, „Wer ist zu Hause“ auf der Übersicht („zu Hause seit …“ / „unterwegs seit …“). Die Zeit im Heimnetz je Tag ist pro Person einschaltbar und verlangt die Bestätigung, dass die Person Bescheid weiß. Home Assistant bekommt je Person auf Wunsch einen device_tracker (standardmäßig aus, mit derselben Bestätigung; IDs aus der Personennummer, nie aus dem Namen). Keine Kommen/Gehen-Benachrichtigungen.
- Verhaltens-Baseline (bewusst klein): Fragt ein bekanntes Gerät, das seit mindestens einer Woche da ist und zweimal mit seinem System gesehen wurde, zweimal wie eine andere Systemfamilie nach einer Adresse (z. B. Windows statt Apple), gibt es einen Hinweis mit „Neu eingerichtet“ und „Kenne ich nicht“. Neu offene Ports erscheinen im Aktivitätsprotokoll, wenn zwei Port-Scans desselben Umfangs übereinstimmen.
- WLAN-Diagnose aus der FRITZ!Box (Mesh-Liste über TR-064, alle 5 Minuten): Zugangspunkt, Band, Kanal, Signal in dBm und Datenrate je Gerät, Anbindung der Repeater. Hinweise wie „Laptop hängt im 2,4-GHz-Band am Repeater, obwohl es 5 GHz kann“, schwaches Signal oder schwach angebundener Repeater erscheinen erst nach drei Abfragen in Folge und benachrichtigen nicht. Neue Seite „WLAN“ (über Geräte) und Abschnitt „WLAN“ im Gerätedetail. Ohne Recht für die Mesh-Liste dient die einfache WLAN-Liste als Ersatz; die Geräteliste der FRITZ!Box ist davon nicht betroffen.

## 0.1.0 – 2026-10-26

### Neu
- Software und Support je Gerät: System, Version, Quelle, Support-Status, Handlungsempfehlung, Wissensstand mit Datum ([docs/08](docs/08-software-und-support.md)).
- Quellen:
  - mDNS: AirPlay, Apple-Geräteinfo, Synology, ESPHome, Shelly, Home Assistant
  - SSDP: FRITZ!OS, Sonos
  - DHCP: Android, dhcpcd, udhcp
  - FRITZ!Box TR-064: installiertes FRITZ!OS und verfügbares Update
  - Sicherheitscheck: SSH-Kennung und Windows-Version über SMB/NTLM
- Wissensbasis:
  - Windows-Builds inkl. Ausgaben
  - Apple-Systeme und Modellgrenzen
  - Android, Debian, Ubuntu, DSM, FRITZ!OS
- Eigene Angaben: System und Hauptversion für Handys/Tablets/Macs; Windows-Ausgabe.
- Befunde `software_outdated` und `software_update` mit Benachrichtigung (gebündelt ab vier), bewertet beim Start, täglich und nach Änderungen.
- Oberfläche:
  - Abschnitt „Software“ im Gerätedetail
  - Übersichtsbereich „Software und Updates“
  - Kennzeichen und Filter „Updates nötig“ in der Geräteliste
  - Texte auf der Sicherheitsseite und im Aktivitätsprotokoll
- Optionaler Hinweis auf neue Versionen (GitHub, standardmäßig aus).
- API:
  - `PUT /api/v1/devices/{id}/software`
  - `software` in Gerätedetail, Geräteliste und Übersicht
  - `update` und `softwareDataAsOf` in `/api/v1/system`
  - `POST /api/v1/auth/sessions/revoke-others`

### Geändert
- Image-Tags: `latest` nur noch aus Releases, `edge` für `main`, kein `:0`-Tag.
- Releases entstehen im CI: Die Datei `VERSION` auf `main` bestimmt die Version, die CI legt Tag, Image und GitHub-Release (aus `docs/releases/<tag>.md`) an.
- Container-Test prüft das Update vom letzten Release auf denselben Daten.
- Lizenzhinweis nennt die tatsächlich verwendeten Icons und Schriften.

### Behoben
- „Alle anderen abmelden“ brach bei einer bereits abgelaufenen Sitzung ab.
- Einstellungsseiten verglichen mit veralteten Werten.
- Aktivitätsliste der Übersicht lud nach einem Tab-Wechsel nicht nach.
- Herzschlag-Abfrage lief in versteckten Tabs und nach dem Abmelden weiter.
- Kleinere Darstellungsfehler (Internet-Kurve bei Totalausfall, Netzring bei vielen Geräten, Pluralformen).

### Datenbank
- Migration 2: Index `presence(ended_at)`.
- Migration 3: Tabelle `software`.
- Vor jeder Migration wird `backup/pre-update-schema-N.db` angelegt.

## Vor 0.1.0

Entwicklungsstände ohne Versionsnummer (Image-Tag `latest` bis 26.10.2026): Grundfunktionen, Redesign „Leitstand“, Reviews 1–4.
