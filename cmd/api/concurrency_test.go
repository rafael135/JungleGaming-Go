package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"jungle/internal/application"

	"github.com/google/uuid"
	"go.uber.org/fx/fxtest"
)

func TestConcurrencyRequirements(t *testing.T) {
	// Obtem tokens do Keycloak
	tokenInternal := fetchToken(t, "internal-service", "internal-service-secret")
	tokenProviderA := fetchToken(t, "provider-a", "provider-a-secret")

	// Inicia aplicação Fx
	app := fxtest.New(t, AppModule)
	startCtx, cancelStart := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelStart()

	if err := app.Start(startCtx); err != nil {
		t.Fatalf("falha ao iniciar aplicacao Fx: %v", err)
	}
	defer func() {
		stopCtx, cancelStop := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelStop()
		_ = app.Stop(stopCtx)
	}()

	time.Sleep(200 * time.Millisecond)
	baseURL := "http://localhost:8081"
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Helper para criar carteira
	createTestWallet := func(t *testing.T, initialAmount string) (string, string) {
		playerID := uuid.NewString()
		body := fmt.Sprintf(`{
			"playerId": "%s",
			"initialBalance": {"amount": "%s", "currency": "BRL"}
		}`, playerID, initialAmount)

		req, _ := http.NewRequest(http.MethodPost, baseURL+"/wallets", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tokenInternal)
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("erro ao criar carteira: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("falha ao criar carteira (%d): %s", resp.StatusCode, string(b))
		}

		var out application.CreateWalletOutput
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out.ID, playerID
	}

	// =========================================================================
	// CENÁRIO 1: Envie a mesma aposta 50 vezes em paralelo e comprove um único débito
	// =========================================================================
	t.Run("1. Mesma aposta enviada 50 vezes em paralelo gera unico debito", func(t *testing.T) {
		walletID, playerID := createTestWallet(t, "1000.00")
		extTxID := "ext-dup-50-" + uuid.NewString()[:8]
		idempKey := "provider-a:" + extTxID

		payload := fmt.Sprintf(`{
			"providerId": "provider-a",
			"externalTransactionId": "%s",
			"playerId": "%s",
			"walletId": "%s",
			"roundId": "round-dup-1",
			"gameId": "game-1",
			"kind": "BET",
			"money": {"amount": "100.00", "currency": "BRL"}
		}`, extTxID, playerID, walletID)

		const concurrency = 50
		var wg sync.WaitGroup
		var successCount int64
		var replayCount int64
		var initialCount int64

		startBarrier := make(chan struct{})

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startBarrier // Aguarda todos os goroutines estarem prontos para disparar simultaneamente

				req, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", strings.NewReader(payload))
				req.Header.Set("Authorization", "Bearer "+tokenProviderA)
				req.Header.Set("Idempotency-Key", idempKey)
				req.Header.Set("Content-Type", "application/json")

				resp, err := httpClient.Do(req)
				if err != nil {
					t.Errorf("erro na chamada concorrente: %v", err)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode == http.StatusOK {
					atomic.AddInt64(&successCount, 1)
					var out application.ProcessWagerOutput
					if err := json.NewDecoder(resp.Body).Decode(&out); err == nil {
						if out.IdempotentReplay {
							atomic.AddInt64(&replayCount, 1)
						} else {
							atomic.AddInt64(&initialCount, 1)
						}
					}
				} else {
					b, _ := io.ReadAll(resp.Body)
					t.Errorf("status inesperado: %d body: %s", resp.StatusCode, string(b))
				}
			}()
		}

		// Disparo simultâneo das 50 requisições
		close(startBarrier)
		wg.Wait()

		if successCount != concurrency {
			t.Fatalf("esperava %d sucessos, obteve %d", concurrency, successCount)
		}
		if initialCount != 1 {
			t.Errorf("esperava exatamente 1 processamento original, obteve %d", initialCount)
		}
		if replayCount != concurrency-1 {
			t.Errorf("esperava %d replays, obteve %d", concurrency-1, replayCount)
		}

		// Confere o saldo final via reconciliação
		recReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/wallets/%s/reconciliation", baseURL, walletID), nil)
		recReq.Header.Set("Authorization", "Bearer "+tokenInternal)
		recResp, err := httpClient.Do(recReq)
		if err != nil {
			t.Fatalf("falha ao reconciliar: %v", err)
		}
		defer recResp.Body.Close()

		var recOut application.ReconcileWalletOutput
		_ = json.NewDecoder(recResp.Body).Decode(&recOut)

		if !recOut.Consistent {
			t.Errorf("carteira inconsistente apos 50 chamadas concorrentes")
		}
		if recOut.StoredBalance.Amount != "900.00" {
			t.Errorf("saldo esperado 900.00 (um unico debito de 100.00), obteve %s", recOut.StoredBalance.Amount)
		}
		if recOut.CheckedEntries != 2 {
			t.Errorf("esperava exatamente 2 lancamentos no ledger (abertura + 1 aposta), obteve %d", recOut.CheckedEntries)
		}
	})

	// =========================================================================
	// CENÁRIO 2: Disputa de duas apostas de 80.00 sobre saldo de 100.00
	// =========================================================================
	t.Run("2. Disputa de duas apostas de 80.00 sobre saldo de 100.00", func(t *testing.T) {
		walletID, playerID := createTestWallet(t, "1000.00") // Saldo inicial 100.00
		_ = walletID
		_ = playerID

		walletID100, playerID100 := createTestWallet(t, "100.00")

		extTx1 := "ext-race-1-" + uuid.NewString()[:8]
		extTx2 := "ext-race-2-" + uuid.NewString()[:8]

		payload1 := fmt.Sprintf(`{
			"providerId": "provider-a",
			"externalTransactionId": "%s",
			"playerId": "%s",
			"walletId": "%s",
			"roundId": "round-race-1",
			"gameId": "game-1",
			"kind": "BET",
			"money": {"amount": "80.00", "currency": "BRL"}
		}`, extTx1, playerID100, walletID100)

		payload2 := fmt.Sprintf(`{
			"providerId": "provider-a",
			"externalTransactionId": "%s",
			"playerId": "%s",
			"walletId": "%s",
			"roundId": "round-race-2",
			"gameId": "game-1",
			"kind": "BET",
			"money": {"amount": "80.00", "currency": "BRL"}
		}`, extTx2, playerID100, walletID100)

		var wg sync.WaitGroup
		var statusCodes [2]int
		var bodies [2][]byte

		startBarrier := make(chan struct{})

		sendBet := func(idx int, extID, payload string) {
			defer wg.Done()
			<-startBarrier

			req, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", strings.NewReader(payload))
			req.Header.Set("Authorization", "Bearer "+tokenProviderA)
			req.Header.Set("Idempotency-Key", "provider-a:"+extID)
			req.Header.Set("Content-Type", "application/json")

			resp, err := httpClient.Do(req)
			if err != nil {
				t.Errorf("erro no envio: %v", err)
				return
			}
			defer resp.Body.Close()

			statusCodes[idx] = resp.StatusCode
			bodies[idx], _ = io.ReadAll(resp.Body)
		}

		wg.Add(2)
		go sendBet(0, extTx1, payload1)
		go sendBet(1, extTx2, payload2)

		close(startBarrier)
		wg.Wait()

		// Uma deve ser PROCESSED (aposta aceita) e a outra REJECTED (saldo insuficiente)
		var processedCount, rejectedCount int
		for i := 0; i < 2; i++ {
			if statusCodes[i] == http.StatusOK {
				var out application.ProcessWagerOutput
				if err := json.Unmarshal(bodies[i], &out); err == nil {
					if out.Status == "PROCESSED" {
						processedCount++
					} else if out.Status == "REJECTED" {
						rejectedCount++
						if out.FailureCode != "INSUFFICIENT_FUNDS" {
							t.Errorf("esperava FailureCode INSUFFICIENT_FUNDS, obteve %s", out.FailureCode)
						}
					}
				}
			} else {
				t.Errorf("status inesperado no indice %d: %d body: %s", i, statusCodes[i], string(bodies[i]))
			}
		}

		if processedCount != 1 || rejectedCount != 1 {
			t.Fatalf("esperava 1 processada (PROCESSED) e 1 rejeitada (REJECTED), obteve %d processadas e %d rejeitadas", processedCount, rejectedCount)
		}

		// Reconciliação confirma saldo final de exatamente 20.00 BRL
		recReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/wallets/%s/reconciliation", baseURL, walletID100), nil)
		recReq.Header.Set("Authorization", "Bearer "+tokenInternal)
		recResp, err := httpClient.Do(recReq)
		if err != nil {
			t.Fatalf("falha ao reconciliar: %v", err)
		}
		defer recResp.Body.Close()

		var recOut application.ReconcileWalletOutput
		_ = json.NewDecoder(recResp.Body).Decode(&recOut)

		if !recOut.Consistent {
			t.Errorf("carteira inconsistente apos disputa de saldo")
		}
		if recOut.StoredBalance.Amount != "20.00" {
			t.Errorf("saldo esperado 20.00 (100.00 - 80.00), obteve %s", recOut.StoredBalance.Amount)
		}
		if recOut.CheckedEntries != 2 {
			t.Errorf("esperava 2 lancamentos (abertura + 1 aposta aceita), obteve %d", recOut.CheckedEntries)
		}
	})

	// =========================================================================
	// CENÁRIO 3: Processamento simultâneo de carteiras distintas
	// =========================================================================
	t.Run("3. Processamento de carteiras distintas simultaneamente", func(t *testing.T) {
		const numWallets = 10
		var walletIDs [numWallets]string
		var playerIDs [numWallets]string

		for i := 0; i < numWallets; i++ {
			walletIDs[i], playerIDs[i] = createTestWallet(t, "500.00")
		}

		var wg sync.WaitGroup
		var successCount int64

		startBarrier := make(chan struct{})

		for i := 0; i < numWallets; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-startBarrier

				extID := fmt.Sprintf("ext-multi-%d-%s", idx, uuid.NewString()[:6])
				payload := fmt.Sprintf(`{
					"providerId": "provider-a",
					"externalTransactionId": "%s",
					"playerId": "%s",
					"walletId": "%s",
					"roundId": "round-multi-1",
					"gameId": "game-1",
					"kind": "BET",
					"money": {"amount": "50.00", "currency": "BRL"}
				}`, extID, playerIDs[idx], walletIDs[idx])

				req, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", strings.NewReader(payload))
				req.Header.Set("Authorization", "Bearer "+tokenProviderA)
				req.Header.Set("Idempotency-Key", "provider-a:"+extID)
				req.Header.Set("Content-Type", "application/json")

				resp, err := httpClient.Do(req)
				if err != nil {
					t.Errorf("erro: %v", err)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode == http.StatusOK {
					atomic.AddInt64(&successCount, 1)
				}
			}(i)
		}

		close(startBarrier)
		wg.Wait()

		if successCount != numWallets {
			t.Errorf("esperava %d apostas com sucesso em carteiras distintas, obteve %d", numWallets, successCount)
		}
	})
}
