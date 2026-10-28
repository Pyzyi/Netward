# 05 · UX- und Design-Konzept

> Rolle: Product Designer · Status: **Redesign 3 „Klartext“** (0.2.1; Abschnitt 0 ersetzt die visuellen Festlegungen
> der Abschnitte 2–4; Prinzipien, Informationsarchitektur und Mikrotexte gelten weiter)

## 0. Redesign 3 · Designsprache „Klartext“

### Warum
„Leitstand“ (Redesign 2) hatte eine eigene Idee, wirkte aber wieder generiert. Nutzerfeedback zu 0.2:
„sieht immer noch nach Vibecode-KI-Slop aus“, nur die Übersicht mit dem Router in der Mitte überzeugte.
Die Ursache waren Stilmittel, die heute fast jede generierte Oberfläche trägt:
- Rubriken in Monospace-Versalien mit Sperrung („NEU“, „IP-ADRESSE“, „KRITISCH“)
- eine 120-px-Kennzahl in Signalfarbe
- Hinweise mit farbigem Randstrich links
- harte schwarze Trennlinien
- quadratische Statusmarken
- warmes „Papier“ mit Tinte

### Leitidee
Netward soll wie ein gut gemachtes Werkzeug wirken, nicht wie eine Inszenierung. Ruhige weiße Flächen
auf neutralem Grund, normale Schreibweise, klare Hierarchie über Größe und Gewicht. Gestaltung nur dort,
wo sie Information trägt: der Netzring, der Herzschlag-Streifen, Statusfarben.

### Was bleibt
1. **Netzring** auf dunkler Bühne: Router in der Mitte, Geräte als Punkte nach Kategorie. Jetzt größer
   (bis 420 px), mit Legende (online, offline, neu) und Kategorienamen in normaler Schrift. Auf dem Handy
   wird er ohne Beschriftung gezeigt, statt ganz zu fehlen.
2. **Herzschlag-Streifen** der letzten 24 Stunden in der Geräteliste.
3. **Dunkles Band** mit Netzname, Navigation und Scan-Status.

### Regeln
| Thema | Regel |
|---|---|
| Fläche | Grund `#F3F4F6`, Inhalte in weißen Panels (Radius 10 px, 1-px-Rahmen, kein Schatten). Listen stehen *in* Panels, Zeilen trennt eine Haarlinie. Keine Karten in Karten, keine Kennzahl-Kachelraster. |
| Farbe | Primäraktion neutral dunkel (hell im Dunkelmodus). Blau `#2856C7` nur für Auswahl, Fokus und Schalter. Status: Grün (online), Bernstein (neu/Warnung), Rot (kritisch), Grau (offline). Keine Verläufe. |
| Schrift | **IBM Plex Sans** für alles, **IBM Plex Mono** nur für IP, MAC, Ports und Versionen. Normale Schreibweise, keine Versalien-Rubriken. Uhrzeiten mit tabellarischen Ziffern in der Grundschrift. |
| Hierarchie | Seitentitel 24 px/600, Abschnittstitel 15 px/600, Grundtext 14 px, Beschriftungen 12–13 px/500 in Grau. Höchstes Gewicht 600. |
| Status | Kleine getönte Chips in normaler Schreibweise („Neu“, „Update“, „Kritisch“) statt Versalien mit Quadrat. Punkte vor Wörtern, wo der Platz knapp ist. |
| Hinweise | Getönte Fläche mit Symbol und feinem Rahmen in Statusfarbe, kein Randstrich. |
| Bedienelemente | Segment-Schalter für Filter und Zeiträume, runde Schalter, Radius 6 px für Felder und Knöpfe. Fokus als blauer Ring. |
| Bewegung | Nur Bedeutung: Scan-Puls im Band, Halo neuer Geräte, Einblenden des Panels. `prefers-reduced-motion` schaltet alles ab. |
| Dunkelmodus | Neutrales Fast-Schwarz statt Nachtblau für Flächen, das Band bleibt dunkler als der Grund. |

### Seiten
- **Übersicht:** dunkle Bühne mit der Antwort als Satz („26 von 30 Geräten online“, die Zahl in Grün), darunter
  Neu, Offline und Internet mit Latenzkurve; rechts der Netzring. Darunter zwei Spalten mit Panels:
  Braucht Aufmerksamkeit, Software und Updates, Gerätetypen; Wer ist zu Hause, Letzte Aktivität, Sicherheit.
- **Geräte:** Suche, Gruppierung und Sortierung in einer Zeile, Filter als Segment-Schalter mit Zählern,
  die Liste als ein Panel mit Spaltenköpfen. Das Gerätedetail ist ein Panel rechts (Desktop) bzw. Vollbild (Handy).
- **Aktivität:** ein Panel je Tag, Uhrzeit links, Symbol, Satz.
- **Sicherheit:** Zählchips (Kritisch, Warnung, Hinweis), Befunde in einem Panel mit Schweregrad-Chip,
  aufklappbar „Warum / Was tun“.
- **Personen:** ein Panel je Person, auf breiten Bildschirmen zweispaltig.
- **Einstellungen:** Navigation links als Liste mit hinterlegtem aktivem Eintrag, jeder Abschnitt ein Panel
  mit Fußzeile für Speichern.
- **Anmeldung/Einrichtung:** links das dunkle Band mit dem Ring, rechts das Formular.

## 1. Gestaltungsprinzipien

1. **Antwort zuerst.** Jede Seite beantwortet zuerst die Frage, mit der man sie öffnet
   („Ist alles in Ordnung?“, „Wer ist das?“). Details folgen auf Nachfrage.
2. **Ruhig statt laut.** Eine Akzentfarbe, neutrale Flächen. Farbe nur für Status und Aktion.
   Keine Verläufe, kein Glas-Effekt, keine Emojis, keine Deko-Illustrationen.
3. **Dichte wie ein Werkzeug.** Listen sind kompakt und scannbar wie in Fing:
   Icon, Name, Hersteller/IP, Status. Technische Werte (IP, MAC) stehen in Monospace mit tabellarischen Ziffern.
4. **Sprache wie ein Mensch.** „Neues Gerät im Netz“ statt „Event: DEVICE_NEW“.
   Relative Zeiten („vor 3 Min.“), Tooltip mit exaktem Zeitpunkt.
5. **Sicher ohne Angst.** Sicherheitsbefunde nüchtern formuliert, immer mit konkretem nächsten Schritt.

## 2. Design-Tokens

Die verbindlichen Tokens stehen in [`web/src/app.css`](../web/src/app.css) und folgen Abschnitt 0
(neutraler Grund, weiße Panels, Grün, Bernstein, Rot, Blau nur für Auswahl; IBM Plex Sans und IBM Plex Mono;
Radien 5/6/10 px). Typografie-Skala: 12, 13, 14 (Basis), 15, 16, 20/22, 24, 30 px; Gewichte 400/500/600. Alle Text- und Steuerelement-Kontraste prüft `npm run contrast` in CI
(hell und dunkel, ≥ 4,5:1 für Text, ≥ 3:1 für Steuerelemente).

## 3. Informationsarchitektur

```
Übersicht ─ Geräte ─ Aktivität ─ Sicherheit ─ Einstellungen
                │
                └─ Gerätedetail (Seitenpanel Desktop / Vollbild Mobil)
```

- **Desktop (> 860 px):** Tintenkopf mit Logo, Netzname, Textreitern, Scan-Puls und Abmelden; die Navigation
  steht auf jeder Seite an derselben Stelle. Das Gerätedetail öffnet als Blatt rechts (470 px), die Liste bleibt sichtbar;
  unter 1100 px als modales Blatt.
- **Mobil (≤ 860 px):** Untere Tab-Leiste, Detail als Vollbild. Auf der Übersicht steht die Antwort zuerst
  (Kennzahlen, „Braucht Aufmerksamkeit“); der Netzring erscheint erst, wenn genug Platz für ihn ist.

## 4. Screens

### Ersteinrichtung (2 Schritte)
1. „Willkommen bei Netward“: Benutzername + Passwort (≥ 10 Zeichen, Stärkeanzeige über Länge/Zeichenklassen/Sperrliste, ohne große Bibliothek).
2. „Dein Netzwerk“: erkannte Interfaces/Subnetze vorausgewählt, Sprache. **Scan starten**.
Danach füllt sich die Übersicht live, ohne leeren Zwischenzustand.

### Übersicht
- **Kopfbereich:** Netzname (editierbar, z. B. „Zuhause“), Subnetz · Gateway, Button „Jetzt scannen“ mit Fortschritt.
- **Kennzahlen (4 Kacheln):** Online · Offline · Neu · Internet (Latenz, Status).
- **„Braucht Aufmerksamkeit“:** neue unbekannte Geräte mit Direktaktionen *Bekannt* / *Details*.
  Leerer Zustand: „Alles bekannt. Neue Geräte erscheinen hier.“
- **Letzte Aktivität:** die letzten 8 Ereignisse, Link zu Aktivität.
- **Nach Typ:** kompakte Verteilung (Telefone 6, Computer 4, …) als Filter-Einstieg.

### Geräte
- Suchfeld (`/`), Filter-Chips: Alle · Online · Offline · Neu · Favoriten; Gruppierung: keine / Typ / Ort.
- Zeile: Icon+Status · **Name** · Hersteller/Modell · IP (mono) · zuletzt gesehen/„online“ · Favoritenstern.
- Sortierung: Status, Name, IP (numerisch), zuletzt gesehen.

### Gerätedetail
- Kopf: großes Icon, Name (inline editierbar), Status-Pill, Typ-Auswahl.
- Aktionen: *Bekannt markieren*, *Beobachten* (Benachrichtigung bei online/offline), *Favorit*,
  *Aufwecken (WoL)*, *Ping*, *Ports scannen*, Menü: *Vergessen*.
- Abschnitte: **Anwesenheit** (Balken-Zeitleiste 24 h / 7 d / 30 d + Online-Anteil), **Netzwerk**
  (IP, MAC kopierbar, Hersteller, Interface, IP-Historie), **Erkannt als** (alle Namen mit Quelle,
  Dienste aus mDNS/SSDP, DHCP-Fingerprint/OS), **Offene Ports**, **Notizen & Ort**.

### Aktivität
Chronologische Liste nach Tagen gruppiert, Filter nach Art. Jede Zeile: Icon, Satz („**iPhone von Anna** ist wieder online“), Zeit.

### Sicherheit
Ergebnisliste nach Schweregrad (Kritisch / Warnung / Hinweis) mit Erklärung „Warum ist das wichtig?“
und „Was tun?“. Kopf: letzter Check, Button „Jetzt prüfen“. Kein Gamification-Score.

### Einstellungen
Abschnitte: Allgemein · Scannen · Benachrichtigungen · Home Assistant · Sicherheit & Konto · Daten · Über.

## 5. Mikrotexte (Auswahl)

| Situation | Text |
|---|---|
| Neues Gerät | „Neues Gerät im Netz: **{name}** ({vendor}, {ip})“ |
| Gerät offline | „**{name}** ist offline“ |
| Leere Geräteliste nach Filter | „Keine Geräte für „{query}“. Filter zurücksetzen?“ |
| Fallback-Modus | „Eingeschränkter Scanmodus: Dem Container fehlt die Berechtigung `NET_RAW`. Geräte werden langsamer erkannt.“ |
| HTTP-Hinweis | „Verbindung unverschlüsselt. Im Heimnetz meist in Ordnung, für Zugriff von außen bitte HTTPS verwenden.“ |

## 6. Anti-Muster (verbindlich vermeiden)

- Hero-Banner, Marketing-Sprache, „Willkommen zurück 👋“
- Lila-Blau-Verläufe, Glassmorphism, Neon-Glow, übermäßige Schatten
- Karten in Karten, Icons ohne Funktion, Platzhaltertexte
- Spinner für alles: stattdessen Skeletons bei Listen, Inline-Fortschritt bei Aktionen
- Modale Dialoge für Dinge, die inline gehen
