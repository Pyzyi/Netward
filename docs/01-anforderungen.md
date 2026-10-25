# 01 · Anforderungen

> Status: **überarbeitet nach Plan-Review 1** (siehe [07](07-review-protokoll.md)) · Rolle: Product Owner
> Bezug: [ISO/IEC 25010:2023 Qualitätsmodell](04-qualitaetsmodell-iso25010.md)

## 1. Produktvision

**Netzwart** ist ein selbst gehosteter Netzwerk-Wächter für Heim- und Kleinbüro-Netze.
Er läuft als **ein einziger Docker-Container** auf einem Heimserver (Zielplattform: ZimaOS),
erkennt jedes Gerät im lokalen Netz, benennt es verständlich, zeigt, wer gerade online ist,
und meldet sich, wenn etwas Unerwartetes passiert.

Leitsatz: *So einfach wie Fing, so offen wie NetAlertX, sicherer als beide.*

## 2. Wettbewerbsanalyse

| Kriterium | Fing (App / Agent) | NetAlertX | **Netzwart (Ziel)** |
|---|---|---|---|
| Kosten | Basis gratis, Agent/Premium kostenpflichtig (Abo) | gratis, Open Source | gratis, Open Source (MIT) |
| Betrieb | Cloud-Konto nötig, Daten bei Fing | self-hosted | self-hosted, **keine Cloud, keine Telemetrie** |
| Einrichtung | sehr einfach | viele Plugins/Settings, steile Lernkurve | **Compose einfügen → Passwort setzen → fertig** |
| Geräteerkennung | sehr gut (Cloud-Datenbank) | gut (arp-scan, nmap, Plugins) | gut, rein lokal: OUI + DHCP-Fingerprint + mDNS + SSDP + NetBIOS + Ports |
| Echtzeit-Erkennung neuer Geräte | ja (Fingbox/Agent) | nur zyklisch | **ja: passives Mithören von ARP/DHCP + zyklischer Sweep** |
| Anwesenheitsverlauf | ja | ja | ja, Zeitleiste 24 h / 7 Tage / 30 Tage |
| Benachrichtigungen | Push über Fing-App | sehr viele (Apprise u. a.) | ntfy, Gotify, Pushover, Telegram, Discord, Slack/Mattermost, Webhook, E-Mail, MQTT |
| Home Assistant | nein | MQTT | **MQTT Discovery (device_tracker + Sensoren)** |
| Sicherheitscheck | ja (Router-Ports, offene Ports) | via nmap-Plugin | eingebaut: riskante Dienste, UPnP, IP-Konflikte, unbekannte Geräte |
| Internet-Ausfälle | ja | nein | ja, Latenz und Ausfallhistorie |
| Wake-on-LAN | ja | ja | ja |
| Zugangsschutz | Fing-Konto | Passwort, **Standardpasswort ab Werk** | Pflicht-Einrichtung, Argon2id, Brute-Force-Schutz, CSRF, DNS-Rebinding-Schutz, CSP |
| Container-Rechte | Agent: privilegiert/host | root + host-Netz | **nicht-root, zur Laufzeit keine Capabilities, read-only FS, keine Shell im Image** |
| FRITZ!Box | nein | Plugin | eingebaut (optional, TR-064) |
| Oberfläche | modern, mobil | funktional, überladen | modern, ruhig, responsiv, Dark Mode, DE/EN, installierbar (PWA) |

### Bewusst *nicht* übernommen

- **Geräte sperren / Internet pausieren** (Fingbox). Das funktioniert nur per ARP-Spoofing.
  Das ist ein Angriff auf das eigene Netz: Es destabilisiert Verbindungen, wird von
  Sicherheitssoftware erkannt und erweitert die Angriffsfläche erheblich. Stattdessen verlinkt
  Netzwart auf die Sperrfunktion des Routers (z. B. FRITZ!Box-Kindersicherung).
- **Cloud-Geräteerkennung**. Sie würde Daten aus dem Heimnetz an Dritte senden.
- **Speedtest**. Er erzeugt Last und braucht externe Server; Internet-Monitoring misst stattdessen Latenz und Ausfälle.

## 3. Personas

- **Jonas, Heimserver-Besitzer** – betreibt ZimaOS mit Diensten für die Familie. Will wissen, welche
  Geräte im Netz sind, und eine Nachricht bekommen, wenn ein fremdes Gerät auftaucht.
  Wenig Zeit, kein Interesse an Konfigurationsdateien.
- **Anna, Technik-Interessierte** – nutzt Home Assistant und will Anwesenheit („Handy zuhause?“)
  für Automationen nutzen.
- **Kleinbüro-Admin** – braucht Inventar, Export und Sicherheitsüberblick für 20–200 Geräte.

## 4. Funktionale Anforderungen (MoSCoW)

IDs werden in Code, Tests und Review-Protokoll referenziert.

### Must

| ID | Anforderung | Akzeptanzkriterium |
|---|---|---|
| F-01 | Automatische Erkennung aller IPv4-Geräte im lokalen Subnetz per ARP-Sweep | /24-Netz wird in < 10 s vollständig gescannt; jedes antwortende Gerät erscheint mit IP und MAC |
| F-02 | Passive Erkennung über ARP- und DHCP-Verkehr | Ein neu verbundenes Gerät erscheint in der Regel innerhalb weniger Sekunden nach seiner DHCP-Anfrage |
| F-03 | Herstellererkennung über IEEE-OUI-Datenbank (offline, eingebettet) | Hersteller wird für registrierte MAC-Präfixe angezeigt; zufällige (private) MACs werden als solche markiert |
| F-04 | Namensauflösung über DHCP-Hostname, mDNS, NetBIOS, SSDP/UPnP, Reverse-DNS | Bester verfügbarer Name wird nach fester Priorität angezeigt, alle Quellen sind im Detail sichtbar |
| F-05 | Automatische Gerätetyp-Klassifizierung (≥ 20 Typen) | Typ und Icon werden aus Signalen abgeleitet; Nutzer kann überschreiben |
| F-06 | Online/Offline-Status mit konfigurierbarer Schwelle | Gerät gilt nach `offline_after` (Standard 5 min, mobil 15 min) ohne Lebenszeichen **und** ohne Antwort auf Weck-Proben als offline |
| F-07 | Anwesenheitsverlauf je Gerät | Zeitleiste 24 h / 7 d / 30 d, Online-Anteil in % der beobachteten Zeit; Zeiten ohne Überwachung sind sichtbar markiert |
| F-08 | Ereignisprotokoll | Neues Gerät, online/offline (beobachtete Geräte), IP-Wechsel, IP-Konflikt, Internet-Ausfall, Sicherheitsbefunde |
| F-09 | Gerät bearbeiten | Name, Typ, Ort/Raum, Notizen, „bekannt“, Favorit, „beobachten“ |
| F-10 | Benachrichtigung bei neuem, unbekanntem Gerät | Auslösung ≤ 1 Scan-Zyklus nach Erkennung; der erste Scan nach Installation löst nichts aus (Lernphase); Flutschutz (max. 20 neue Geräte/min); private MACs abschaltbar |
| F-11 | Benachrichtigungskanäle | ntfy, Gotify, Pushover, Telegram, Discord, Slack/Mattermost, generischer Webhook, E-Mail (SMTP); Testnachricht pro Kanal |
| F-12 | Ersteinrichtung mit Admin-Konto | Kein Standardpasswort; ohne Einrichtung ist keine Funktion außer Setup erreichbar |
| F-13 | Anmeldung, Abmeldung, Passwort ändern, Sitzungen verwalten | Sitzungen sind einzeln widerrufbar |
| F-14 | Passwort-Reset ohne Datenverlust | `docker exec netzwart /netzwart reset-password` **oder** Datei `reset-password` im AppData-Ordner anlegen und neu starten: Das Einmalpasswort steht im Log. `factory-reset` entfernt Konten, Kanäle und Einstellungen |
| F-15 | Einstellungen in der Oberfläche | Interfaces/Subnetze, Scan-Intervall, Offline-Schwelle, Sprache, Theme, Aufbewahrungsdauer |
| F-16 | Live-Aktualisierung der Oberfläche | Änderungen erscheinen ohne Neuladen (Server-Sent Events) |
| F-17 | Geräte zusammenführen | Zwei Einträge (z. B. nach Wechsel der privaten MAC) lassen sich zu einem vereinen; Name, Notizen und Verlauf bleiben erhalten |
| F-18 | Repeater mit MAC-NAT | Geräte hinter einem Repeater mit gemeinsamer MAC werden einzeln (je IP) geführt, nicht zu einem verschmolzen |

### Should

| ID | Anforderung | Akzeptanzkriterium |
|---|---|---|
| F-20 | Port-Scan je Gerät (TCP connect, Top-Ports) | Ergebnis ≤ 15 s, Dienstname je Port |
| F-21 | Sicherheitscheck | Findet riskante Dienste (Telnet, FTP, VNC, RDP, SMB, MQTT, Redis …), UPnP-IGD am Router, IP-Konflikte und unbekannte Geräte; jeder Befund mit verständlicher Handlungsempfehlung, quittierbar („akzeptiert“). Portprüfung nur auf Anforderung oder per opt-in-Zeitplan; ausschließlich TCP-Connect ohne Nutzdaten |
| F-22 | Internet-Monitor | Verfügbarkeit, aktuelle Latenz, Ausfallhistorie; Gateway wird zuerst geprüft (LAN- vs. WAN-Ausfall); Benachrichtigung nach Wiederherstellung mit Dauer; abschaltbar |
| F-23 | Wake-on-LAN | Magic Packet an Gerät senden |
| F-24 | Ping je Gerät | Ergebnis mit Latenz |
| F-25 | Home-Assistant-Integration per MQTT Discovery | Beobachtete Geräte erscheinen als `device_tracker` (retained, mit Verfügbarkeit/LWT, neu veröffentlicht bei HA-Neustart), dazu Sensoren für Gerätezahl und Internetstatus |
| F-26 | Export CSV/JSON | Export ist formel-injektionssicher und enthält keine Geheimnisse |
| F-27 | Zweisprachig DE/EN, Hell/Dunkel/System | Sprache und Theme lassen sich umschalten |
| F-28 | Zum Startbildschirm hinzufügbar | Manifest und Icons vorhanden; iOS „Zum Home-Bildschirm“ funktioniert auch per HTTP, Chrome/Android-Installation nur über HTTPS |
| F-29 | FRITZ!Box-Anbindung (optional) | Mit FRITZ!Box-Benutzer liefert TR-064 Gerätenamen und WLAN-Anmeldestatus; verbessert Namen und Anwesenheit schlafender Handys |

### Could (Roadmap)

- Import von Gerätemetadaten (JSON), Latenzhistorie als Diagramm
- DHCP-Lease-Import aus Pi-hole, AdGuard Home, OpenWrt, UniFi
- IPv6-Nachbarerkennung, VLAN/geroutete Subnetze per ICMP
- Netzwerkkarte (Topologie-Ansicht)
- Mehrere Benutzer mit Rollen (Lesen/Admin)
- OIDC/Proxy-Auth

### Won't (bewusst ausgeschlossen)

- ARP-Spoofing-basierte Gerätesperre, Cloud-Abgleich, Speedtest (Begründung siehe §2)

## 5. Nicht-funktionale Anforderungen (Kurzfassung)

Die vollständige Zuordnung zu ISO/IEC 25010 steht in [04-qualitaetsmodell-iso25010.md](04-qualitaetsmodell-iso25010.md).

| ID | Anforderung |
|---|---|
| N-01 | Installation ausschließlich per `docker compose` (copy & paste), ohne weitere Dateien |
| N-02 | Image ≤ 40 MB, RAM im Leerlauf ≤ 60 MB (Spitze beim Login ≤ 90 MB), CPU < 1 % im Leerlauf und < 3 % eines Kerns während SMB-Übertragung auf Celeron-Klasse (ZimaBoard) |
| N-03 | Läuft auf `linux/amd64` und `linux/arm64` |
| N-04 | Keine ausgehenden Verbindungen außer konfigurierten Benachrichtigungen und dem (abschaltbaren) Internet-Monitor |
| N-05 | Prozess läuft nicht als root und hat zur Laufzeit **keine** Capabilities (Raw-Sockets werden vor dem Rechteverzicht geöffnet) |
| N-06 | Oberfläche zielt auf WCAG 2.2 AA (Kontrast automatisch geprüft, Tastatur, Fokus, Semantik) |
| N-07 | Daten bleiben über Container-Updates erhalten; Schema-Migrationen laufen automatisch |
