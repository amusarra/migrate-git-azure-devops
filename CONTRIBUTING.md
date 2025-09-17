# Contribuire al progetto migrazione-git-azure-devops

Grazie per il tuo interesse! Questo documento spiega come proporre modifiche, aprire issue e inviare pull request a questo progetto Go.

## Come posso aiutare?

- Segnalazione bug: apri una Issue descrivendo passi per riprodurre, output atteso/ottenuto e versione del tool (`--version`).
- Proposte di feature: apri una Issue con contesto, problema, soluzione proposta e impatto.
- Documentazione: correzioni e miglioramenti al README o ai commenti del codice sono benvenuti.
- Codice: invia PR piccole e mirate, con test (dove ha senso) e descrizione chiara.

## Requisiti

- Go 1.22+ (consigliato l’ultimo minor)
- Git
- Opzionale:
  - golangci-lint per il lint locale
  - Docker/Buildx per testare l’immagine
  - GoReleaser per build snapshot locali

## Setup ambiente

```bash
git clone https://github.com/amusarra/migrazione-git-azure-devops.git
cd migrazione-git-azure-devops
go mod tidy
```

Build locale del tool:

```bash
go build -o bin/migrazione-git-azure-devops ./cmd/migrazione-git-azure-devops
./bin/migrazione-git-azure-devops --version
```

Snapshot con GoReleaser (artefatti in dist/):

```bash
goreleaser release --clean --snapshot --skip=publish
```

## Flusso di lavoro per le PR

1. Forka il repository e crea un branch da `main`:
   - nome branch suggerito: tipo/scope-breve-esempio (es. `feat/wizard-prompt`, `fix/http-302`)
2. Sviluppo:
   - format: `go fmt ./...`
   - analisi: `go vet ./...`
   - lint (opzionale ma consigliato): `golangci-lint run`
   - test (se presenti): `go test ./...`
   - build: `go build ./cmd/migrazione-git-azure-devops`
3. Mantieni le modifiche piccole, con commenti godoc su funzioni e blocchi complessi.
4. Aggiorna README se cambi l’uso della CLI o aggiungi flag.
5. Apri la PR descrivendo:
   - problema risolto/feature
   - cambiamenti principali
   - note su compatibilità, impatti e come testare

## Stile dei commit (Conventional Commits)

Usa messaggi tipo:

- feat: aggiungi una nuova funzionalità
- fix: correggi un bug
- docs: modifica documentazione
- refactor: refactoring senza cambiamenti funzionali
- test: aggiungi/aggiorna test
- build/ci: cambi a build system o pipeline CI
- chore: attività di manutenzione

Esempi:

- `feat: aggiunto flag --version`
- `fix: gestito HTTP 302 senza follow dei redirect`

## Linee guida codice

- Mantieni funzioni piccole e coese; estrai helper in file dedicati (es. api.go, utils.go).
- Documenta funzioni e tipi con commenti godoc.
- Gestisci sempre gli errori (non ignorare i return di Close/Remove).
- Non stampare body delle risposte HTTP in chiaro, se non in `--trace`.
- Mantieni l’output CLI chiaro e stabile; evita breaking changes non necessari.

## Versione e Release

- Le variabili `version`, `commit`, `date` sono impostate via `-ldflags` in build/release.
- Non aggiornare manualmente la versione nel codice.
- I rilasci sono taggati (SemVer) e gestiti da GitHub Actions + GoReleaser.
- Il changelog è generato automaticamente.

## Sicurezza

- Non includere credenziali/PAT negli issue, PR o log.
- Per vulnerabilità, usa GitHub Security Advisories o contatta i maintainer in privato.
- Non aprire Issue pubbliche con PoC che espongono dati sensibili.

## Codice di condotta

Adottiamo un comportamento rispettoso e professionale. Come riferimento, il [Contributor Covenant](https://www.contributor-covenant.org/version/2/1/code_of_conduct/) è una buona base.

## Domande?

Apri una Issue con etichetta “question” o avvia una discussione. Grazie per il contributo!
