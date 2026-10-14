package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"jungle/migrations"
)

func main() {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://jungle:jungle_secret@localhost:5432/jungle_db?sslmode=disable"
	}

	flag.Parse()
	command := flag.Arg(0)
	if command == "" {
		command = "up"
	}

	switch command {
	case "up":
		err := migrations.Up(connStr)
		if err != nil {
			if errors.Is(err, migrations.ErrNoChange) {
				fmt.Println("Nenhuma migration pendente. Banco de dados ja esta atualizado!")
				return
			}
			log.Fatalf("falha ao executar migrations up: %v", err)
		}
		fmt.Println("Sucesso: migrations aplicadas com sucesso via golang-migrate!")

	case "down":
		err := migrations.Down(connStr)
		if err != nil {
			if errors.Is(err, migrations.ErrNoChange) {
				fmt.Println("Nenhuma migration para reverter.")
				return
			}
			log.Fatalf("falha ao executar migration down: %v", err)
		}
		fmt.Println("Sucesso: migration revertida com sucesso via golang-migrate!")

	case "version":
		v, dirty, err := migrations.Version(connStr)
		if err != nil {
			if errors.Is(err, migrations.ErrNoChange) {
				fmt.Println("Nenhuma migration aplicada ainda.")
				return
			}
			log.Fatalf("falha ao consultar versao: %v", err)
		}
		fmt.Printf("Versao atual: %d (dirty: %t)\n", v, dirty)

	default:
		log.Fatalf("comando desconhecido: %s. Use 'up', 'down' ou 'version'", command)
	}
}
