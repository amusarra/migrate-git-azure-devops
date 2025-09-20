# Tool di migrazione repository Git Azure DevOps tra progetti/organizzazioni

![Go Build](https://github.com/amusarra/migrazione-git-azure-devops/actions/workflows/build.yml/badge.svg)
![Go Release](https://github.com/amusarra/migrazione-git-azure-devops/actions/workflows/release.yml/badge.svg)

CLI in Go per migrare repository Git tra progetti/organizzazioni Azure DevOps:

- modalità interattiva (wizard) o non interattiva (flag)
- mirror completo (branch/tag, con rimozione di ref eliminate)
- filtri (regex) e file lista
- dry-run e trace

Requisiti credenziali:

- SRC_PAT: Personal access token con scope "Code Read"
- DST_PAT: Personal access token con scope "Code Read, Write & Manage" (richiesto per migrazione)

> Nota: per generare PAT con i permessi necessari, vedere la [documentazione Microsoft](https://learn.microsoft.com/en-us/azure/devops/organizations/accounts/use-personal-access-tokens-to-authenticate)

A seguire un esempio del tool in azione.

[![asciicast](https://asciinema.org/a/741276.svg)](https://asciinema.org/a/741276?t=0:12)

## Quickstart

- Il primo passo è creare due PAT (Personal Access Token) con i permessi necessari e esportarli come variabili d'ambiente:

  ```bash
  export SRC_PAT="<PAT_Sorgente_Code_Read>"
  export DST_PAT="<PAT_Destinazione_Code_RW_Manage>"
  ```

- Come ottenere la lista dei repository nella sorgente:

  ```bash
  migrazione-git-azure-devops --src-org <org-src> --src-project <proj-src> --list-repos

  # abbreviazioni:
  # migrazione-git-azure-devops -so <org-src> -sp <proj-src> --list-repos
  ```

- Come avviare la migrazione usando il wizard interattivo (consigliato per prima migrazione)

  ```bash
  migrazione-git-azure-devops -so <org-src> -sp <proj-src> -do <org-dst> -dp <proj-dst> --wizard
  ```

- Come avviare la migrazione usando la modalità non interattiva (regex)

  ```bash
  migrazione-git-azure-devops -so <org-src> -sp <proj-src> \
    -do <org-dst> -dp <proj-dst> \
    -f '^horse-.*$'
  ```

- Come avviare il Dry-run (simulazione, nessuna modifica)

  ```bash
  migrazione-git-azure-devops -so <org-src> -sp <proj-src> -do <org-dst> -dp <proj-dst> \
    -f '^horse-.*$' --dry-run
  ```

- Come forzare il push su repo già esistenti in destinazione

  ```bash
  migrazione-git-azure-devops -so <org-src> -sp <proj-src> -do <org-dst> -dp <proj-dst> \
    -f '^horse-.*$' --force-push
  ```

## Uso della CLI

Flag principali:

- `--src-org`, `-so`: organizzazione sorgente
- `--src-project`, `-sp`: progetto sorgente
- `--dst-org`, `-do`: organizzazione destinazione
- `--dst-project`, `-dp`: progetto destinazione
- `--filter`, `-f`: regex dei repository da migrare (es: '^horse-.*$')
- `--repo-list`, `-rl`: file con lista nomi repo (uno per riga, "#" per commenti)
- `--dry-run`: non esegue modifiche, mostra solo le azioni
- `--force-push`, `-fp`: forza push mirror su repo già esistenti
- `--trace`, `-t`: output di debug; mostra anche body delle risposte HTTP in errore
- `--list-repos`: elenca i repository della sorgente e termina
- `--wizard`: modalità interattiva
- `-h`, `--help`: help

Esempi:

- Lista repo:

  ```bash
  migrazione-git-azure-devops -so myorg -sp MyProject --list-repos
  ```

- Migrazione con regex:
  
  ```bash
  migrazione-git-azure-devops -so srcorg -sp Src -do dstorg -dp Dst -f '^horse-(core|svc)-.*$'
  ```

- Migrazione da file lista:

  ```plaintext
  # File con la lista dei repository da migrare (uno per riga, "#" per commenti)
  horse-core
  horse-svc
  horse-cli
  ```

  ```bash
  migrazione-git-azure-devops -so srcorg -sp Src -do dstorg -dp Dst --repo-list repo.txt
  ```

Output e report:

- Al termine viene stampata una tabella di riepilogo della migrazione: Repository, Esito, Azure URL.
- In caso di errori API:
  - viene mostrato "[ERRORE API] HTTP {{codice}}"
  - in modalità `--trace` viene mostrato anche il body della risposta
- I redirect HTTP (3xx) non vengono seguiti: se il PAT è errato potresti vedere 302 invece di una 200 con pagina HTML.

## Installazione

Sono disponibili diverse opzioni per installare il tool.

> Accertarsi di avere Go 1.22+ installato e anche GOPATH/bin nel PATH oltre a git per la build locale.

Opzione A) Da sorgente (Go 1.22+)

```bash
# Installazione tramite 'go install'
# Il binario sarà $GOPATH/bin/migrazione-git-azure-devops
go install github.com/amusarra/migrazione-git-azure-devops/cmd/migrazione-git-azure-devops@latest
```

Opzione B) Build locale

```bash
git clone https://github.com/amusarra/migrazione-git-azure-devops.git
cd migrazione-git-azure-devops
go build -o bin/migrazione-git-azure-devops ./cmd/migrazione-git-azure-devops
```

Opzione C) Da release (binari precompilati)

- Vai alla pagina Release: <https://github.com/amusarra/migrazione-git-azure-devops/releases>
- Scarica il pacchetto per la tua piattaforma (tar.gz o .zip)
  - Linux AMD64: migrazione-git-azure-devops_x.y.z_linux_amd64.tar.gz
  - Linux ARM64: migrazione-git-azure-devops_x.y.z_linux_arm64.tar.gz
  - macOS Apple Silicon: migrazione-git-azure-devops_x.y.z_darwin_arm64.tar.gz
  - macOS Intel: migrazione-git-azure-devops_x.y.z_darwin_amd64.tar.gz
  - Windows AMD64: migrazione-git-azure-devops_x.y.z_windows_amd64.zip
  - Windows ARM64: migrazione-git-azure-devops_x.y.z_windows_arm64.zip

> Sui comandi di seguito, sostituisci `x.y.z` con la versione desiderata (es. `1.0.0-RC.4`).

Installazione sistema su ambienti Unix-like (richiede sudo e /usr/local/bin esistente):

```bash
# Linux AMD64
TMP="$(mktemp -d)"
curl -L -o "$TMP/migrazione-git-azure-devops_linux_amd64.tar.gz" \
  "https://github.com/amusarra/migrazione-git-azure-devops/releases/download/x.y.z/migrazione-git-azure-devops_x.y.z_linux_amd64.tar.gz"
tar -xzf "$TMP/migrazione-git-azure-devops_linux_amd64.tar.gz" -o -C "$TMP"
sudo install -m 0755 "$TMP/migrazione-git-azure-devops_linux_amd64" /usr/local/bin/migrazione-git-azure-devops
```

```bash
# macOS Apple Silicon (arm64)
TMP="$(mktemp -d)"
curl -L -o "$TMP/migrazione-git-azure-devops_darwin_arm64.tar.gz" \
  "https://github.com/amusarra/migrazione-git-azure-devops/releases/download/x.y.z/migrazione-git-azure-devops_x.y.z_darwin_arm64.tar.gz"
tar -xzf "$TMP/migrazione-git-azure-devops_darwin_arm64.tar.gz" -o -C "$TMP"
sudo install -m 0755 "$TMP/migrazione-git-azure-devops_darwin_arm64" /usr/local/bin/migrazione-git-azure-devops
```

Installazione sistema su Windows (PowerShell, copia in $HOME).

```bash
# Windows (PowerShell)
$TMP = New-Item -ItemType Directory -Path (Join-Path $env:TEMP (New-Guid))
Invoke-WebRequest -Uri "https://github.com/amusarra/migrazione-git-azure-devops/releases/download/x.y.z/migrazione-git-azure-devops_x.y.z_windows_amd64.zip" -OutFile "$TMP/migrazione-git-azure-devops.zip"
Expand-Archive -Path "$TMP/migrazione-git-azure-devops.zip" -DestinationPath "$TMP"
Copy-Item -Recurse -Force "$TMP/migrazione-git-azure-devops_windows_amd64.exe" "$HOME/migrazione-git-azure-devops.exe"
```

Facoltativo: verifica checksum (scarica checksums.txt dalla release e verifica l’hash).

Dopo l’installazione, verifica la versione:

```bash
# Esecuzione su Unix-like (Linux, macOS)
migrazione-git-azure-devops --version

# Esecuzione su Windows (PowerShell) da $HOME
.\migrazione-git-azure-devops.exe --version

# Esempio di output:
migrazione-git-azure-devops 1.0.0-RC.4
commit: 19dd541501d82a0d6fc274a01538ee67db6ff8ee
built:  2025-09-17T15:51:04Z
```

L'immagine seguente mostra l'output di `--version` e `--help` su di un sistema Microsoft Windows.

![screenshot-windows-help-version](docs/resources/images/verifica_dopo_installazione_tool_su_windows.jpg)

### Note per utenti Windows

- Se si usa PowerShell, potrebbe essere necessario modificare la [Execution Policy](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_execution_policies) per eseguire script scaricati (es. `Set-ExecutionPolicy RemoteSigned -Scope CurrentUser`).
- Windows Defender SmartScreen potrebbe bloccare l'esecuzione del tool scaricato. In tal caso, fare clic con il tasto destro sul file `.exe`, selezionare "Proprietà" e quindi "Sblocca" per consentire l'esecuzione (vedi immagine sotto).

![screenshot-windows-sblocco-file](docs/resources/images/program_on_windows_details_1.jpg)

Il blocco di SmartScreen avviene perché il file `.exe` non è firmato con un certificato riconosciuto da Microsoft. Per firmare il binario, è stato usato un certificato self-signed (non riconosciuto da Microsoft). Fare riferimento alla GitHub Action di release per i dettagli. A seguire un esempio di come vedere il certificato self-signed usato per la firma del binario Windows.

![screenshot-windows-certificato-self-signed](docs/resources/images/program_on_windows_details_3.jpg)

## Build e Release (per maintainer)

Snapshot con GoReleaser (artefatti in dist/).

> Accertarsi di avere GoReleaser installato (<https://goreleaser.com/install/>).

```bash
goreleaser release --clean --snapshot --skip=publish
```

Build nativa

```bash
go build -o bin/migrazione-git-azure-devops ./cmd/migrazione-git-azure-devops
```

CI (GitHub Actions)

- Lint con golangci-lint (vedi `.github/workflows/build.yml`)
- GoReleaser in modalità snapshot carica gli artefatti come artifact di workflow.
- La release completa (senza --snapshot) genera changelog e pubblica gli artefatti (vedi `.github/workflows/release.yml`).

## Funzionalità Report Migrazione

Il tool può generare un report dettagliato della migrazione in formato **JSON** e/o **HTML**. Questa funzionalità è utile per audit, troubleshooting e documentazione delle attività svolte.

### Come abilitare il report

Aggiungi il flag `--report-format` per scegliere uno o più formati (es. `--report-format json,html`).  
Specifica la directory di destinazione con `--report-path` (deve esistere), altrimenti il report viene salvato nella directory temporanea di sistema.

Esempio:

```bash
migrazione-git-azure-devops ... --report-format html,json --report-path /percorso/dove/salvare
```

### Informazioni contenute nel report

Il report include:

- Data e ora di inizio e fine migrazione
- Durata totale (in minuti)
- Hostname della macchina da cui è stata eseguita la migrazione
- Elenco dettagliato dei repository migrati con:
  - Nome repository
  - Esito (OK, errore, skipped, dry-run)
  - URL sorgente e destinazione
  - Numero e nomi dei branch migrati
  - Numero e nomi dei tag migrati
  - Dimensione del repository in byte

A seguire un esempio di output HTML.

![screenshot-report-html](docs/resources/images/report_html_example.jpg)

A seguire un esempio di output JSON.

```json
{
  "StartTime": "2024-06-01T10:00:00Z",
  "EndTime": "2024-06-01T10:05:12Z",
  "Duration": 5.2,
  "Hostname": "myhost.local",
  "Summaries": [
    {
      "Repo": "horse-core",
      "Result": "OK",
      "DstWebURL": "https://dev.azure.com/org/proj/_git/horse-core",
      "SrcWebURL": "https://dev.azure.com/org/proj/_git/horse-core",
      "NumBranches": 3,
      "BranchNames": ["main", "develop", "feature-x"],
      "NumTags": 2,
      "TagNames": ["v1.0.0", "v1.1.0"],
      "Size": 1234567
      // ...altri campi...
    }
    // ...
  ]
}
```

### Note

- Il nome del file report contiene un timestamp per garantire unicità.
- Se la directory specificata con `--report-path` non esiste, il tool mostra un errore.
- È possibile generare entrambi i formati contemporaneamente.

## Note e consigli

- PAT:
  - SRC_PAT richiesto sempre (anche per `--list-repos`)
  - DST_PAT richiesto quando si specifica la destinazione (migrazione)
- Trace:
  - abilita "[TRACE] ..." con URL richiesti
  - stampa il body delle risposte HTTP in errore
- Dry-run:
  - nessuna modifica lato Azure DevOps
  - utile per verificare filtri/lista e azioni che verranno eseguite
- Force-push:
  - sovrascrive lo stato della repo di destinazione (mirror + --force se già esiste)
