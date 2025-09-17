#!/usr/bin/env bash
set -euo pipefail

# Migrazione repo Azure DevOps da sorgente a destinazione con filtro regex/glob e mirror push.
# Dipendenze: bash, git, curl, jq
#
# Autenticazione:
#   export SRC_PAT="xxxx"   # PAT per org/progetto sorgente (scope: Code -> Read)
#   export DST_PAT="yyyy"   # PAT per org/progetto destinazione (scope: Code -> Read, write, & manage)
#
# Esempi:
#   ./migrazione_git_repo.sh --src-org myorg --src-project MyProject \
#                --dst-org targetorg --dst-project TargetProject \
#                --filter 'ansc-*'
#
#   ./migrazione_git_repo.sh -so myorg -sp MyProject -do targetorg -dp TargetProject -f '^ansc-(core|svc)-.*$' --dry-run
#
# Note:
# - Il filtro accetta sia regex (es. '^ansc-.*$') sia glob tipo shell (es. 'ansc-*').
# - In --dry-run NON vengono create repo in destinazione né eseguiti push: vengono solo mostrati i piani d'azione.
# - Il mirror replica TUTTI i ref (branch, tag) e rimuove quelli cancellati (comportamento simile a --prune).
# - Se la repo esiste già in destinazione, il push NON viene eseguito a meno di usare --force-push.
#
# Autore: Antonio Musarra <amusarr@sogei.it>
# Date: 2024-09-16
# Version: 1.0.1
#

SRC_ORG=""
SRC_PROJECT=""
DST_ORG=""
DST_PROJECT=""
FILTER_PATTERN=""
DRY_RUN=0
TRACE=0
FORCE_PUSH=0
REPO_LIST_FILE=""

declare -a MIGRATION_SUMMARY
LIST_REPOS=0

check_bash_version() {
  if ((BASH_VERSINFO[0] < 4)); then
    echo "Errore: è richiesta Bash versione >= 4. Versione corrente: $BASH_VERSION" >&2
    exit 1
  fi
}

check_dependencies() {
  local missing=0
  for cmd in curl jq git; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      echo "Errore: comando richiesto non trovato: $cmd" >&2
      missing=1
    fi
  done
  if ((missing)); then
    exit 1
  fi

  # Controllo versione jq >= 1.6
  local jq_ver
  jq_ver="$(jq --version 2>/dev/null | awk -F- '{print $2}')"
  if [[ -z "$jq_ver" || "$(printf '%s\n' "$jq_ver" "1.6" | sort -V | head -n1)" != "1.6" ]]; then
    echo "Errore: jq >= 1.6 richiesto. Versione corrente: ${jq_ver:-N/A}" >&2
    exit 1
  fi

  # Controllo versione curl >= 8
  local curl_ver
  curl_ver="$(curl --version 2>/dev/null | head -n1 | awk '{print $2}')"
  if [[ -z "$curl_ver" || "${curl_ver%%.*}" -lt 8 ]]; then
    echo "Errore: curl >= 8.x richiesto. Versione corrente: ${curl_ver:-N/A}" >&2
    exit 1
  fi
}

check_repo_list_file() {
  if [[ -n "$REPO_LIST_FILE" && ! -f "$REPO_LIST_FILE" ]]; then
    echo "Errore: file lista repo non trovato: $REPO_LIST_FILE" >&2
    exit 1
  fi
}

check_required_vars() {
  if [[ -z "${SRC_ORG}" || -z "${SRC_PROJECT}" || -z "${DST_ORG}" || -z "${DST_PROJECT}" ]]; then
    echo "Errore: specificare tutti i parametri obbligatori." >&2
    usage
    exit 1
  fi

  : "${SRC_PAT:?Variabile ambiente SRC_PAT mancante}"
  : "${DST_PAT:?Variabile ambiente DST_PAT mancante}"
}

glob_to_regex() {
  local pat="$1"
  # Se sembra già una regex, non toccarla
  if [[ "$pat" =~ [\^\$\(\)\|\+] ]]; then
    printf '%s' "$pat"
    return
  fi
  # Escapa metacaratteri regex, poi sostituisci * -> .*, ? -> .
  # 1) Escape: .[]{}()+^$|\  (e backslash stesso)
  local esc
  esc="$(printf '%s' "$pat" | sed -E 's/([][().{}+^$|\])/\\\1/g; s/\\/\\\\/g')"
  # 2) Sostituisci * e ?
  esc="$(printf '%s' "$esc" | sed -E 's/\*/.*/g; s/\? /./g; s/\?$/./')"
  printf '^%s$' "$esc"
}

api_get() {
  # $1=ORG $2=PROJECT(optional or '-') $3=PATH $4=PAT
  local org="$1" proj="$2" path="$3" pat_var="$4"
  local base="https://dev.azure.com/${org}"
  local url
  if [[ "$proj" == "-" || -z "$proj" ]]; then
    url="${base}/${path}"
  else
    url="${base}/$(urlencode "$proj")/${path}"
  fi
  # Cattura body e status code senza sopprimere errori
  local resp http_code
  resp="$(curl -sS -u ":${pat_var}" -w $'\n%{http_code}' "$url")" || {
    echo "Errore cURL durante la richiesta API: ${url}" >&2
    return 1
  }
  http_code="${resp##*$'\n'}"
  resp="${resp%$'\n'*}"
  if [[ ! "$http_code" =~ ^2[0-9]{2}$ ]]; then
    echo "Errore API Azure DevOps (HTTP ${http_code}) su: ${url}" >&2
    # Mostra il body ricevuto (HTML/JSON) per diagnosi
    echo "$resp" >&2
    return 1
  fi
  printf '%s' "$resp"
}

api_post_json() {
  # $1=ORG $2=PROJECT $3=PATH $4=JSON_BODY $5=PAT
  local org="$1" proj="$2" path="$3" body="$4" pat="$5"
  local url="https://dev.azure.com/${org}/$(urlencode "$proj")/${path}"
  local resp http_code
  resp="$(curl -sS -u ":${pat}" -H "Content-Type: application/json" -X POST -d "$body" -w $'\n%{http_code}' "$url")" || {
    echo "Errore cURL durante la chiamata POST: ${url}" >&2
    return 1
  }
  http_code="${resp##*$'\n'}"
  resp="${resp%$'\n'*}"
  if [[ ! "$http_code" =~ ^20(0|1)$ ]]; then
    echo "Errore API Azure DevOps (HTTP ${http_code}) su: ${url}" >&2
    echo "$resp" >&2
    return 1
  fi
  printf '%s' "$resp"
}

urlencode() {
  jq -rn --arg s "$1" '$s|@uri'
}

print_repo_list() {
  local org="$1" project="$2" pat="$3"
  local api="_apis/git/repositories?api-version=7.1"
  local json=""
  if ! json="$(api_get "$org" "$project" "$api" "$pat")"; then
    echo "Errore nel recupero dei repository sorgente." >&2
    return 1
  fi
  if [[ -z "$json" || "$(echo "$json" | jq -r '.count // 0')" = "0" ]]; then
    echo "Nessun repository trovato in ${org}/${project}."
    return 0
  fi
  echo "Repository disponibili in ${org}/${project}:"
  echo
  echo "$json" | jq -r '.value[] | "- \(.name)\n    cloneUrl: \(.remoteUrl)\n    webUrl: \(.webUrl)"'
  echo
}

usage() {
  cat <<'EOF'
Uso:
  migrazione_git_repo.sh --src-org ORG --src-project PROJ \
             --dst-org ORG --dst-project PROJ \
             --filter PATTERN [--repo-list FILE] [--dry-run] [--trace] [--force-push] [--list-repos]

Opzioni:
  --src-org,   -so   Organizzazione sorgente (Azure DevOps)
  --src-project,-sp  Progetto sorgente
  --dst-org,   -do   Organizzazione destinazione
  --dst-project,-dp  Progetto destinazione
  --filter,    -f    Filtro su nome repo (regex o glob tipo 'ansc-*')
  --repo-list, -rl   File con lista di repository da migrare (uno per riga, ignora --filter)
  --dry-run           Non crea/pusha sulla destinazione; stampa solo cosa farebbe
  --trace,     -t     Abilita il trace (set -x)
  --force-push,-fp    Esegue il push anche se la repo esiste già in destinazione
  --list-repos        Mostra la lista dei repository disponibili nella sorgente e termina
  -h, --help          Mostra questo aiuto

Autenticazione:
  Esportare le variabili d'ambiente:
    SRC_PAT  PAT per la sorgente (almeno Code Read)
    DST_PAT  PAT per org/progetto destinazione (scope: Code -> Read, write, & manage)

Esempi:
  migrazione_git_repo.sh -so srcorg -sp SrcProj -do dstorg -dp DstProj -f 'ansc-*'
  migrazione_git_repo.sh -so srcorg -sp SrcProj -do dstorg -dp DstProj -f '^ansc-.*$' --dry-run
EOF
}

main() {
  check_bash_version
  check_dependencies
  check_required_vars

  if [[ $TRACE -eq 1 ]]; then
    set -x
  fi

  check_repo_list_file

  REGEX_FILTER="$(glob_to_regex "$FILTER_PATTERN")"

  echo ">>> Sorgente:      ${SRC_ORG}/${SRC_PROJECT}"
  echo ">>> Destinazione:  ${DST_ORG}/${DST_PROJECT}"
  echo ">>> Filtro (regex): ${REGEX_FILTER}"
  if [[ $DRY_RUN -eq 1 ]]; then
    echo ">>> Modalità: DRY-RUN (nessuna creazione/push in destinazione)"
  fi
  echo

  # 1) Elenco repo sorgente
  SRC_API="_apis/git/repositories?api-version=7.1"
  SRC_REPOS_JSON=""
  if ! SRC_REPOS_JSON="$(api_get "$SRC_ORG" "$SRC_PROJECT" "$SRC_API" "$SRC_PAT")"; then
    echo "Errore nella chiamata API per la sorgente. Interrompo." >&2
    exit 1
  fi
  if [[ -z "$SRC_REPOS_JSON" || "$(echo "$SRC_REPOS_JSON" | jq -r '.count // 0')" = "0" ]]; then
    echo "Nessun repository trovato nella sorgente, o accesso negato." >&2
    exit 1
  fi

  # 2) Filtra repo per nome via jq + regex oppure usa lista da file
  if [[ -n "$REPO_LIST_FILE" ]]; then
    mapfile -t MATCHED_REPOS < "$REPO_LIST_FILE"
    echo "Repository da migrare (da file) (${#MATCHED_REPOS[@]}):"
  else
    mapfile -t MATCHED_REPOS < <(echo "$SRC_REPOS_JSON" \
      | jq -r --arg re "$REGEX_FILTER" '.value[] | select(.name | test($re)) | .name' )
    echo "Repository da migrare (${#MATCHED_REPOS[@]}):"
  fi

  if [[ ${#MATCHED_REPOS[@]} -eq 0 ]]; then
    echo "Nessun repository da migrare."
    exit 0
  fi
  for r in "${MATCHED_REPOS[@]}"; do echo "  - $r"; done
  echo

  # 3) Elenco repo destinazione (per verificare esistenza)
  DST_API="_apis/git/repositories?api-version=7.1"
  DST_REPOS_JSON=""
  if ! DST_REPOS_JSON="$(api_get "$DST_ORG" "$DST_PROJECT" "$DST_API" "$DST_PAT")"; then
    echo "Errore nella chiamata API per la destinazione. Interrompo." >&2
    exit 1
  fi
  declare -A DST_REPO_EXISTS
  if [[ -n "$DST_REPOS_JSON" ]]; then
    while IFS= read -r name; do
      name="$(echo "$name" | tr -d '\r\n' | xargs)"
      DST_REPO_EXISTS["$name"]=1
    done < <(echo "$DST_REPOS_JSON" | jq -r '.value[]?.name')
  fi

  # 4) Per ogni repo: crea (se manca) e push --mirror
  TMPDIR="$(pwd)/tmp_migrazione_git_$RANDOM"
  mkdir -p "$TMPDIR"
  echo "Usando directory temporanea: $TMPDIR"
  cleanup() { rm -rf "$TMPDIR"; }
  trap cleanup EXIT

  for REPO in "${MATCHED_REPOS[@]}"; do
    # Rimuovi eventuali spazi e caratteri di fine riga (\r, \n) sempre, anche se non usi --repo-list
    REPO="$(echo "$REPO" | tr -d '\r\n' | xargs)"

    echo "=== Repo: $REPO ==="
    REPO_ENC="$(urlencode "$REPO")"
    SRC_URL="https://$(urlencode 'user'):${SRC_PAT}@dev.azure.com/${SRC_ORG}/$(urlencode "$SRC_PROJECT")/_git/${REPO_ENC}"
    DST_URL="https://$(urlencode 'user'):${DST_PAT}@dev.azure.com/${DST_ORG}/$(urlencode "$DST_PROJECT")/_git/${REPO_ENC}"
    SRC_URL_REDACTED="https://user:***@dev.azure.com/${SRC_ORG}/$(urlencode "$SRC_PROJECT")/_git/${REPO_ENC}"
    DST_URL_REDACTED="https://user:***@dev.azure.com/${DST_ORG}/$(urlencode "$DST_PROJECT")/_git/${REPO_ENC}"

    PUSH_ALLOWED=1
    STATUS=""

    # 1) Controlla se la repo esiste già in destinazione
    if [[ -n "${DST_REPO_EXISTS[$REPO]:-}" ]]; then
      echo "  Repo già presente in destinazione."
      if [[ $FORCE_PUSH -ne 1 ]]; then
        echo "  Push NON eseguito (repo già presente). Usa --force-push per forzare."
        PUSH_ALLOWED=0
        STATUS="SKIPPED: repo già presente"
        MIGRATION_SUMMARY+=("$REPO | $STATUS | $DST_URL_REDACTED | $DST_URL_REDACTED")
        echo
        continue
      fi
    fi

    # 2) Clona il repo sorgente (solo se non dry-run)
    if [[ $DRY_RUN -eq 1 ]]; then
      STATUS="DRY-RUN"
      echo "  [DRY] Clonerei in mirror dal sorgente:"
      echo "        git clone --mirror '$SRC_URL_REDACTED' '${TMPDIR}/${REPO}.git'"
    else
      echo "  Clono (mirror) dal sorgente..."
      if ! git clone --quiet --mirror "$SRC_URL" "${TMPDIR}/${REPO}.git"; then
        echo "  Errore: repository sorgente non trovato o accesso negato: $REPO"
        echo "  Non creo la repo in destinazione. Continuo con il prossimo repository."
        STATUS="ERRORE: sorgente non trovato"
        MIGRATION_SUMMARY+=("$REPO | $STATUS | -")
        echo
        continue
      fi
    fi

    # 3) Crea repo in destinazione se non esiste
    if [[ -z "${DST_REPO_EXISTS[$REPO]:-}" ]]; then
      if [[ $DRY_RUN -eq 1 ]]; then
        STATUS="DRY-RUN"
        echo "  [DRY] Creerei la repo in destinazione: $REPO"
      else
        echo "  Creo la repo in destinazione: $REPO"
        RESPONSE=""
        if ! RESPONSE="$(api_post_json "$DST_ORG" "$DST_PROJECT" "_apis/git/repositories?api-version=7.1" "$(jq -cn --arg name "$REPO" '{name:$name}')" "$DST_PAT")"; then
          echo "  Errore nella creazione della repo: $REPO" >&2
          echo "  Output API:" >&2
          echo "$RESPONSE" >&2
          STATUS="ERRORE: creazione destinazione"
          MIGRATION_SUMMARY+=("$REPO | $STATUS | -")
          echo
          continue
        fi
        DST_REPO_EXISTS["$REPO"]=1
      fi
    fi

    # 4) Push --mirror verso destinazione
    if [[ $PUSH_ALLOWED -eq 1 ]]; then
      if [[ $DRY_RUN -eq 1 ]]; then
        STATUS="DRY-RUN"
        if [[ $FORCE_PUSH -eq 1 ]]; then
          echo "  [DRY] Eseguirei push --mirror --force verso destinazione:"
          echo "        (cd '${TMPDIR}/${REPO}.git' && git remote add dest '$DST_URL_REDACTED' && git push --mirror --force dest)"
        else
          echo "  [DRY] Eseguirei push --mirror verso destinazione:"
          echo "        (cd '${TMPDIR}/${REPO}.git' && git remote add dest '$DST_URL_REDACTED' && git push --mirror dest)"
        fi
      else
        echo "  Push --mirror verso destinazione..."
        (
          cd "${TMPDIR}/${REPO}.git"
          git remote add dest "$DST_URL"
          if [[ $FORCE_PUSH -eq 1 ]]; then
            git push --quiet --mirror --force dest
          else
            git push --quiet --mirror dest
          fi
        )
        echo "  OK."
        STATUS="OK"
      fi
    fi

    # Salva riepilogo
    MIGRATION_SUMMARY+=("$REPO | $STATUS | $DST_URL_REDACTED | $DST_URL_REDACTED")

    echo
  done

  echo ">>> Completato."

  # Riepilogo finale
  echo
  echo "===== RIEPILOGO MIGRAZIONE ====="
  printf "%-40s | %-25s | %-50s | %s\n" "Repository" "Esito" "Azure URL" "URL di clone"
  printf "%-40s-+-%-25s-+-%-50s-+-%s\n" "----------------------------------------" "-------------------------" "--------------------------------------------------" "---------------------------------------------"
  for entry in "${MIGRATION_SUMMARY[@]}"; do
    IFS='|' read -r repo status url clone <<< "$entry"
    status_trimmed="$(echo "$status" | xargs)"
    if [[ "$status_trimmed" =~ ^(ERRORE|SKIPPED) ]]; then
      printf "\033[31m%-40s | %-25s | %-50s | %s\033[0m\n" "$repo" "$status_trimmed" "$url" "$clone"
    else
      printf "%-40s | %-25s | %-50s | %s\n" "$repo" "$status_trimmed" "$url" "$clone"
    fi
  done
  echo "================================"
}

# --- INIZIO SCRIPT ---

while [[ $# -gt 0 ]]; do
  case "$1" in
    --src-org|-so)     SRC_ORG="${2:-}"; shift 2 ;;
    --src-project|-sp) SRC_PROJECT="${2:-}"; shift 2 ;;
    --dst-org|-do)     DST_ORG="${2:-}"; shift 2 ;;
    --dst-project|-dp) DST_PROJECT="${2:-}"; shift 2 ;;
    --filter|-f)       FILTER_PATTERN="${2:-}"; shift 2 ;;
    --repo-list|-rl)   REPO_LIST_FILE="${2:-}"; shift 2 ;;
    --dry-run)         DRY_RUN=1; shift ;;
    --trace|-t)        TRACE=1; shift ;;
    --force-push|-fp)  FORCE_PUSH=1; shift ;;
    --list-repos)      LIST_REPOS=1; shift ;;
    -h|--help)         usage; exit 0 ;;
    *) echo "Argomento sconosciuto: $1"; usage; exit 1 ;;
  esac
done

if [[ $LIST_REPOS -eq 1 ]]; then
  check_bash_version
  check_dependencies
  if [[ -z "${SRC_ORG}" || -z "${SRC_PROJECT}" ]]; then
    echo "Errore: --src-org e --src-project sono obbligatori con --list-repos" >&2
    usage
    exit 1
  fi
  : "${SRC_PAT:?Variabile ambiente SRC_PAT mancante}"
  [[ $TRACE -eq 1 ]] && set -x
  if ! print_repo_list "$SRC_ORG" "$SRC_PROJECT" "$SRC_PAT"; then
    exit 1
  fi
  exit 0
fi

main "$@"
