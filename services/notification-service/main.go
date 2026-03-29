package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	amqp "github.com/rabbitmq/amqp091-go"
)

type NotificationMessage struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	EventID   string `json:"event_id"`
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"`
}

var (
	db            *sql.DB
	rabbitConn    *amqp.Connection
	rabbitChannel *amqp.Channel
)

func connectDB() {
	postgresURL := os.Getenv("POSTGRES_URL")
	if postgresURL == "" {
		postgresURL = "postgres://user:password@local"
	}

	var err error
	db, err = sql.Open("postgres", postgresURL)
	if err != nil {
		panic("Failed to connect to database: " + err.Error())
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS notifications (
			id SERIAL PRIMARY KEY,
			order_id VARCHAR(50) NOT NULL,
			user_id VARCHAR(50) NOT NULL,
			event_id VARCHAR(50) NOT NULL,
			status VARCHAR(20) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		)
	`)
	if err != nil {
		panic("Failed to create notifications table: " + err.Error())
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

func consumeNotifications() {
	msgs, err := rabbitChannel.Consume(
		"notification-queue",
		"",    // consumer tag
		false, // auto-ack (false = manual ack)
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		panic("Failed to start consuming: " + err.Error())
	}

	log.Println("Notification Service: Waiting for messages...")

	for msg := range msgs {
		var notification NotificationMessage
		err := json.Unmarshal(msg.Body, &notification)
		if err != nil {
			log.Printf("Failed to parse message: %s", err)
			msg.Nack(false, false) // reject, don't requeue
			continue
		}

		// Record notification in Postgres
		_, err = db.Exec(
			"INSERT INTO notifications (order_id, user_id, event_id, status) VALUES ($1, $2, $3, $4)",
			notification.OrderID, notification.UserID, notification.EventID, notification.Status,
		)
		if err != nil {
			log.Printf("Failed to record notification: %s", err)
			msg.Nack(false, true) // reject, requeue
			continue
		}

		log.Printf("Notification sent: Order %s for User %s - %s",
			notification.OrderID, notification.UserID, notification.Status)

		msg.Ack(false) // acknowledge
	}
}

func healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{"status": "healthy"})
}

func main() {
	connectDB()
	connectRabbitMQ()
	defer db.Close()
	defer rabbitConn.Close()
	defer rabbitChannel.Close()

	//S Start consuming in background
	go consumeNotifications()

	// Health Check Endpoint
	app := gin.Default()
	app.GET("/health", healthCheck)

	app.Run(":8082")
}
