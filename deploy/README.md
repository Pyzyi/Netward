# Netzwart in App-Stores eintragen

Hier liegt alles, was für die Einträge in den App-Stores nötig ist. Einreichen kann nur der Inhaber des
GitHub-Kontos (Pull Requests in fremden Repositories, Formulare, Forenbeiträge). Die Schritte unten sind so
geschrieben, dass sie vom Handy aus im Browser funktionieren, bis auf die Punkte mit „Rechner“.

| Store | Dateien | Wo einreichen | Aufwand |
|---|---|---|---|
| CasaOS / ZimaOS | [`casaos/Netzwart/`](casaos/Netzwart) | Pull Request an [IceWhaleTech/CasaOS-AppStore](https://github.com/IceWhaleTech/CasaOS-AppStore) | gering |
| Unraid (Community Applications) | [`unraid/`](unraid) | eigenes Repo `Pyzyi/unraid-templates` + Formular auf [ca.unraid.net/submit](https://ca.unraid.net/submit) | gering |
| TrueNAS SCALE (Community-Katalog) | [`truenas/ix-dev/community/netzwart/`](truenas/ix-dev/community/netzwart) | Pull Request an [truenas/apps](https://github.com/truenas/apps) | mittel, braucht einen Rechner mit Docker |
| TrueNAS ohne Katalog | [`truenas/netzwart.yaml`](truenas/netzwart.yaml) | sofort nutzbar über „Install via YAML“ | – |

Bei jedem neuen Release die Versionsnummer in den Store-Dateien erhöhen (CasaOS `image` und `version`,
TrueNAS `app_version`, `ix_values.yaml` `tag` und `version`). Unraid nutzt `latest` und braucht das nicht.

## 1. CasaOS / ZimaOS App Store

Der ZimaOS-App-Store ist derselbe wie der von CasaOS (`IceWhaleTech/CasaOS-AppStore`).

1. [IceWhaleTech/CasaOS-AppStore](https://github.com/IceWhaleTech/CasaOS-AppStore) öffnen → **Fork**.
2. Im Fork den Ordner `Apps/Netzwart/` anlegen und alle Dateien aus [`casaos/Netzwart/`](casaos/Netzwart)
   hochladen (Add file → Upload files): `docker-compose.yml`, `icon.svg`, `thumbnail.png`, `screenshot-1.png` bis
   `screenshot-4.png`.
3. In `Apps/Netzwart/docker-compose.yml` die Bild-Adressen auf den Store umstellen, also
   `https://cdn.jsdelivr.net/gh/Pyzyi/Netward@main/deploy/casaos/Netzwart/` durch
   `https://cdn.jsdelivr.net/gh/IceWhaleTech/CasaOS-AppStore@main/Apps/Netzwart/` ersetzen (so machen es alle
   anderen Apps dort).
4. **Contribute → Open pull request**. Titel: `Add Netzwart`. Text: siehe [PR-Texte](#pr-texte).
5. Die CI des Stores prüft die Datei automatisch. Falls sie etwas bemängelt, steht es im PR.

## 2. Unraid Community Applications

Community Applications liest Vorlagen aus einem eigenen GitHub-Repository mit `ca_profile.xml` im Hauptordner.

1. Auf GitHub ein neues, öffentliches Repository `unraid-templates` anlegen, Lizenz **MIT** auswählen.
2. `unraid/netzwart.xml` und `unraid/ca_profile.xml` dort in den Hauptordner hochladen.
3. In `netzwart.xml` die `TemplateURL` auf das neue Repo ändern:
   `https://raw.githubusercontent.com/Pyzyi/unraid-templates/main/netzwart.xml`
4. Optional, aber gern gesehen: im [Unraid-Forum](https://forums.unraid.net/forum/47-docker-containers/) einen
   Support-Thread eröffnen (Text unten) und dessen Link in `<Support>` eintragen.
5. [ca.unraid.net/submit](https://ca.unraid.net/submit) öffnen, die Repo-Adresse eingeben, **Validate** und
   **Scan** ausführen und einreichen.

## 3. TrueNAS SCALE Community-Katalog

Die Dateien folgen dem Aufbau von `truenas/apps` (`ix-dev/community/<app>/`). Zwei Dinge erzeugt deren Werkzeug
selbst und müssen auf einem Rechner mit Docker laufen:

```bash
git clone https://github.com/<dein-konto>/apps.git && cd apps     # vorher truenas/apps forken
cp -r <netzwart>/deploy/truenas/ix-dev/community/netzwart ix-dev/community/
./.github/scripts/port_validation.py                              # prüft den Port 8840 gegen andere Apps
devbox run copy-lib                                               # erzeugt templates/library und lib_version_hash
./.github/scripts/ci.py --app netzwart --train community --test-file basic-values.yaml --wait=true
```

Wenn der Test grün ist: committen, PR mit der Vorlage `app_addition.md` öffnen. Icon und Screenshots stehen in
`app.yaml` als Links, das TrueNAS-Team lädt sie auf sein CDN. Vorher lohnt ein kurzes Issue, ob eine App mit
Host-Netzwerk in den Katalog passt.

Bis dahin funktioniert [`truenas/netzwart.yaml`](truenas/netzwart.yaml) über **Apps → Discover Apps → ⋮ →
Install via YAML**.

## PR-Texte

### CasaOS-AppStore

```markdown
## New app: Netzwart

Netzwart is a self-hosted home network monitor with security checks
(https://github.com/Pyzyi/Netward, MIT).

- Discovers every device on the LAN (ARP, DHCP, mDNS, UPnP) with readable names and icons
- Presence, notifications (ntfy, Gotify, Telegram, …), Home Assistant via MQTT
- Security check: risky open ports, outdated systems, rogue DHCP server, ARP spoofing
- amd64 + arm64, signed image on ghcr.io, about 23 MB

Why `network_mode: host`: layer-2 discovery only works on the host network. The container is
read-only and drops every capability after start-up (CHOWN/SETUID/SETGID/NET_RAW are only used
to open the sockets and switch to an unprivileged user).

Tested by importing the compose file as a custom app on ZimaOS.
```

### truenas/apps

```markdown
## New community app: Netzwart

Self-hosted home network monitor with security checks — https://github.com/Pyzyi/Netward (MIT).

- Uses the host network (layer-2 discovery: ARP, DHCP, mDNS). Starts as root, opens its raw
  sockets, switches to the configured user (default 568) and drops all capabilities.
- Read-only root filesystem, built-in healthcheck (`/netzwart healthcheck`).
- Icon: https://raw.githubusercontent.com/Pyzyi/Netward/main/web/public/icon-512.png
- Screenshots: see app.yaml
```

### Unraid-Forum (Support-Thread)

```text
[Support] Netzwart – home network monitor with security checks

Netzwart finds every device on your network, gives it a readable name, shows who is online and alerts
you about new devices and outages. It also checks for risky open ports, outdated systems, a rogue DHCP
server and ARP spoofing.

GitHub: https://github.com/Pyzyi/Netward
Image: ghcr.io/pyzyi/netward (amd64, arm64)

Notes for Unraid:
- Host network is required for layer-2 discovery.
- Open the WebUI within 30 minutes of the first start to create your account.
- If you run several bridges or VLANs, set "Network interfaces" (e.g. br0,br0.10).

Please report bugs on GitHub; questions are welcome here.
```

## Note on the data folder owner (PUID/PGID)

The Unraid template uses 99:100 (`nobody:users`) and the TrueNAS app 568 (`apps`), because that is what users of
those systems expect. Many other containers run with the same IDs. Any of them that mounts the same appdata path could
read Netzwart's database (password hash, notification tokens, FRITZ!Box and MQTT credentials). The data folder is
created with mode 0700 and nothing else should mount it; if you want a clear separation, give Netzwart its own UID/GID
(e.g. 10001, the image default) and `chown` the folder to it.
