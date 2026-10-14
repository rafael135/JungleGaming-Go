package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"go.uber.org/fx/fxtest"
)

func TestAppModuleLifecycle(t *testing.T) {
	// Cria uma aplicação Fx de teste que valida o grafo de dependências e os hooks de lifecycle
	app := fxtest.New(t, AppModule)

	// 1. Inicia todos os serviços (HTTP, SQS consumer, workers, postgres pool)
	startCtx, cancelStart := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelStart()

	if err := app.Start(startCtx); err != nil {
		t.Fatalf("falha ao iniciar aplicacao Fx: %v", err)
	}

	// 2. Verifica se o servidor HTTP está respondendo no health check
	time.Sleep(200 * time.Millisecond)
	resp, err := http.Get("http://localhost:8081/health/live")
	if err != nil {
		t.Fatalf("falha ao chamar /health/live do servidor Fx: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("esperava status 200 no health live, recebeu %d", resp.StatusCode)
	}

	// 3. Encerra a aplicação graciosamente (OnStop de todos os hooks: workers, consumer, server, DB)
	stopCtx, cancelStop := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelStop()

	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("falha ao parar aplicacao Fx graciosamente: %v", err)
	}
}
