# 08 · Software und Support (Version 0.1)

Stand: 26.10.2026 · Status: Plan, zur Prüfung

## 1. Ziel

Bisher zeigt Netzwart, **wer** im Netz ist und **wann**. Nutzerrückmeldung: „So wirklich einen Nutzen habe ich daraus nicht.“
Version 0.1 beantwortet die Frage, die im Heimnetz tatsächlich zählt:

> **Welche Geräte bekommen keine Sicherheitsupdates mehr – und was mache ich jetzt?**

Dazu zeigt Netzwart je Gerät, **welche Software** läuft (soweit das Gerät es verrät), **ob sie noch unterstützt** wird, woher diese Information stammt und was zu tun ist.
Ein Gerät ohne Updates wird zum Sicherheitsbefund mit Benachrichtigung, genau wie ein offener Telnet-Port.

## 2. Was technisch möglich ist – und was nicht

Ehrlichkeit vor Vollständigkeit: Eine falsche Entwarnung ist schlimmer als „unbekannt“.

| Gerät | Was ein Netzwerkscanner sehen kann | Ergebnis |
|---|---|---|
| **iPhone / iPad** | Apple gibt die iOS-Version im Netz bewusst nicht preis (private MAC, keine Versions-TXT). Sichtbar ist manchmal das **Modell** (z. B. `iPhone11,2` über `_device-info`, FRITZ!Box-Namen, Hostname). | Aus dem Modell folgt die **höchste installierbare iOS-Version**: Ein iPhone X kann höchstens iOS 16 → bekommt sicher keine Updates mehr. Die genaue installierte Version bleibt unbekannt. |
| **Apple TV, HomePod, Mac** | AirPlay meldet `osvers=` (tvOS/audioOS/macOS), Macs melden `osxvers=` (Darwin-Version) und das Modell. | Version + Status |
| **Windows-PC** | Die Windows-Anmeldung über SMB (Port 445) nennt beim Verbindungsaufbau ohne Zugangsdaten die **Build-Nummer** (NTLM-Challenge, wie `nmap smb-os-discovery`). | z. B. „Windows 10 22H2 (Build 19045)“ → seit 13.10.2026 auch ohne ESU keine Updates mehr |
| **Android** | Der DHCP-Client meldet `android-dhcp-14`. | Android-Version + Status (Google patcht die vier neuesten Versionen) |
| **Linux, Raspberry Pi, NAS** | SSH-Kennung (`SSH-2.0-OpenSSH_9.2p1 Debian-2+deb12u7`), DHCP-Kennung von `dhcpcd` mit Kernelversion, Synology-mDNS mit DSM-Version. | Distribution + Version + Status |
| **FRITZ!Box** | SSDP-Kennung enthält die FRITZ!OS-Version; mit FRITZ!Box-Benutzer meldet TR-064 zusätzlich, **ob ein Update bereitsteht**. | Version + „Update verfügbar“ aus erster Hand |
| **Smart Home** | ESPHome (`version=`), Shelly Gen2 (`ver=`), Sonos (SSDP-Kennung) | Version (nur Anzeige, kein Urteil ohne verlässliche Daten) |
| **Fernseher, Kameras, Drucker, IoT allgemein** | Meist nichts Verwertbares. | „Unbekannt – das Gerät verrät keine Version“ |

**Nicht** Teil von 0.1: CVE-Abgleich einzelner Programmversionen. Distributionen spielen Sicherheitskorrekturen ein, ohne die Versionsnummer zu ändern (Debian-OpenSSH 9.2p1 ist gepatcht). Ein Versionsvergleich würde massenhaft Fehlalarme erzeugen. Bewertet wird deshalb das **Support-Ende des Betriebssystems bzw. der Firmware**, nicht einzelne Lücken.

## 3. Quellen

### 3.1 Passiv (läuft ohnehin, keine zusätzlichen Pakete)

| Quelle | Feld | Produkt |
|---|---|---|
| mDNS `_airplay._tcp` | `osvers`, `model` | tvOS, audioOS, macOS |
| mDNS `_device-info._tcp` | `model`, `osxvers` | Modell (Apple), macOS über Darwin-Version |
| mDNS `_http._tcp` (Synology) | `vendor`, `version_major/minor/build` | DSM |
| mDNS `_esphomelib._tcp` | `version` | ESPHome |
| mDNS `_shelly._tcp` | `ver`, `gen` | Shelly-Firmware |
| SSDP `SERVER` | Produktkennung | FRITZ!OS, Sonos, Synology, Linux-Kernel (nur Anzeige) |
| UPnP-Beschreibung | `softwareVersion`, `firmwareVersion`, `displayVersion` | Firmware (Anzeige) |
| DHCP Option 60 | `android-dhcp-N`, `dhcpcd-x:Linux-k`, `udhcp x` | Android, Linux-Kernel, BusyBox |
| FRITZ!Box TR-064 (optional, vorhandener Benutzer) | `UserInterface:GetInfo` → `NewUpgradeAvailable`, `NewX_AVM-DE_Version`; `DeviceInfo:GetInfo` → `NewSoftwareVersion` | FRITZ!OS + Update verfügbar |

Gespeichert werden nur die benannten Felder. Seriennummern und Kennungen in denselben TXT-Records (z. B. `serial=`, `deviceid=`) werden **nicht** übernommen.

### 3.2 Aktiv (nur mit der Sicherheitsprüfung: auf Knopfdruck oder per opt-in-Zeitplan)

| Probe | Ablauf | Gesendete Daten |
|---|---|---|
| **SSH-Kennung** (Port 22) | TCP-Connect, erste Zeile lesen (max. 255 Byte, 3 s), schließen | **keine** – der Server spricht zuerst |
| **SMB/NTLM** (Port 445, nur wenn offen) | SMB2-`NEGOTIATE`, dann `SESSION_SETUP` mit NTLMSSP-`NEGOTIATE` (ohne Benutzer, ohne Passwort); aus der `CHALLENGE` wird nur das Versionsfeld gelesen; Verbindung schließen | Standard-Handshake ohne Zugangsdaten; keine Anmeldung, kein Dateizugriff |

Die Proben laufen mit denselben Grenzen wie die Portprüfung (max. 4 Geräte parallel, kurze Timeouts) und nur gegen Geräte im überwachten Subnetz.
F-21 („ausschließlich TCP-Connect ohne Nutzdaten“) wird für den SMB-Handshake ausdrücklich erweitert (siehe §7).

### 3.3 Modell-Wissen

Aus der Apple-Modellkennung (`iPhone11,8`, `iPad7,11`, `AppleTV6,2`, `Mac14,2` …) folgt die **höchste installierbare Hauptversion**.
Das ist bei iPhones meist die einzige verfügbare Information und reicht für die wichtigste Aussage: „kann nicht mehr aktualisiert werden“.

## 4. Bewertung

### 4.1 Wissensbasis

Eingebettete Datei `internal/software/lifecycle.json`. Sie hat einen sichtbaren **Datenstand** („Stand: Oktober 2026“) und wird mit jedem Release aktualisiert.
Es gibt **keinen** Online-Abgleich (N-04).

Inhalt 0.1 (recherchiert am 26.10.2026, Quellen im Anhang):

| Produkt | Regel |
|---|---|
| iOS / iPadOS | Aktuell: 27; Vorversion 26 erhält Sicherheitsupdates. Geräte, die höchstens iOS 18 können (XS/XR), erhalten noch Korrekturen im 18er-Zweig (zuletzt 18.7.10, Aug. 2026) → **Hinweis „älterer Zweig, Ende absehbar“**. Höchstens iOS 17 oder älter → **keine Updates**. |
| macOS | Unterstützt: 27, 26, 15 (aktuelle + zwei Vorgänger). 14 und älter → keine Updates. Intel-Macs: höchstens macOS 26 → Hinweis. |
| tvOS / audioOS | Aktuell 27, Vorversion 26; älter → Hinweis bzw. keine Updates, analog iOS |
| Windows 10 | Keine Updates mehr (Ende 14.10.2025, Verbraucher-ESU endete 13.10.2026) |
| Windows 11 | Build 22000 (21H2), 22621 (22H2), 22631 (23H2), 26100 (24H2, Home/Pro bis 13.10.2026) → keine Updates; 26200 (25H2, bis 12.10.2027) und 26300 (26H2) → unterstützt; Builds über dem Datenstand gelten als unterstützt. Fristen für Home/Pro; Enterprise/Server haben längere Fristen → Hinweistext. |
| Android | 14–17 unterstützt; 13 und älter → keine Updates (Android 13 seit 2.3.2026). Hinweis: Ob ein Handy die Patches bekommt, entscheidet zusätzlich der Hersteller. |
| Debian | 13 unterstützt; 12 in LTS bis 30.06.2028 → unterstützt; 11 und älter → keine Updates (11 seit 31.08.2026) |
| Ubuntu | 24.04, 22.04 (bis 04/2027) unterstützt; 20.04 und älter → keine Standard-Updates |
| Synology DSM | 7.x → unterstützt (keine verlässlichen Daten zu 7.1); 6.x und älter → keine Updates (6.2 seit 10/2024) |
| FRITZ!OS | Keine Tabelle: maßgeblich ist die Aussage der FRITZ!Box selbst („Update verfügbar“) |
| ESPHome, Shelly, Sonos, Kernel | nur Anzeige, kein Urteil |

### 4.2 Ergebnis je Gerät

```
status:  supported | aging | outdated | update | unknown
product: "Windows 11"   version: "24H2 (Build 26100)"   since: 2026-10-13
source:  "smb" | "ssh" | "airplay" | "device-info" | "dhcp" | "ssdp" | "upnp" | "fritzbox" | "model"
evidence: kurzer Rohwert zur Nachvollziehbarkeit (z. B. "osvers=18.2", "SSH-2.0-OpenSSH_9.2p1 Debian-2+deb12u7")
```

- `outdated` – bekommt keine Sicherheitsupdates mehr → Sicherheitsbefund **Warnung** + Benachrichtigung
- `aging` – unterstützt, aber Ende absehbar (z. B. iOS-18-Zweig, Intel-Mac) → Befund **Hinweis**
- `update` – das Gerät meldet selbst ein verfügbares Update (FRITZ!Box) → Befund **Hinweis**
- `supported` / `unknown` → kein Befund

Bei mehreren Quellen gewinnt die genaueste (aktive Probe > TXT-Version > DHCP > Modell); widersprechen sich Quellen, gewinnt die jüngste Beobachtung.
Die Bewertung wird **beim Abruf** berechnet, nicht gespeichert. Ein Update mit neuer Wissensbasis bewertet also sofort alle Geräte neu.

## 5. Oberfläche

1. **Gerätedetail**: neuer Abschnitt „Software“ direkt unter dem Kopf: Produkt und Version, Status in Klartext („Bekommt seit 13. Oktober 2026 keine Sicherheitsupdates mehr“), „Was kann ich tun?“, „Erkannt über …“ mit Rohwert, Datenstand. Bei `unknown` ein Satz, warum (z. B. „iPhones verraten ihre iOS-Version nicht im Netz“).
2. **Geräteliste**: Kennzeichen „VERALTET“ (wie „NEU“) und ein Filter „Veraltet“.
3. **Sicherheit**: neue Befundart „Keine Sicherheitsupdates mehr“ bzw. „Update verfügbar“, mit Erklärung und Handlungsempfehlung je Produkt (Windows: Update auf 25H2 oder Windows 11; iPhone: Gerät austauschen oder nicht mehr für Banking/Passwörter nutzen …).
4. **Übersicht**: „Braucht Aufmerksamkeit“ führt veraltete Geräte zusätzlich zu neuen Geräten auf.
5. **Einstellungen → System**: installierte Version, Datenstand der Wissensbasis, optional „Nach neuen Versionen suchen“ (siehe §6).

## 6. Versionen und Updates

- Erste nummerierte Version: **0.1.0**. Git-Tag `v0.1.0` → CI baut und signiert `ghcr.io/pyzyi/netzwart:0.1.0`, `:0.1` sowie `:latest` (über `main`).
- **Update ohne Neuinstallation**: Die Daten liegen im Volume, Datenbank-Migrationen laufen beim Start automatisch. Update = neues Image ziehen und Container neu erstellen (ZimaOS-Oberfläche oder `docker compose pull && docker compose up -d`). Konten, Geräte, Namen, Verlauf und Einstellungen bleiben erhalten.
- Zurück auf eine ältere Version verweigert Netzwart bewusst (Schema-Schutz). Der Rückweg ist dokumentiert: Backup des Volumes vor dem Update.
- **Release-Notizen** auf GitHub (Deutsch) und `CHANGELOG.md`.
- **Optional: Hinweis auf neue Versionen.** Einstellung „Nach neuen Versionen suchen“, **standardmäßig aus** (N-04). Ist sie an, fragt Netzwart einmal täglich `https://api.github.com/repos/Pyzyi/netzwart/releases/latest` über den Egress-Client ab (ohne Kennung, ohne Cookies) und zeigt „Version x.y verfügbar“ mit Link zu den Release-Notizen. Netzwart **installiert nichts selbst**: Ein Container, der sich selbst aktualisiert, bräuchte Zugriff auf den Docker-Socket. Das wäre ein erheblich größeres Risiko als der Nutzen.

## 7. Sicherheit (Ergänzungen zum Bedrohungsmodell)

| ID | Bedrohung | Maßnahme |
|---|---|---|
| T-30 | Gefälschte Versionsangaben (mDNS/SSDP/DHCP/SSH sind spoofbar) führen zu falscher Entwarnung oder falschem Alarm | Angaben werden nur dem Gerät mit derselben IP/MAC zugeordnet; die Quelle wird immer angezeigt; kein automatisches Handeln, nur Anzeige und Befund. Restrisiko: ein Angreifer im LAN kann sein eigenes Gerät „aktuell“ aussehen lassen. Das ist akzeptiert, weil er damit nur sich selbst tarnt. |
| T-31 | Parser-Fehler bei feindlichen Eingaben (SSH-Zeile, NTLM-Nachricht, TXT, SERVER-Header) | Reine Parser-Funktionen mit Längenbegrenzung, Fuzz-Tests in CI; NTLM: feste Offsets mit Bereichsprüfung, max. 4 KiB lesen |
| T-32 | Aktive Proben stören Geräte oder lösen Sperren aus | SSH: nichts senden; SMB: kein Anmeldeversuch (NTLM-Challenge wird nie beantwortet) – erzeugt kein „fehlgeschlagene Anmeldung“-Ereignis; nur auf Anforderung oder per Zeitplan; 4 parallel, 3 s Timeout |
| T-33 | Sensible Daten in TXT-Records (Seriennummern, Geräte-IDs) | Whitelist der übernommenen Schlüssel; Rohwert gekürzt auf 120 Zeichen und bereinigt (`textutil.Clean`) |
| T-34 | Update-Abfrage verrät Nutzung an GitHub | standardmäßig aus; nur ein GET ohne Kennung; Ziel fest im Code; Egress-Dialer; Antwort max. 64 KiB, nur `tag_name` und `html_url` (nur `https://github.com/Pyzyi/netzwart/…`) übernommen |

Anforderungen: F-30 (Software und Support je Gerät), F-31 (Befund „Veraltete Software“), F-32 (Hinweis auf neue Versionen, opt-in); N-04 wird um F-32 ergänzt.

## 8. Datenmodell

Migration 3:

```sql
CREATE TABLE software (
  device_id INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  source    TEXT NOT NULL,      -- smb, ssh, airplay, device-info, synology, esphome, shelly, dhcp, ssdp, upnp, fritzbox
  product   TEXT NOT NULL,      -- windows, ios, macos, tvos, audioos, android, debian, ubuntu, dsm, fritzos, ...
  version   TEXT NOT NULL,
  evidence  TEXT NOT NULL DEFAULT '',
  seen_at   INTEGER NOT NULL,
  PRIMARY KEY (device_id, source)
);
```

Beim Zusammenführen von Geräten (F-17) werden die Einträge übernommen. Beim Löschen eines Geräts verschwinden sie mit.

## 9. Tests und Abnahme

- Parser: Tabellentests mit echten Kennungen (FRITZ!Box, Sonos, Synology, Debian/Ubuntu/Raspbian-SSH, dropbear, `android-dhcp-14`, `dhcpcd`, AirPlay-TXT) und Fuzzing (SSH, NTLM, SSDP-Server).
- SMB/NTLM: Test gegen einen Fake-SMB-Server im Test (aufgezeichnete Antwort von Windows 10 und 11).
- Bewertung: Tabellentests je Regel aus §4.1, inklusive „Build neuer als Datenstand → unterstützt“.
- API und UI: Gerätedetail mit Software-Abschnitt, Filter „Veraltet“, Befund auf der Sicherheitsseite (Playwright, Demo-Daten).
- Update-Pfad: Container-Test startet das alte Image, legt Daten an, startet das neue Image auf demselben Volume → Daten vorhanden, Migration 3 angewendet.
- Abnahme: Demo zeigt mindestens einen Windows-10-PC (veraltet), ein altes iPad (veraltet), ein iPhone XS (älterer Zweig), eine FRITZ!Box mit verfügbarem Update und mehrere unterstützte Geräte.

## 10. Nicht in 0.1

CVE-Datenbank, Online-Aktualisierung der Wissensbasis, Firmware-Tabellen für Fernseher/Kameras, automatische Updates des Containers, Agent auf Endgeräten.

## Anhang: Quellen der Wissensbasis (Abruf 26.10.2026)

- iOS 27 unterstützt weiterhin iPhone 11 und neuer: AppleInsider, 8.6.2026
- iOS 18.7.10 für iPhone XS/XR, 17.8.2026: iClarified
- macOS 27 nur Apple Silicon, macOS 26 letzte Version für Intel: AppleInsider, 8.6.2026
- Windows 11 24H2 Home/Pro Ende 13.10.2026; 25H2 bis 12.10.2027; 26H2 = Build 26300: BleepingComputer, Pureinfotech, Techdows
- Android 13 ohne Sicherheitsupdates seit 2.3.2026; Bulletins decken 14–17 ab: heise, teltarif
- Debian 11 LTS Ende 31.08.2026, Debian 12 in LTS bis 30.06.2028: debian.org
- Synology DSM 6.2 Ende 10/2024: ComputerBase
- FRITZ!OS 8.50 seit 29.9.2026 (zunächst 7690): PC-WELT
