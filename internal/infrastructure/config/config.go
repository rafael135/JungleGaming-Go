package config

import (
	"os"
	"strings"
)

type Config struct {
	Port              string
	Environment       string
	LogLevel          string
	DatabaseURL       string
	AWSRegion         string
	AWSEndpoint       string
	SQSQueueURL       string
	SQSEventsQueueURL string
	KeycloakIssuer    string
	KeycloakJWKSURL   string
}

func LoadConfig() *Config {
	port := getEnv("PORT", "8081")
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	return &Config{
		Port:              port,
		Environment:       getEnv("ENVIRONMENT", "development"),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://jungle:jungle_secret@localhost:5432/jungle_db?sslmode=disable"),
		AWSRegion:         getEnv("AWS_REGION", "us-east-1"),
		AWSEndpoint:       getEnv("AWS_ENDPOINT", "http://localhost:4566"),
		SQSQueueURL:       getEnv("SQS_QUEUE_URL", "http://localhost:4566/000000000000/wager-transactions.fifo"),
		SQSEventsQueueURL: getEnv("SQS_EVENTS_QUEUE_URL", "http://localhost:4566/000000000000/wager-events.fifo"),
		KeycloakIssuer:    getEnv("KEYCLOAK_ISSUER", "http://localhost:8080/realms/jungle"),
		KeycloakJWKSURL:   getEnv("KEYCLOAK_JWKS_URL", "http://localhost:8080/realms/jungle/protocol/openid-connect/certs"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
