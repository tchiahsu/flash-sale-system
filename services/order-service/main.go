package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
)

type OrderRequest struct {
	EventID  string `json:"event_id"`
	UserID   string `json:"user_id"`
	Quantity int    `json:"quantity"`
}

type OrderResponse struct {
	OrderID  string `json:"order_id"`
	Status   string `json:"status"`
	EventID  string `json:"event_id"`
	Quantity int    `json:"quantity"`
	Reason   string `json:"reason,omitempty"`
}

type ReserveRequest struct {
	EventID  string `json:"event_id"`
	Quantity int    `json:"quantity"`
}

type ReserveResponse struct {
	Success   bool `json:"success"`
	Remaining int  `json:"remaining"`
}

type NotificationMessage struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	EventID   string `json:"event_id"`
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"`
}

var (
	inventoryServiceURL string
	db                  *sql.DB
	rabbitConn          *amqp.Connection
	rabbitChannel       *amqp.Channel
	httpClient          = &http.Client{Timeout: 5 * time.Second}
)

func init() {
	inventoryServiceURL = os.Getenv("INVENTORY_SERVICE_URL")
	if inventoryServiceURL == "" {
		inventoryServiceURL = "http://localhost:8080"
	}
}

func connectDB() {
	postgresURL := os.Getenv("POSTGRES_URL")
	var err error
	for i := range 10 {
		db, err = sql.Open("postgres", postgresURL)
		if err == nil {
			if err = db.Ping(); err == nil {
				break
			}
		}
		log.Printf("DB not ready, retrying in 3s (attempt %d/10)", i+1)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		panic("Failed to connect to database after 10 attempts: " + err.Error())
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS orders (
			id VARCHAR(50) PRIMARY KEY,
			user_id VARCHAR(50) NOT NULL,
			event_id VARCHAR(50) NOT NULL,
			quantity INT NOT NULL,
			status VARCHAR(20) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		panic("Failed to create orders table: " + err.Error())
	}
}

func connectRabbitMQ() {
	rabbitURL := os.Getenv("RABBITMQ_URL")
	var err error
	for i := range 10 {
		rabbitConn, err = amqp.Dial(rabbitURL)
		if err == nil {
			rabbitChannel, err = rabbitConn.Channel()
			if err == nil {
				break
			}
		}
		log.Printf("RabbitMQ not ready, retrying in 3s (attempt %d/10)", i+1)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		panic("Failed to connect to RabbitMQ after 10 attempts: " + err.Error())
	}

	_, err = rabbitChannel.QueueDeclare(
		"notification-queue",
		true, false, false, false, nil,
	)
	if err != nil {
		panic("Failed to declare queue: " + err.Error())
	}
}

func processOrder(c *gin.Context) {
	var request OrderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	orderID := uuid.New().String()

	reserveRequest := ReserveRequest{EventID: request.EventID, Quantity: request.Quantity}
	body, _ := json.Marshal(reserveRequest)

	response, err := httpClient.Post(
		inventoryServiceURL+"/api/reserve",
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "inventory service unavailable"})
		return
	}
	defer response.Body.Close()

	var reserveResponse ReserveResponse
	json.NewDecoder(response.Body).Decode(&reserveResponse)

	orderResp := OrderResponse{
		OrderID:  orderID,
		EventID:  request.EventID,
		Quantity: request.Quantity,
	}

	if !reserveResponse.Success {
		orderResp.Status = "failed"
		orderResp.Reason = "sold_out"
		c.JSON(http.StatusConflict, orderResp)
		return
	}

	// Only write to DB if reservation succeeded
	_, err = db.Exec(
		"INSERT INTO orders (id, user_id, event_id, quantity, status) VALUES ($1, $2, $3, $4, $5)",
		orderID, request.UserID, request.EventID, request.Quantity, "confirmed",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record order"})
		return
	}

	// Publish notification, log but don't fail the request if it errors
	notification := NotificationMessage{
		OrderID:   orderID,
		UserID:    request.UserID,
		EventID:   request.EventID,
		Status:    "confirmed",
		Timestamp: time.Now().UnixMilli(),
	}
	notificationBody, _ := json.Marshal(notification)
	err = rabbitChannel.Publish(
		"", "notification-queue", false, false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         notificationBody,
		},
	)
	if err != nil {
		log.Printf("WARNING: failed to publish notification for order %s: %s", orderID, err)
	}

	orderResp.Status = "confirmed"
	c.JSON(http.StatusOK, orderResp)
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

func main() {
	connectDB()
	connectRabbitMQ()
	defer db.Close()
	defer rabbitConn.Close()
	defer rabbitChannel.Close()

	app := gin.Default()
	app.POST("/api/process-order", processOrder)
	app.GET("/health", healthCheck)
	app.Run(":8080")
}
