package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
)

func init() {
	inventoryServiceURL = os.Getenv("INVENTORY_SERVICE_URL")
	if inventoryServiceURL == "" {
		inventoryServiceURL = "http://localhost:8082"
	}
}

func connectDB() {
	postgresURL := os.Getenv("POSTGRES_URL")
	if postgresURL == "" {
		postgresURL = "postgres URL"
	}

	var err error
	db, err = sql.Open("postgres", postgresURL)
	if err != nil {
		panic("Failed to connect to database: " + err.Error())
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
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@localhost:5672/"
	}

	var err error
	rabbitConn, err = amqp.Dial(rabbitURL)
	if err != nil {
		panic("Failed to connect to RabbitMQ: " + err.Error())
	}

	rabbitChannel, err = rabbitConn.Channel()
	if err != nil {
		panic("Failed to open RabbitMQ channel: " + err.Error())
	}

	_, err = rabbitChannel.QueueDeclare(
		"notification-queue",
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // arguments
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

	// Call Inventory Service
	reserveRequest := ReserveRequest{
		EventID:  request.EventID,
		Quantity: request.Quantity,
	}
	body, _ := json.Marshal(reserveRequest)

	response, err := http.Post(
		inventoryServiceURL+"/api/reserve",
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "inventory service unavailable",
		})
		return
	}
	defer response.Body.Close()

	var reserveResponse ReserveResponse
	json.NewDecoder(response.Body).Decode(&reserveResponse)

	// Determine Order Status
	var status string
	var reason string
	if reserveResponse.Success {
		status = "confirmed"
	} else {
		status = "failed"
		reason = "sold_out"
	}

	// Record Order In Postgres
	_, err = db.Exec(
		"INSERT INTO orders (id, user_id, event_id, quantity, status) VALUES ($1, $2, $3, $4, $5)",
		orderID, request.UserID, request.EventID, request.Quantity, status,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to record order",
		})
		return
	}

	// Publish to RabbitMQ
	notification := NotificationMessage{
		OrderID:   orderID,
		UserID:    request.UserID,
		EventID:   request.EventID,
		Status:    status,
		Timestamp: time.Now().UnixMilli(),
	}
	notificationBody, _ := json.Marshal(notification)

	rabbitChannel.Publish(
		"",
		"notification-queue",
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         notificationBody,
		},
	)

	// Return Response
	orderResp := OrderResponse{
		OrderID:  orderID,
		Status:   status,
		EventID:  request.EventID,
		Quantity: request.Quantity,
		Reason:   reason,
	}

	if status == "confirmed" {
		c.JSON(http.StatusOK, orderResp)
	} else {
		c.JSON(http.StatusConflict, orderResp)
	}
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

	app.Run(":8081")
}
