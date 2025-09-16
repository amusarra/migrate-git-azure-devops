---
title: "Guida Migrazione Repository Azure DevOps"
author: "Antonio Musarra <amusarra@sogei.it>"
creator: "Antonio Musarra"
subject: "Guida Migrazione Repository Azure DevOps"
keywords: [azure, devops, git, migration, repository, bash, script, cli]
lang: it
layout: article
slug: "guida-migrazione-repository-azure-devops"
date: "2025-09-15"
version: "1.0.0"
scope: Public
state: Released
---

## Cronologia delle revisioni

| Versione | Data       | Autore          | Descrizione delle Modifiche |
| :------- | :--------- | :-------------- | :-------------------------- |
| 1.0.0    | 2025-09-02 | Antonio Musarra | Prima release               |

[TOC]

<div style="page-break-after: always; break-after: page;"></div>

Questo script consente di migrare repository Git tra progetti/organizzazioni Azure DevOps, con supporto a filtri, lista repo, dry-run e mirror push.

Un classico caso d'uso è la migrazione di tutte le repo di un progetto sorgente in un progetto destinazione, ad esempio per consolidare progetti o spostare repo tra organizzazioni o meglio servizi ICT che sono mappati su team project.

## Prerequisiti

- Bash >= 4
- jq >= 1.6
- curl >= 8.x
- git
- Variabili d'ambiente:  
  - `SRC_PAT` (Personal Access Token sorgente, scope: Code Read)
  - `DST_PAT` (Personal Access Token destinazione, scope: Code Read, Write & Manage)

## Uso Base

```bash
./migrazione_git_repo.sh --src-org myorg --src-project MyProject \
  --dst-org targetorg --dst-project TargetProject \
  --filter 'ansc-*'
```

Console 1 - Uso base con filtro glob

A seguire l'esempio di output per il comando sopra che esegue la migrazione dei repository che matchano il filtro `ansc-*` dal progetto `ansc-tool-test-coop-services` dell'organizzazione `amusarra` al progetto `ansc-tool-test-coop-services-mirror` dell'organizzazione `amusarra`. In questo caso l'orgnanizzazione è la stessa (sia sorgente che destinazione), ma potrebbe essere diversa.

```plaintext
>>> Sorgente:      amusarra/ansc-tool-test-coop-services
>>> Destinazione:  amusarra/ansc-tool-test-coop-services-mirror
>>> Filtro (regex): ansc-*

Repository da migrare (1):
  - ansc-tool-test-coop-services

Usando directory temporanea: /Users/amusarra/dev/tools/ansc/git/migrazione/tmp_migrazione_git_13809
=== Repo: ansc-tool-test-coop-services ===
  Clono (mirror) dal sorgente...
  Creo la repo in destinazione: ansc-tool-test-coop-services
  Push --mirror verso destinazione...
  OK.

>>> Completato.

===== RIEPILOGO MIGRAZIONE =====
Repository                               | Esito                     | Azure URL                                          | URL di clone
-----------------------------------------+---------------------------+----------------------------------------------------+----------------------------------------------
ansc-tool-test-coop-services            | OK                        |  https://user:***@dev.azure.com/amusarra/ansc-tool-test-coop-services-mirror/_git/ansc-tool-test-coop-services  |  https://user:***@dev.azure.com/amusarra/ansc-tool-test-coop-services-mirror/_git/ansc-tool-test-coop-services
================================
```

Output 1 - Uso base con filtro glob

## Esempi

### Migrazione con filtro glob

```bash
./migrazione_git_repo.sh -so srcorg -sp SrcProj -do dstorg -dp DstProj -f 'ansc-*'
```

Console 2 - Uso base con filtro glob

### Migrazione con filtro regex

```bash
./migrazione_git_repo.sh -so srcorg -sp SrcProj -do dstorg -dp DstProj -f '^ansc-(core|svc)-.*$'
```

Console 3 - Uso base con filtro regex

### Dry-run (simulazione, nessuna modifica)

Questo comando mostra le repo che verrebbero migrate senza eseguire alcuna operazione. Usando questa modalità possiamo verificare quali repo verrebbero migrate prima di eseguire la migrazione e anche il dettaglio delle operazioni che verrebbero eseguite.

```bash
./migrazione_git_repo.sh -so srcorg -sp SrcProj -do dstorg -dp DstProj -f '^ansc-.*$' --dry-run
```

Console 4 - Dry-run (simulazione, nessuna modifica)

A seguire l'esempio di parte dell'output:

```plaintext
...
>>> Sorgente:      amusarra/ansc-tool-test-coop-services
>>> Destinazione:  amusarra/ansc-tool-test-coop-services-mirror
>>> Filtro (regex): ansc-*
>>> Modalità: DRY-RUN (nessuna creazione/push in destinazione)

Repository da migrare (1):
  - ansc-tool-test-coop-services

Usando directory temporanea: /Users/amusarra/dev/tools/ansc/git/migrazione/tmp_migrazione_git_19737
=== Repo: ansc-tool-test-coop-services ===
  [DRY] Clonerei in mirror dal sorgente:
        git clone --mirror 'https://user:***@dev.azure.com/amusarra/ansc-tool-test-coop-services/_git/ansc-tool-test-coop-services' '/Users/amusarra/dev/tools/ansc/git/migrazione/tmp_migrazione_git_19737/ansc-tool-test-coop-services.git'
  [DRY] Creerei la repo in destinazione: ansc-tool-test-coop-services
  [DRY] Eseguirei push --mirror verso destinazione:
        (cd '/Users/amusarra/dev/tools/ansc/git/migrazione/tmp_migrazione_git_19737/ansc-tool-test-coop-services.git' && git remote add dest 'https://user:***@dev.azure.com/amusarra/ansc-tool-test-coop-services-mirror/_git/ansc-tool-test-coop-services' && git push --mirror dest)
...
```

Output 2 - Dry-run (simulazione, nessuna modifica)

### Migrazione di una lista di repository

Puoi specificare una lista di repository da migrare usando l'opzione `--repo-list`.
Crea un file `repo_list.txt` con i nomi delle repo (una per riga):

```plaintext
ansc-core
ansc-svc
ansc-tool-test-coop-services
```

File 1 - Contenuto di `repo_list.txt` che contiene i nomi delle repo da migrare

Esegui lo script:

```bash
./migrazione_git_repo.sh -so srcorg -sp SrcProj -do dstorg -dp DstProj --repo-list repo_list.txt
```

Console 5 - Migrazione di una lista di repository

### Forzare il push su repo già esistente

Può essere utile se la repo esiste già in destinazione e si vuole sovrascrivere il contenuto (ad esempio per retry di una migrazione fallita).

> **Importante**: Il processo aziendale definito in SOGEI, impone che la creazione dei repository deve avvenire tramite il [Portale ALM](https://portalealm.sogei.it/) usando l'apposita funzionalità "Nuovo Repository". Occorre assicurarsi che i repository da migrare esistano già in destinazione prima di eseguire lo script con l'opzione `--force-push`. Lo script è capace di creare il repository sulla destinazione solo se non esiste già e se si hanno i permessi necessari ma nel caso di Team Project gestiti tramite il Portale ALM, la creazione deve avvenire tramite il Portale stesso.

Senza `--force-push`, se la repo esiste già in destinazione, il push non viene eseguito e la migrazione di quella repo viene saltata.

```plaintext
Repository da migrare (da file) (3):
  - ansc-tool-non-esistente
  - ansc-tool-test-coop-services
  - ansc-tool-non-esistente-1

Usando directory temporanea: /Users/amusarra/dev/tools/ansc/git/migrazione/tmp_migrazione_git_3553
=== Repo: ansc-tool-non-esistente ===
  Clono (mirror) dal sorgente...
remote: TF401019: The Git repository with name or identifier ansc-tool-non-esistente does not exist or you do not have permissions for the operation you are attempting.
fatal: repository 'https://dev.azure.com/amusarra/ansc-tool-test-coop-services/_git/ansc-tool-non-esistente/' not found
  Errore: repository sorgente non trovato o accesso negato: ansc-tool-non-esistente
  Non creo la repo in destinazione. Continuo con il prossimo repository.

=== Repo: ansc-tool-test-coop-services ===
  Repo già presente in destinazione.
  Push NON eseguito (repo già presente). Usa --force-push per forzare.
```

Output 3 - Output nel caso di repo già esistente

Con `--force-push`, il push viene eseguito anche se la repo esiste già in destinazione, sovrascrivendo il contenuto.

```bash
./migrazione_git_repo.sh -so srcorg -sp SrcProj -do dstorg -dp DstProj -f 'ansc-*' --force-push
```

Console 6 - Forzare il push su repo già esistente

### Abilitare il trace (debug)

Per abilitare il trace (debug) e vedere i comandi eseguiti, usare l'opzione `--trace`.

```bash
./migrazione_git_repo.sh ...altri parametri... --trace
```

Console 7 - Abilitare il trace (debug)

## Note

- Il filtro accetta sia glob (`ansc-*`) che regex (`^ansc-.*$`).
- In dry-run non vengono create repo né eseguiti push.
- Il mirror replica tutti i branch/tag e rimuove quelli cancellati.
- Se la repo esiste già in destinazione, il push NON viene eseguito a meno di usare `--force-push`.
- La directory temporanea usata per il clone viene cancellata automaticamente.

## Supporto

Per problemi o richieste, contattare: Antonio Musarra (<amusarr@sogei.it>)
