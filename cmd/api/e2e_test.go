package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"jungle/internal/application"

	"github.com/google/uuid"
	"go.uber.org/fx/fxtest"
)

type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

func fetchToken(t *testing.T, clientID, clientSecret string) string {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)

	issuer := os.Getenv("KEYCLOAK_ISSUER")
	if issuer == "" {
		issuer = "http://localhost:8080/realms/jungle"
	}

	resp, err := http.Post(
		issuer+"/protocol/openid-connect/token",
		"application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		t.Fatalf("erro ao chamar Keycloak para cliente %s: %v", clientID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("falha ao obter token do Keycloak (%d): %s", resp.StatusCode, string(b))
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		t.Fatalf("erro ao decodificar token do Keycloak: %v", err)
	}
	return tr.AccessToken
}

func TestEndToEndKeycloakAndAPI(t *testing.T) {
	// 1. Obter tokens reais do Keycloak
	tokenInternal := fetchToken(t, "internal-service", "internal-service-secret")
	tokenProviderA := fetchToken(t, "provider-a", "provider-a-secret")
	tokenProviderB := fetchToken(t, "provider-b", "provider-b-secret")

	// 2. Iniciar aplicação Uber Fx completa
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
	httpClient := &http.Client{Timeout: 5 * time.Second}

	// 3. Teste: Acesso sem token em endpoint protegido retorna 401
	t.Run("Endpoint protegido sem token retorna 401", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/wallets", bytes.NewReader([]byte(`{}`)))
		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("erro na requisicao: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("esperava 401, recebeu %d", resp.StatusCode)
		}
	})

	// 4. Teste: Provedor tenta criar carteira (endpoint restrito a internal) retorna 403
	t.Run("Provedor criando carteira retorna 403 Forbidden", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/wallets", bytes.NewReader([]byte(`{
			"playerId": "player-unauth",
			"initialBalance": {"amount": "100.00", "currency": "BRL"}
		}`)))
		req.Header.Set("Authorization", "Bearer "+tokenProviderA)
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("erro na requisicao: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("esperava 403 Forbidden, recebeu %d", resp.StatusCode)
		}
	})

	// 5. Teste: Internal service cria carteira com 1000.00 BRL
	playerID := uuid.NewString()
	var walletID string

	t.Run("Internal service cria carteira com sucesso", func(t *testing.T) {
		body := fmt.Sprintf(`{
			"playerId": "%s",
			"initialBalance": {"amount": "1000.00", "currency": "BRL"}
		}`, playerID)

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
			t.Fatalf("esperava 201 Created, recebeu %d: %s", resp.StatusCode, string(b))
		}

		var created application.CreateWalletOutput
		if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
			t.Fatalf("erro ao ler resposta da criacao de carteira: %v", err)
		}
		walletID = created.ID
		if created.Balance.Amount != "1000.00" {
			t.Errorf("esperava saldo 1000.00, recebeu %s", created.Balance.Amount)
		}
	})

	// 6. Teste: Provider-A envia aposta (BET) de 100.00 BRL com sucesso
	extTxID := "ext-e2e-" + uuid.NewString()[:8]
	idempKey := "provider-a:" + extTxID

	t.Run("Provider-A envia aposta de 100.00 BRL", func(t *testing.T) {
		payload := fmt.Sprintf(`{
			"providerId": "provider-a",
			"externalTransactionId": "%s",
			"playerId": "%s",
			"walletId": "%s",
			"roundId": "round-e2e-1",
			"gameId": "game-safari",
			"kind": "BET",
			"money": {"amount": "100.00", "currency": "BRL"}
		}`, extTxID, playerID, walletID)

		req, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+tokenProviderA)
		req.Header.Set("Idempotency-Key", idempKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("erro na requisicao de aposta: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("esperava 200 OK na aposta, recebeu %d: %s", resp.StatusCode, string(b))
		}

		var out application.ProcessWagerOutput
		_ = json.NewDecoder(resp.Body).Decode(&out)
		if out.Status != "PROCESSED" {
			t.Errorf("esperava PROCESSED, recebeu %s", out.Status)
		}
		if out.Balance.Amount != "900.00" {
			t.Errorf("esperava saldo 900.00, recebeu %s", out.Balance.Amount)
		}
		if out.IdempotentReplay {
			t.Errorf("primeira execucao nao deveria ser replay")
		}
	})

	// 7. Teste: Replay idempotente com mesma Idempotency-Key devolve resultado original
	t.Run("Replay idempotente devolve resultado persistido e saldo observado", func(t *testing.T) {
		payload := fmt.Sprintf(`{
			"providerId": "provider-a",
			"externalTransactionId": "%s",
			"playerId": "%s",
			"walletId": "%s",
			"roundId": "round-e2e-1",
			"gameId": "game-safari",
			"kind": "BET",
			"money": {"amount": "100.00", "currency": "BRL"}
		}`, extTxID, playerID, walletID)

		req, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+tokenProviderA)
		req.Header.Set("Idempotency-Key", idempKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("erro no replay: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("esperava 200 OK no replay, recebeu %d: %s", resp.StatusCode, string(b))
		}

		var out application.ProcessWagerOutput
		_ = json.NewDecoder(resp.Body).Decode(&out)
		if !out.IdempotentReplay {
			t.Errorf("esperava idempotentReplay = true")
		}
		if out.Balance.Amount != "900.00" {
			t.Errorf("esperava saldo original 900.00, recebeu %s", out.Balance.Amount)
		}
	})

	// 8. Teste: Isolamento de provedor - Provider-B não pode consultar transação do Provider-A
	t.Run("Isolamento entre provedores: Provider-B nao consulta transacao do Provider-A", func(t *testing.T) {
		req, _ := http.NewRequest(
			http.MethodGet,
			fmt.Sprintf("%s/providers/provider-a/wagering/transactions/%s", baseURL, extTxID),
			nil,
		)
		req.Header.Set("Authorization", "Bearer "+tokenProviderB)

		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("erro ao consultar transacao: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("esperava 403 Forbidden para Provider-B tentando ver Provider-A, recebeu %d", resp.StatusCode)
		}
	})

	// 9. Teste: Reconciliação confirma consistência (abertura 1000.00 - aposta 100.00 = 900.00)
	t.Run("Reconciliacao confirma consistencia matematica exata", func(t *testing.T) {
		req, _ := http.NewRequest(
			http.MethodPost,
			fmt.Sprintf("%s/wallets/%s/reconciliation", baseURL, walletID),
			nil,
		)
		req.Header.Set("Authorization", "Bearer "+tokenInternal)

		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("erro na reconciliacao: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("esperava 200 OK na reconciliacao, recebeu %d: %s", resp.StatusCode, string(b))
		}

		var out application.ReconcileWalletOutput
		_ = json.NewDecoder(resp.Body).Decode(&out)
		if !out.Consistent {
			t.Errorf("esperava carteira consistente, diferenca=%s", out.Difference.Amount)
		}
		if out.StoredBalance.Amount != "900.00" {
			t.Errorf("esperava storedBalance 900.00, recebeu %s", out.StoredBalance.Amount)
		}
		if out.CalculatedBalance.Amount != "900.00" {
			t.Errorf("esperava calculatedBalance 900.00, recebeu %s", out.CalculatedBalance.Amount)
		}
		if out.CheckedEntries != 2 {
			t.Errorf("esperava 2 lancamentos checados (opening + bet), recebeu %d", out.CheckedEntries)
		}
	})
}
