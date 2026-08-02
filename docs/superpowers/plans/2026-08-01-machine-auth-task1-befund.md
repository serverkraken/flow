# Task-1-Befund: Authentik-Token-Form (2026-08-02)

Ausgeführt nach dem Merge von `serverkraken/homelab-study#1050`, das den
Provider `flow-machine` und den Service-Account `wartung-agent` angelegt hat.
Der Aufruf ist der `curl` aus Spec §9.1, gegen die Live-Instanz.

## Ergebnis: die Annahme trägt

| Frage | Antwort |
|---|---|
| Grant funktioniert? | **ja** — HTTP 200, `token_type: Bearer`, `expires_in: 3600`, `scope: openid` |
| JWT statt opak? | **ja** — drei Segmente, Header `{"alg":"RS256","typ":"JWT"}` |
| `iss` | `https://id.thebackend.org/application/o/flow-machine/` |
| `aud` | `flow-machine` — **ein String, kein Array** |
| `sub` | `wartung-agent` |
| `azp` | `flow-machine` |
| `exp - iat` | 3600 s |

## Daraus folgt für die Konfiguration

Die in `homelab-study` bereits eingetragenen Werte stimmen ohne Änderung:

```sh
FLOW_OIDC_MACHINE_ISSUER=https://id.thebackend.org/application/o/flow-machine/
FLOW_OIDC_MACHINE_CLIENT_ID=flow-machine
FLOW_MACHINE_ACCOUNTS=wartung-agent=msoent:wartung-agent
```

- Der Issuer ist der **Application-Slug**, nicht der Provider-Anzeigename — die
  Annahme aus Blueprint 53 und Spec §9 ist bestätigt.
- `sub` ist der Username, weil der Provider `sub_mode: user_username` fährt.
  Der Besitzer-Sub `msoent` steht so auch in `FLOW_ALLOWED_SUBS`.

## Zwei Beobachtungen, die keine Blocker sind

**`aud` ist ein String, kein Array.** Damit ist der im Whole-Branch-Review
gefundene Reihenfolge-Fehler in `Verify` in dieser Topologie ohnehin nicht
erreichbar — es gibt nur eine Audience. Der machine-preferring Fix bleibt
trotzdem richtig: er deckt die Dev-Topologie mit einem geteilten Dex-Issuer ab
und einen künftigen Authentik-Wechsel auf globalen Issuer-Modus. Der Fix war
Vorsorge, nicht Reparatur eines akuten Lecks.

**`preferred_username` und `groups` sind `null`.** Für den Maschinen-Pfad
folgenlos: `EnsureUser` läuft dort nicht, die Identität wird nicht aus Claims
aufgebaut, und die Allowlist ist die serverseitige Aufzählung in
`FLOW_MACHINE_ACCOUNTS`. Es bestätigt nebenbei die Entscheidung aus Spec §5.3,
den Gruppenzwang nicht als zweite Bedingung einzubauen — er hätte hier
zuverlässig fehlgeschlagen.

## Was damit noch offen bleibt

Dieser Befund belegt die **Token-Form**, nicht den Ende-zu-Ende-Pfad. Der
Live-Smoke aus Task 8 (201 auf freigegebener Route, 403 auf `DELETE`, 403 auf
`/nodes`, 401 bei manipuliertem Token, Provenance in der Web-UI) braucht
zusätzlich das ausgerollte flow-server-Image aus `serverkraken/flow#71`.
