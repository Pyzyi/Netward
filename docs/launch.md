# Netzwart bekannt machen

Ein Leitfaden in der Reihenfolge, in der sich die Schritte lohnen. Alles, was im Repository liegen kann, ist
vorbereitet. Was nur der Kontoinhaber kann (Einstellungen auf GitHub, Beiträge in Foren, Anträge bei Stores), steht
hier als Schritt mit fertigem Text.

## 1. GitHub-Seite einrichten (10 Minuten, geht am Handy)

Die meisten Besucher entscheiden in wenigen Sekunden anhand von Beschreibung, Vorschaubild und README.

1. **Beschreibung und Themen:** Auf der Repo-Startseite rechts bei „About“ auf das Zahnrad tippen.
   - Beschreibung:
     `Self-hosted home network monitor: see every device, who is online, new devices, outages and security issues. No cloud. ZimaOS, CasaOS, Unraid, TrueNAS, Docker.`
   - Website: `https://github.com/Pyzyi/netzwart#readme`
   - Topics: `self-hosted selfhosted homelab network-monitoring network-scanner device-discovery home-network
     presence-detection home-assistant docker zimaos casaos unraid truenas fritzbox security golang svelte`
   - Alternativ am Rechner: `sh scripts/github-repo-setup.sh` (setzt alles auf einmal, schaltet Discussions und private Sicherheitsmeldungen ein und schützt die Release-Tags vor Verschieben und Löschen).
2. **Vorschaubild:** Settings → General → Social preview → [`docs/social-preview.png`](social-preview.png)
   hochladen. Das Bild erscheint, wenn der Link auf Reddit, Mastodon, Discord oder in Messengern geteilt wird.
3. **Discussions einschalten:** Settings → General → Features → Discussions. Die Issue-Vorlagen verweisen für
   Fragen dorthin.
4. **Release-Notizen auf Englisch:** Die Release-Seite ist für viele der erste Eindruck. Ab dem nächsten Release
   `docs/releases/vX.Y.Z.md` mit einem englischen Abschnitt beginnen und den deutschen darunter stellen.

## 2. App-Stores

Siehe [`deploy/README.md`](../deploy/README.md): CasaOS/ZimaOS (Pull Request), Unraid Community Applications
(eigenes Vorlagen-Repo + Formular), TrueNAS (Pull Request, braucht einen Rechner mit Docker). Ein Store-Eintrag
bringt dauerhaft Nutzer, weil dort gesucht wird. Ein Forenbeitrag bringt dagegen nur einmal Aufmerksamkeit.
Darum zuerst die Stores, dann die Beiträge.

## 3. Verzeichnisse und Newsletter

| Wo | Was | Hinweis |
|---|---|---|
| [selfh.st](https://selfh.st) | wöchentlicher Newsletter und App-Verzeichnis | Projekt über das Formular auf der Seite einreichen |
| [awesome-selfhosted](https://github.com/awesome-selfhosted/awesome-selfhosted-data) | die bekannteste Liste | verlangt meines Wissens, dass das erste Release mindestens 4 Monate alt ist (also ab etwa März 2027); Richtlinien vorher prüfen |
| [AlternativeTo](https://alternativeto.net) | als Alternative zu Fing und NetAlertX eintragen | Leute suchen dort gezielt nach „Fing alternative“ |
| [Docker Hub](https://hub.docker.com) | Spiegel des Images | Viele suchen nur dort; optional, wenn die CI zusätzlich dorthin veröffentlicht |

## 4. Beiträge in Communities

Am besten wirkt ein Beitrag mit einem Bild (Netzring oder Handy-Screenshot), ehrlichen Grenzen
(Host-Netzwerk nötig, FRITZ!Box-Funktionen nur mit FRITZ!Box) und schnellen Antworten in den ersten Stunden.
Vor dem Posten die Regeln der jeweiligen Community lesen: Manche erlauben eigene Projekte nur an bestimmten Tagen
oder in Sammelthreads.

| Community | Sprache | Passender Aufhänger |
|---|---|---|
| r/selfhosted | EN | Fing-Alternative ohne Cloud, Sicherheitscheck |
| r/homelab | EN | Netzring-Bild, Unraid/TrueNAS-Installation |
| r/unRAID, r/truenas | EN | sobald die Vorlage/App verfügbar ist |
| ZimaOS-Community (Forum/Discord) | EN | „built for ZimaOS“, Installation per Import |
| Home Assistant Community → „Share your Projects“ | EN | Personen als `device_tracker`, MQTT |
| Hacker News „Show HN“ | EN | Technik: ein statisches Binary, keine Capabilities, kein Root |
| ComputerBase-Forum, Hardwareluxx | DE | Heimnetz-Überblick, Sicherheitscheck |
| IP-Phone-Forum (FRITZ!Box) | DE | WLAN-Diagnose, Leitungsdaten, Nachweis für den Provider |

### Entwurf r/selfhosted / r/homelab

```text
Title: Netzwart – a self-hosted network monitor that tells you what's on your LAN (and what's risky)

I built Netzwart because I wanted Fing-like visibility of my home network without a cloud account.

- Finds every device (ARP, DHCP, mDNS, UPnP) and gives it a readable name and icon
- "Who is at home", presence timeline per device, Home Assistant via MQTT
- Security check with plain advice: open Telnet/RDP/SMB, devices without security updates,
  a second DHCP server, ARP spoofing of the router address, known exploited iOS/macOS bugs
- Internet monitor with an outage report you can send to your ISP
- Notifications via ntfy, Gotify, Pushover, Telegram, Discord, e-mail …

It runs as a ~23 MB container. It needs the host network for layer-2 discovery, but drops every
Linux capability right after start-up and runs read-only as an unprivileged user.
amd64 + arm64, MIT licensed, no telemetry.

Install: docker compose, ZimaOS/CasaOS import, Unraid template, TrueNAS "Install via YAML".
GitHub: https://github.com/Pyzyi/netzwart

Happy to hear what's missing for your setup.
```

### Entwurf „Show HN“

```text
Show HN: Netzwart – self-hosted home network monitor without root or capabilities at runtime

Netzwart discovers devices on a home LAN (passive ARP/DHCP/mDNS plus periodic sweeps), names them
from DHCP fingerprints, mDNS, UPnP and the IEEE registry, and runs a defensive security check
(risky open services, end-of-life OS versions, rogue DHCP servers, ARP spoofing of the gateway).

The part I found interesting: layer-2 discovery needs host networking and raw sockets, but the
process opens its two sockets as root, chowns the data directory and then drops to an unprivileged
user with an empty capability set (CapEff = 0). The image is a single static Go binary with no shell.

https://github.com/Pyzyi/netzwart
```

### Entwurf ComputerBase / IP-Phone-Forum

```text
Titel: Netzwart – Heimnetz-Überblick und Sicherheitscheck, selbst gehostet (Open Source)

Ich habe ein kleines Werkzeug gebaut, das alle Geräte im Heimnetz findet, verständlich benennt und
meldet, wenn etwas Neues auftaucht oder das Internet ausfällt. Dazu ein Sicherheitscheck mit
konkreten Tipps (offenes Telnet/RDP, Geräte ohne Updates, zweiter DHCP-Server, ARP-Spoofing).

Mit FRITZ!Box (TR-064, optional): Gerätenamen aus der Box, WLAN-Diagnose (welches Gerät hängt an
welchem Repeater, in welchem Band, wie gut), Sync-Raten und ein PDF-Nachweis über Ausfälle und
Leitungswerte für den Provider.

Läuft als Docker-Container (ZimaOS, CasaOS, Unraid, TrueNAS, Raspberry Pi), ohne Cloud und Abo.
https://github.com/Pyzyi/netzwart
```

## 5. Danach

- **Erste Issues schnell beantworten.** Früh im Projekt entscheidet die Reaktionszeit, ob Leute wiederkommen.
- **Gerätewissen sammeln:** Die Vorlage „Device recognition“ liefert Daten für bessere Erkennung. Jede Verbesserung
  ist ein guter Anlass für ein kurzes Update in den Threads.
- **Wirkung messen:** Insights → Traffic auf GitHub zeigt, woher Besucher kommen.
