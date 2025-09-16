APP_NAME = migrazione-git-azure-devops
CMD_DIR = ./cmd/$(APP_NAME)
BUILD_DIR = ./bin

# Default target
.DEFAULT_GOAL := help

## build: Compila il progetto
build:
	@echo ">> Compilazione..."
	@go build -o $(BUILD_DIR)/$(APP_NAME) $(CMD_DIR)

## run: Esegue l'applicazione
run: build
	@echo ">> Avvio applicazione..."
	@$(BUILD_DIR)/$(APP_NAME)

## test: Esegue i test
test:
	@echo ">> Esecuzione test..."
	@go test ./... -v

## lint: Analizza il codice con golangci-lint
lint:
	@echo ">> Analisi statica..."
	@golangci-lint run

## clean: Rimuove i file compilati
clean:
	@echo ">> Pulizia..."
	@rm -rf $(BUILD_DIR)

## deps: Aggiorna le dipendenze
deps:
	@echo ">> Aggiornamento dipendenze..."
	@go mod tidy

## install: Installa l'eseguibile nel GOPATH/bin
install: build
	@echo ">> Installazione eseguibile..."
	@go install $(CMD_DIR)

## release: Compila per più piattaforme (esempio Linux e Windows)
release:
	@echo ">> Build cross-compilation..."
	@GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(APP_NAME)-linux $(CMD_DIR)
	@GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(APP_NAME).exe $(CMD_DIR)

## help: Mostra i comandi disponibili
help:
	@grep -E '^##' Makefile | sed -e 's/## //'