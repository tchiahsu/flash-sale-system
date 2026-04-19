package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
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
	db        *sql.DB
	rabbitURL string
)

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
		CREATE TABLE IF NOT EXISTS notifications (
			id SERIAL PRIMARY KEY,
			order_id VARCHAR(50) NOT NULL,
			user_id VARCHAR(50) NOT NULL,
			event_id VARCHAR(50) NOT NULL,
			status VARCHAR(20) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		panic("Failed to create notifications table: " + err.Error())
	}
}

func connectRabbitMQ() (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return nil, nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, err
	}

	_, err = ch.QueueDeclare(
		"notification-queue",
		true, false, false, false, nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, nil, err
	}

	return conn, ch, nil
}

// consumeLoop keeps trying to connect and consume forever.
// If the broker goes down, it waits and reconnects automatically.
func consumeLoop() {
	for {
		log.Println("Notification service: connecting to RabbitMQ...")

		conn, ch, err := connectRabbitMQ()
		if err != nil {
			log.Printf("Failed to connect to RabbitMQ: %s — retrying in 5s", err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Println("Notification service: connected, waiting for messages...")

		msgs, err := ch.Consume(
			"notification-queue",
			"", false, false, false, false, nil,
		)
		if err != nil {
			log.Printf("Failed to start consuming: %s — retrying in 5s", err)
			ch.Close()
			conn.Close()
			time.Sleep(5 * time.Second)
			continue
		}

		// process messages until the channel closes (broker down)
		for msg := range msgs {
			var notification NotificationMessage
			if err := json.Unmarshal(msg.Body, &notification); err != nil {
				log.Printf("Failed to parse message: %s", err)
				msg.Nack(false, false)
				continue
			}

			_, err = db.Exec(
				"INSERT INTO notifications (order_id, user_id, event_id, status) VALUES ($1, $2, $3, $4)",
				notification.OrderID, notification.UserID, notification.EventID, notification.Status,
			)
			if err != nil {
				log.Printf("Failed to record notification: %s", err)
				msg.Nack(false, true)
				continue
			}

			log.Printf("Notification recorded: order %s for user %s — %s",
				notification.OrderID, notification.UserID, notification.Status)
			msg.Ack(false)
		}

		// if we get here, the msgs channel closed — broker went down
		log.Println("RabbitMQ connection lost — reconnecting in 5s...")
		ch.Close()
		conn.Close()
		time.Sleep(5 * time.Second)
	}
}

func resetNotifications(c *gin.Context) {
	_, err := db.Exec(`DELETE FROM notifications`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset notifications table"})
		return
	}

	// reconnect temporarily just to purge the queue
	conn, ch, err := connectRabbitMQ()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to RabbitMQ for purge"})
		return
	}
	defer ch.Close()
	defer conn.Close()

	_, err = ch.QueuePurge("notification-queue", false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to purge notification queue"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

func main() {
	rabbitURL = os.Getenv("RABBITMQ_URL")
	connectDB()
	defer db.Close()

	go consumeLoop()

	app := gin.Default()
	app.POST("/api/reset", resetNotifications)
	app.GET("/health", healthCheck)
	app.Run(":8080")
}
