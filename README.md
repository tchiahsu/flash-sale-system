# Flash Sale System

A distributed backend system that simulates high-demand flash sales (e.g., concert tickets) where thousands of users compete for limited inventory simultaneously. Built to study how architectural decisions affect correctness, performance, and resilience under extreme load.

## The Problem

Flash sales create a unique distributed systems challenge: thousands of concurrent requests hitting a limited resource within seconds. Without careful design, this leads to overselling, degraded performance, and poor user experience. This project explores how to prevent that.

## Architecture
```
┌──────────┐     ┌─────────────┐     ┌───────────────┐     ┌───────────────────┐
│  Locust  │────▶│ API Gateway │────▶│ Order Service │────▶│ Inventory Service │
│  (Load)  │     └─────────────┘     └───────┬───────┘     └─────────┬─────────┘
└──────────┘                                 │                       │
                                             │                ┌──────┴──────┐
                                             ▼                ▼             ▼
                                       ┌───────────┐     ┌────────┐   ┌────────┐
                                       │ RabbitMQ  │     │ Redis  │   │Postgres│
                                       └─────┬─────┘     └────────┘   └────────┘
                                             │
                                             ▼
                                    ┌─────────────────┐
                                    │  Notification   │
                                    │    Service      │
                                    └─────────────────┘
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

- **Setup:** 500 concurrent users, 2/4/8 Inventory Service instances
- **Metrics:** Throughput (RPS), p50/p95/p99 latency, correctness validation
- **Finding:** [No Results Yet]

### Experiment 2: Fault Tolerance During Peak Load
What happens when a service instance dies mid-sale? Does the system recover without data loss?

- **Setup:** Terminate Inventory Service task during peak Locust traffic
- **Metrics:** Request loss rate, recovery time, post-sale consistency verification
- **Finding:** [No Results Yet]

### Experiment 3: Redis vs. PostgreSQL for Inventory Management
What's the real performance/consistency tradeoff between fast caching and strong durability?

- **Setup:** Same workload against Redis-backed vs. Postgres-backed inventory
- **Metrics:** Latency, throughput, oversell rate, sync failure rate
- **Finding:** [No Results Yet]

### Experiment 4: RabbitMQ Resilience Under Load
Does the message queue survive broker failure? What happens when consumers can't keep up?

- **Setup:** Kill RabbitMQ mid-sale; separately, slow consumers to build queue depth
- **Metrics:** Message durability, queue depth stability, eventual processing correctness
- **Finding:** [No Results Yet]

## Tech Stack

| Component       | Technology                      |
|-----------------|---------------------------------|
| Services        | Go Lang / Gin                   |
| Message Broker  | AmazonMQ (RabbitMQ)             |
| Caching         | Redis                           |
| Database        | PostgreSQL                      |
| Infrastructure  | AWS ECS Fargate, ALB, Terraform |
| Load Testing    | Locust                          |
| Monitoring      | CloudWatch                      |

## Project Structure
```
├── load-testing/
│   └── locust/
├── screenshots/
│   └── experiment-screenshots
├── scripts/
│   └── deploy.sh
├── services/
│   ├── api-gateway/
│   ├── order-service/
│   ├── inventory-service/
│   └── notification-service/
├── terraform/
│   └── terraform-files
```

## Deploying to AWS

### Prerequisites
- AWS CLI installed
- Terraform installed
- Access to AWS Sandbox

### Steps

1. **Configure AWS credentials**
   
  Open your AWS Sandbox and retrieve your credentials. Then run:
```bash
   aws configure
```
   Enter your AWS Access Key ID, Secret Access Key, and set region to `us-east-1`.

2. **Initialize and apply Terraform**
```bash
   cd terraform
```
   
   Create a `terraform.tfvars` file with your configuration, then run:
```bash
   terraform init
   terraform apply --auto-approve
```

3. **Update configuration with Terraform outputs**
   
   After Terraform completes, copy the service URL from the output and update:
   - `terraform/terraform.tfvars` with the service URL for all services
   - `scripts/deploy.sh` with your AWS account ID

4. **Run the deploy script**
```bash
   cd ../..  # Return to project root
   chmod +x scripts/deploy.sh
   ./scripts/deploy.sh
```
   During execution, you will be prompted 4 times with a `:` — press `q` each time to continue.

5. **Verify deployment**
   
   The system is now deployed. Use the ALB URL from Terraform output to run load tests.
