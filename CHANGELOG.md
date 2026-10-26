# Änderungen

Alle nennenswerten Änderungen an Netzwart. Versionen folgen [Semantic Versioning](https://semver.org/lang/de/);
vor 1.0 können sich Einzelheiten noch ändern. Die Release-Notizen auf GitHub stehen in [`docs/releases`](docs/releases).

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
