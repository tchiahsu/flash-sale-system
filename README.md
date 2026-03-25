# Flash Sale System

A distributed backend system that simulates high-demand flash sales (e.g., concert tickets) where thousands of users compete for limited inventory simultaneously. Built to study how architectural decisions affect correctness, performance, and resilience under extreme load.

## The Problem

Flash sales create a unique distributed systems challenge: thousands of concurrent requests hitting a limited resource within seconds. Without careful design, this leads to overselling, degraded performance, and poor user experience. This project explores how to prevent that.

## Architecture
```
┌──────────┐     ┌─────────────┐     ┌─────────────────┐
│  Locust  │────▶│ API Gateway │────▶│    RabbitMQ     │
│  (Load)  │     └─────────────┘     └────────┬────────┘
└──────────┘                                  │
                              ┌───────────────┼───────────────┐
                              ▼               ▼               ▼
                       ┌───────────┐   ┌───────────┐   ┌─────────────┐
                       │  Order    │   │ Inventory │   │ Notification│
                       │  Service  │   │  Service  │   │   Service   │
                       └───────────┘   └─────┬─────┘   └─────────────┘
                                             │
                                      ┌──────┴──────┐
                                      ▼             ▼
                                 ┌────────┐    ┌────────┐
                                 │ Redis  │    │Postgres│
                                 └────────┘    └────────┘
```

**Key design decisions:**
- **Asynchronous order processing** via RabbitMQ to decouple services and handle backpressure
- **Dual storage strategy** with Redis for fast inventory checks and PostgreSQL for durable order records
- **Horizontal scaling** with ECS Fargate and Application Load Balancer
- **Infrastructure as Code** using Terraform for reproducible deployments

## Experiments

This project includes four experiments that isolate specific distributed systems variables:

### Experiment 1: Horizontal Scaling Under Burst Traffic
How does adding instances affect throughput and latency? Where are the diminishing returns?

- **Setup:** 500 concurrent users, 1/2/4/8 Inventory Service instances
- **Metrics:** Throughput (RPS), p50/p95/p99 latency, correctness validation
- **Finding:** [To be completed]

### Experiment 2: Fault Tolerance During Peak Load
What happens when a service instance dies mid-sale? Does the system recover without data loss?

- **Setup:** Terminate Inventory Service task during peak Locust traffic
- **Metrics:** Request loss rate, recovery time, post-sale consistency verification
- **Finding:** [To be completed]

### Experiment 3: Redis vs. PostgreSQL for Inventory Management
What's the real performance/consistency tradeoff between fast caching and strong durability?

- **Setup:** Same workload against Redis-backed vs. Postgres-backed inventory
- **Metrics:** Latency, throughput, oversell rate, sync failure rate
- **Finding:** [To be completed]

### Experiment 4: RabbitMQ Resilience Under Load
Does the message queue survive broker failure? What happens when consumers can't keep up?

- **Setup:** Kill RabbitMQ mid-sale; separately, slow consumers to build queue depth
- **Metrics:** Message durability, queue depth stability, eventual processing correctness
- **Finding:** [To be completed]

## Consistency Verification

After every experiment, an automated verification script checks:
- No overselling (confirmed orders ≤ initial inventory)
- No duplicate order IDs
- Every confirmed order has a corresponding notification
- Confirmed orders + remaining inventory = initial inventory

## Tech Stack

| Component | Technology |
|-----------|------------|
| Services | Go Lang / Gin |
| Message Broker | RabbitMQ |
| Caching | Redis |
| Database | PostgreSQL |
| Infrastructure | AWS ECS Fargate, ALB, Terraform |
| Load Testing | Locust |
| Monitoring | CloudWatch |

## Project Structure
```
├── services/
│   ├── api-gateway/
│   ├── order-service/
│   ├── inventory-service/
│   └── notification-service/
├── infrastructure/
│   └── terraform/
├── load-tests/
│   └── locust/
├── scripts/
│   └── consistency-check.py
└── docs/
    └── experiment-results/
```

## Running Locally
```bash
TBD
```

## Deploying to AWS
```bash
TBD
```

## Results & Analysis

[Link to detailed experiment results and analysis]
