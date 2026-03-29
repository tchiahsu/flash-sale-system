package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

type ReserveRequest struct {
	EventID  string `json:"event_id"`
	Quantity int    `json:"quantity"`
}

var (
	useRedis    bool
	redisClient *redis.Client
	db          *sql.DB
)

var reserveScript = redis.NewScript(`
	local remaining = tonumber(redis.call('GET', KEYS[1]))
	if remaining == nil or remaining < tonumber(ARGV[1]) then
		return -1
	end
	return redis.call('DECRBY', KEYS[1], ARGV[1])
`)

func main() {
	const eventID = "event-001"
	const initialInventory = 100

	// Set up redis
	useRedis = os.Getenv("USE_REDIS") == "true"
	if useRedis {
		redisClient = redis.NewClient(&redis.Options{
			Addr: os.Getenv("REDIS_ADDR"),
		})
		if err := initRedis(eventID, initialInventory); err != nil {
			panic(err)
		}

		// Set up Postgres
	} else {
		var err error
		db, err = sql.Open("postgres", os.Getenv("POSTGRES_DSN"))
		if err != nil {
			panic(err)
		}
		if err := initPostgres(db, eventID, initialInventory); err != nil {
			panic(err)
		}
	}

	// Set up server
	router := gin.Default()
	router.POST("api/reserve", reserveTicket)
	router.GET("/health", getHealth)

	router.Run("0.0.0.0:8080")
}

// Reserves a ticket
func reserveTicket(c *gin.Context) {
	var request ReserveRequest

	if err := c.BindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if request.Quantity <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quantity must be greater than 0"})
		return
	}

	var success bool
	var remaining int
	var err error

	// Reserve a ticket using Redis or Postgres
	if useRedis {
		success, remaining, err = reserveRedis(request.EventID, request.Quantity)
	} else {
		success, remaining, err = reservePostgres(request.EventID, request.Quantity)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	httpStatus := http.StatusOK
	if !success {
		httpStatus = http.StatusConflict
	}

	c.JSON(httpStatus, gin.H{
		"success":   success,
		"remaining": remaining,
	})
}

// Reserves a ticket through Redis
func reserveRedis(eventID string, quantity int) (bool, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	key := "inventory:" + eventID
	remaining, err := redisClient.DecrBy(ctx, key, int64(quantity)).Result()
	if err != nil {
		return false, 0, nil
	}

	if remaining < 0 {
		redisClient.IncrBy(ctx, key, int64(quantity))
		return false, 0, nil
	}

	return true, int(remaining), nil
}

// Reserves a ticket through Postgres
func reservePostgres(eventID string, quantity int) (bool, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var remaining int
	err := db.QueryRowContext(ctx, `
		UPDATE inventory
		SET remaining = remaining - $1
		WHERE event_id = $2 AND remaining >= $1
		RETURNING remaining;
	`, quantity, eventID).Scan(&remaining)

	if err == sql.ErrNoRows {
		return false, 0, nil
	}

	if err != nil {
		return false, 0, err
	}

	return true, remaining, nil
}

// Sets up Redis
func initRedis(eventID string, initialInventory int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	key := "inventory:" + eventID
	return redisClient.Set(ctx, key, initialInventory, 0).Err()
}

// Sets up Postgres DB
func initPostgres(db *sql.DB, eventID string, initialInventory int) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS inventory (
			event_id TEXT PRIMARY KEY,
			remaining INT NOT NULL
		);
	`)

	if err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT INTO inventory (event_id, remaining)
		VALUES ($1, $2)
		ON CONFLICT (event_id)
			DO UPDATE SET remaining = EXCLUDED.remaining;
	`, eventID, initialInventory)
	return err
}

func getHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
	})
}
