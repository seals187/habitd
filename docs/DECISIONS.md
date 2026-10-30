# Entscheidungen

Wo Spezifikation und Vorlagen nicht eindeutig waren oder sich widersprochen haben.
Grundregel: Eine explizite Regel im Text gewinnt, ein „z. B.“ weicht den Bildern.

## Spielregeln

- **Levelkurve:** `100 * level^1.5` ist nur ein Beispiel und passt nicht zu den Bildern
  (8.940 XP wären Level 9). Die Kosten eines Levels sind `21,57 * level^1,822`, auf 10
  gerundet. Das ergibt genau Level 12 mit 1.440 / 2.000 XP. Die Kurve bleibt progressiv.
- **Taler pro Check-in:** Das Bild zeigt +5 Taler bei +10 XP, der Text sagt „ein Drittel“.
  Es gilt der Text: +3 Taler. Die Vorschau im Formular zeigt den echten Wert.
- **XP in den Bildern:** „Neue Idee einbringen“ zeigt +20 XP, das gibt es laut Regeln
  nicht (10/15/25). In den Demo-Daten ist das Habit mittel (+15).
- **XP-Anzeige:** Die Liste auf „Heute“ zeigt wie im Bild die Basis-XP ohne Serienbonus,
  „20 Min. lesen“ also +15. Der aktive Bonus steht im Habit-Detail.
- **Serien in Wochen:** Habits mit „n× pro Woche“ und eigenen Wochentagen zählen ihre Serie
  in Wochen, wie in den Bildern. Eine Woche zählt, wenn ihr Ziel erreicht ist. Die laufende
  Woche bricht die Serie nicht.
  - Serienbonus und Joker rechnen mit Wochen × 7 Tagen.
  - Serien-Abzeichen und „Längste Serie“ rechnen mit den Tagen mit Check-in
    (Wochen × Ziel pro Woche), sonst wäre die Monatsflamme in den Bildern schon erreicht.
    Abzeichen ab etwa „Ein Jahr“ sind damit praktisch tägliche Habits.
  - Bereichs-Abzeichen mit Zeitangabe („4 Wochen Serie“) rechnen mit der echten Dauer.
- **Vermeiden:** Ein Tag zählt und wird bezahlt, sobald er vorbei ist. Am ersten Tag steht
  die Serie auf 0. Ein nachgetragener Rückfall nimmt keine XP weg.
- **Serien-Abzeichen** zählen nur Aufbauen-Habits, Vermeiden hat eigene Abzeichen.
- **Joker:** Ein überbrückter Tag erhält die Serie, erhöht sie aber nicht und bringt keine
  XP. Wird der Tag später echt nachgetragen, kommt der Joker zurück. Bei Wochen-Habits
  prüft der automatische Einsatz am Sonntag, und nur wenn genau ein Check-in fehlt.
- **Joker-Grenze:** Die Grenze von 3 gilt für alle Quellen. Beim Level-Up mit 3 Jokern
  entfällt die Zeile „+1“ auf dem Feier-Screen, im Shop steht „max. 3“.
- **Level und Rückgängig:** Der erreichte Level wird gemerkt. Rückgängig machen senkt die
  XP, nie den Level, und Level-Boni, Abzeichen und Serien-Joker gibt es nur einmal.
  Abgehakt und wieder rückgängig gemacht lässt sich also nichts farmen.
- **Tagesquest:** „Alle n Habits“ zählt heute fällige Aufbauen-Habits plus Vermeiden-Habits
  ohne Rückfall. Wochen-Habits zählen, solange das Wochenziel offen ist. Wird das letzte
  Häkchen zurückgenommen, wird die Quest wieder abgezogen.
- **Perfekte Woche / Monat** brauchen mindestens zwei Habits, sonst wären sie mit einem
  einzigen Habit dasselbe wie die Serie.
- **Abzeichen mit „ד** zählen laut Text alle Check-ins des Bereichs. Angerufen und
  Nicht vergessen hängen daher am selben Zähler (Bereich Familie), nur Sport, Wasser
  und Lesen werden über das Tag am Icon unterschieden.
- **Jahreszeiten:** Eine Saison zählt ab 80 % bei mindestens 60 geplanten Check-ins, damit
  wenige Tage nicht reichen. Gemessen werden Habits mit festen Tagen.
- **Feiertage:** Bundesweite Feiertage DE sind eingebaut, weitere Tage und das Abschalten
  gehen unter Profil → Server.

- **Monatlich:** Die Vorlage „Blut spenden“ brauchte einen neuen Rhythmus. Die Serie zählt in
  Monaten, Bonus und Joker rechnen mit 30 Tagen je Monat. Joker gelten hier nicht.
- **Neue Abzeichen ohne Check-ins:** Digital, Geld und Umwelt enthalten fast nur
  Vermeiden-Vorlagen. Deshalb zählen „×“ dort saubere Tage statt Check-ins (Offline-Stunde,
  Mehrweg). Velo statt Auto zählt die Tags `auto` und `velo` zusammen.
- **Detox-Tag:** Ein ganzer Tag offline lässt sich nicht messen. Das Abzeichen gilt, wenn
  3 Digital-Vermeiden-Habits zugleich mindestens einen sauberen Tag in Serie haben.
- **`templates.json` korrigiert:** „Sportwetten und Glücksspiel“ trug das Tag `sport` und hätte
  Sport-Abzeichen ausgelöst, das Tag ist entfernt. „Essen bestellen“ hat das neue Tag
  `lieferdienst` für Selbstgemacht.

## Oberfläche

- **Jahr-Heatmap:** 53 Wochen, horizontal scrollbar, beim Öffnen rechts am aktuellen Rand.
  Pro Tag gibt es bei einem einzelnen Habit nur erledigt oder nicht, die Farbstufen zeigen
  daher, wie weit der Tag in einer Serie liegt (1–2, 3–9, ab 10 Tagen).
- **Serien im Zeitverlauf:** Die Balken zeigen die Lage auf der Zeitachse des Habits, wie im
  Bild. „Davor“ ist die längste Serie vor dem Rekord.
- **Monatsstatistik:** Wie im Bild (28/29) zählt der laufende Tag nicht mit.
- **Neues Habit:** Zuerst nur Name, Vorschläge, „Beliebt“ und der Link zur Bibliothek, wie im
  Bild. Art, Häufigkeit und die übrigen Felder erscheinen, sobald eine Vorlage oder „… als
  eigenes Habit anlegen“ gewählt ist. Eine Vorlage übernimmt ihre Werte per Seitenaufruf
  (`/habits/new?template=…`), so geht es ohne JavaScript-Logik.
- **Vorschläge:** Beliebte zuerst, dann Namen mit passendem Anfang, dann der Rest.
- **Onboarding Schritt 2:** Pro Bereich bis zu 3 beliebte Vorlagen. Hat ein Bereich weniger
  als 2, wird aufgefüllt, ohne dasselbe Tag doppelt zu zeigen. Die Häufigkeit folgt
  `templates.json`, „30 Min. Sport“ ist dort täglich, im Bild 3× pro Woche.
- **Onboarding:** Startet, solange es keine Habits gibt und es nicht abgeschlossen wurde.
  „Überspringen“ steht unter dem Button und fehlt im Bild.
- **Profil:** Zusätzlich „Onboarding erneut starten“, das Bild zeigt nur die anderen Zeilen.
- **Abzeichen, Übersicht:** Pro Kategorie zwei Reihen wie im Bild, der Rest über „›“ oder
  die Filter. Die Kategorieansicht zeigt zusätzlich den Hinweistext. Geheime Abzeichen
  zeigen „???“ ohne Hinweis, auch wenn die Übersichtsgrafik ihn nennt.
- **Zusätzliche Bedienelemente:** Stift im Habit-Detail, Link „Neues Habit“ auf Heute, Tab
  „Verlauf“ bei Gönn dir, 18 statt 6 Icons, Wochentag-Auswahl bei „Eigene“. Die Kennzahlen
  und die Ziel-Belohnung stehen im Habit-Detail unter dem jeweiligen Bild-Inhalt.
- **Rückfall melden:** Tippen auf den Kreis bei „Kein Alkohol“, mit Rückfrage.
- **Nachtragen:** Tage der letzten 7 Tage sind in Woche und Monat antippbar.

## Technik

- **Benutzer:** `HABITD_USER` gilt bei jedem Start, `HABITD_PASSWORD` nur beim ersten.
- **Zwei-Stufen-CSRF:** Jeder POST braucht das Session-Token, zusätzlich prüft Go
  `http.CrossOriginProtection` Origin und `Sec-Fetch-Site`, auch für den Login.
- **`Referrer-Policy: same-origin`** statt `no-referrer`: Bei `no-referrer` senden Browser
  `Origin: null`, und der Login scheitert über HTTP von anderen Geräten im Netz.
- **Alpine CSP-Build,** weil das normale Alpine `unsafe-eval` bräuchte. htmx läuft mit
  `allowEval: false`. Die CSP erlaubt nur `style`-Attribute inline für berechnete Breiten
  und Farben, keine `<style>`-Blöcke und keine Skripte.
- **Web-Push** ist mit der Standardbibliothek umgesetzt (VAPID, aes128gcm, getestet gegen den
  Testvektor aus RFC 8291). Die Nachricht läuft zwangsläufig über den Push-Dienst des Browsers.
- **Mehr Tabellen als vorgesehen:** `sessions`, `celebrations` und `push_subscriptions`. `areas`
  hat zusätzlich `key`, damit Abzeichen ein Umbenennen überleben, `habits` hat
  `weekly_target` (Check-ins pro Woche oder Monat).
- **Version:** Das erste Release heißt `0.0.1`, die Bilder zeigen beispielhaft „v1.0“.
- **Migrationen** laufen mit deaktivierten Foreign Keys auf einer eigenen Verbindung, weil
  SQLite für eine neue CHECK-Regel die Tabelle neu aufbauen muss. Danach prüft
  `foreign_key_check` die Daten.
- **`internal/service`** ergänzt die vorgeschlagene Struktur: Dort laufen die Transaktionen,
  `internal/game` bleibt pur und ohne Datenbank.
- **Demo-Daten:** Die Salden stimmen exakt mit den Bildern überein. Der Seed zeigt mehr
  freigeschaltete Abzeichen (24 statt 18), weil die Regeln auf den Bild-Daten mehr
  auslösen, etwa Jongleur bei 5 Habits. Die Daten sind nicht geschönt, sonst käme beim ersten
  Abhaken eine Serie von Feiern. Lesen existiert 246 Tage, damit 212 Tage 86 % ergeben, obwohl
  das Profil „dabei seit 212 Tagen“ zeigt: So widersprechen sich die Bilder selbst.
