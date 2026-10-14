package http

import (
	"context"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/jackc/pgx/v5/pgxpool"
)

type HealthHandler struct {
	pool      *pgxpool.Pool
	sqsClient *sqs.Client
	queueName string
}

func NewHealthHandler(pool *pgxpool.Pool, sqsClient *sqs.Client) *HealthHandler {
	return &HealthHandler{
		pool:      pool,
		sqsClient: sqsClient,
		queueName: "wager-transactions.fifo",
	}
}

func (h *HealthHandler) Live(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "UP",
	})
}

func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	pgStatus := "UP"
	if h.pool != nil {
		if err := h.pool.Ping(ctx); err != nil {
			pgStatus = "DOWN"
		}
	} else {
		pgStatus = "DOWN"
	}

	sqsStatus := "UP"
	if h.sqsClient != nil {
		_, err := h.sqsClient.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
			QueueName: aws.String(h.queueName),
		})
		if err != nil {
			sqsStatus = "DOWN"
		}
	} else {
		sqsStatus = "DOWN"
	}

	isReady := pgStatus == "UP" && sqsStatus == "UP"
	status := "UP"
	httpStatus := http.StatusOK

	if !isReady {
		status = "DOWN"
		httpStatus = http.StatusServiceUnavailable
	}

	writeJSON(w, httpStatus, map[string]string{
		"status":   status,
		"postgres": pgStatus,
		"sqs":      sqsStatus,
	})
}
