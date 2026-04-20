# Flash Sale System
 
A distributed flash sale platform built on AWS to evaluate how architectural decisions affect correctness and performance under extreme concurrency. The system simulates a high-traffic ticket sale where hundreds to thousands of users attempt to purchase a limited number of tickets simultaneously, and tests whether the system can prevent overselling, eliminate duplicate orders, and maintain full data consistency under load and partial failure.
 
---
 
## Why We Built This
 
Flash sale systems are one of the hardest problems in distributed systems. The correctness requirements are strict, oversell even one ticket and you have a real-world dispute. The performance requirements are equally strict, time out under load and users give up. Most systems get one right but not both.
 
We wanted to build a system that takes both seriously, and then deliberately stress-test it to find exactly where and how it breaks. Every experiment in this project is designed to answer a specific question about a specific architectural decision: Does horizontal scaling help if you scale the wrong service? Does the system stay correct when a critical component fails mid-sale? Does Redis actually outperform Postgres for inventory reservation, and does that advantage hold for correctness as well as speed?
 
---
 
## System Architecture
 
The system is built as four microservices deployed on AWS ECS Fargate, sitting behind an Application Load Balancer.
 
```
                        Internet / User
                               │
                               ▼
                   Application Load Balancer
                               │
                               ▼
                    API Gateway Service
                           2 tasks
                               │
                               ▼
                          Order Service
                            8 tasks
              ┌────────────────┼────────────────┐
              │                │                │ async
              ▼                ▼                ▼
      Inventory Service    Order RDS        RabbitMQ
      2 tasks                                   │ async
         │                                      ▼
         ├──────────────┐            Notification Service
         ▼              ▼          
      Redis          Inventory RDS
```
 
### Purchase Flow
 
1. A user sends a purchase request to the API Gateway, which forwards it to the Order Service
2. The Order Service makes a **synchronous** HTTP call to the Inventory Service to reserve a ticket, it waits for a response before proceeding
3. The Inventory Service performs an atomic decrement on the ticket count (via Redis Lua script or PostgreSQL atomic UPDATE) and returns success or failure
4. If the reservation succeeds, the Order Service writes a confirmed order to PostgreSQL and returns a confirmation to the user
5. The Order Service publishes a message to RabbitMQ, the Notification Service picks this up **asynchronously** and records a notification
The critical path is synchronous by design. If the Inventory Service is unreachable, the Order fails cleanly with no partial state written. Notifications are asynchronous because they do not affect purchase correctness and should not block the user.
 
### Inventory Backends
 
The Inventory Service supports two backends, switchable via the `INVENTORY_BACKEND` environment variable:
 
| Backend | How it works |
|---|---|
| `postgres` | Atomic `UPDATE inventory SET remaining = remaining - 1 WHERE remaining >= 1` — one database, one operation, one source of truth |
| `redis_postgres` | Atomic decrements Redis in-memory counter; background goroutine syncs remaining count back to PostgreSQL asynchronously |
 
---
 
## Infrastructure
 
All infrastructure is defined in Terraform and deployed to AWS.
 
| Component | Service | Spec |
|---|---|---|
| API Gateway | ECS Fargate | 256 CPU, 512MB, 2 tasks |
| Order Service | ECS Fargate | 512 CPU, 1024MB, 8 tasks |
| Inventory Service | ECS Fargate | 256 CPU, 512MB, 2 tasks |
| Notification Service | ECS Fargate | 256 CPU, 512MB, 1 task |
| Orders DB | RDS PostgreSQL 15.7 | db.t3.micro |
| Inventory DB | RDS PostgreSQL 15.7 | db.t3.micro |
| Redis | ElastiCache Redis 7.1 | cache.t3.micro, 1 node |
| Message Broker | Amazon MQ RabbitMQ 3.13 | mq.m7g.medium |
| Load Balancer | Application Load Balancer | 15s health check interval |
 
---
 
## Repository Structure
 
```
flash-sale-system/
├── services/
│   ├── api-gateway/          # Go — request forwarding and reset coordination
│   ├── order-service/        # Go — purchase coordination, DB writes, RabbitMQ publishing
│   ├── inventory-service/    # Go — atomic ticket reservation (postgres or redis_postgres)
│   └── notification-service/ # Go — RabbitMQ consumer, notification recording
├── terraform/                # AWS infrastructure — VPC, ECS, RDS, ElastiCache, MQ, ALB
├── load-testing/             # Locust files for all four experiments
├── scripts/                  # Consistency verification script, deploy script
└── graphs/                   # CloudWatch and Locust result graphs per experiment
```
 
---
 
## Deployment
 
### Prerequisites
 
- AWS CLI configured with appropriate credentials
- Terraform >= 1.0
- Docker
- `jq`
- Git
### 1. Clone the repository
 
```bash
git clone <repo-url>
cd flash-sale-system
```
 
### 2. Configure Terraform variables
 
Create a `terraform.tfvars` file in the `terraform/` directory:
 
```hcl
db_password           = "yourpassword"
rabbitmq_password     = "yourpassword"
inventory_backend     = "postgres"    # or "redis_postgres"
```
 
### 3. Provision infrastructure
 
```bash
cd terraform
terraform init
terraform apply
```
 
Take note of the outputs — you will need `alb_dns_name` for load testing and the RDS endpoints for the consistency check script.
 
### 4. Build and deploy services
 
```bash
cd ..
./scripts/deploy.sh
```
 
This script builds Docker images for all four services, pushes them to ECR, registers new ECS task definitions, and forces new deployments.
 
### 5. Verify deployment
 
Check that all services are healthy:
 
```bash
curl http://<alb_dns_name>/health
```
 
Each service should return `{"status":"healthy"}`.
 
### 6. Reset state before each test run
 
```bash
curl -X POST http://<alb_dns_name>/api/reset
```
 
This resets inventory to the initial count, clears the orders table, and purges the notification queue.
 
### Switching Inventory Backends
 
To switch between `postgres` and `redis_postgres` backends, update `terraform.tfvars` and force a new task definition manually via the AWS CLI (the deploy script may overwrite the environment variable):
 
```bash
aws ecs register-task-definition \
  --cli-input-json "$(aws ecs describe-task-definition \
    --task-definition flash-sale-system-inventory-service \
    --query taskDefinition | \
    jq '.containerDefinitions[0].environment |= map(if .name == "INVENTORY_BACKEND" then .value = "redis_postgres" else . end) | del(.taskDefinitionArn, .revision, .status, .requiresAttributes, .compatibilities, .registeredAt, .registeredBy)')" \
  --region us-east-1
 
aws ecs update-service \
  --cluster flash-sale-system-cluster \
  --service flash-sale-system-inventory-service \
  --task-definition flash-sale-system-inventory-service:<new_revision> \
  --force-new-deployment \
  --region us-east-1
```
 
---
 
## Consistency Verification
 
After each experiment run, execute the consistency check script to validate correctness:
 
```bash
cd scripts
python consistency_check.py
```
 
The script queries the databases directly and checks five properties:
 
| Check | Description |
|---|---|
| 1 | No overselling — confirmed orders do not exceed initial inventory |
| 2 | No duplicate order IDs |
| 3 | All confirmed orders exist in the orders database |
| 4 | Confirmed orders + remaining inventory = initial inventory |
| 5 | Every confirmed order has a corresponding notification record |
 
Update the database connection strings in the script to match your Terraform outputs before running.

---

## Project Progression
 
The project started from a high-level proposal describing four services and three experiments. Several significant decisions were made as implementation began.
 
**Service communication model.** The initial proposal left inter-service communication unspecified. After working through the purchase flow, we settled on a hybrid synchronous/asynchronous design. The critical path is synchronous because users need an immediate answer. Notifications are asynchronous through RabbitMQ because they do not affect purchase correctness and should not block the user.
 
**Role clarification between Order and Inventory.** Early on it was unclear which service should orchestrate the purchase flow. We determined that the Order Service should act as the coordinator, it calls Inventory to reserve, writes the order record, and publishes to RabbitMQ. The Inventory Service is solely responsible for protecting the ticket count. This separation allowed us to independently scale and swap each service during experiments.
 
**Experiment 1 redesign.** The original Experiment 1 targeted the Inventory Service for horizontal scaling. After analysis we believed the Order Service would be the actual bottleneck. Experiment 1 was revised to scale the Order Service. Results showed the bottleneck was downstream, directly motivating Experiment 3.
 
**Experiment 3 redesign.** The original Experiment 3 compared Redis-only against Postgres-only. A mock interview flagged that Redis without persistence is not a realistic production design, a node restart would lose all inventory state. The experiment was redesigned to compare a Redis+Postgres hybrid against Postgres-only, which is both more realistic and more meaningful as an architectural comparison.
 
**Addition of Experiment 4.** The original proposal had three experiments. A fourth was added to test RabbitMQ behavior under stress. Most projects treat the message queue as infrastructure that just works, we wanted to explicitly test what happens when it does not.
 
---
 
## Problems Encountered
 
**RDS subnet configuration.** Making RDS publicly accessible for the consistency check script required both setting `publicly_accessible = true` and moving the subnet group to public subnets. Terraform cannot modify the subnet group on a running RDS instance, which required destroying and recreating both instances.
 
**RabbitMQ reconnection.** Both the Order Service and Notification Service initially had no reconnection logic. If the broker went down, services would silently fail without recovering. This was discovered during Experiment 4 testing and fixed by implementing retry loops in both services before running the experiment.
 
**Flawed consistency check.** The original consistency check script sent 200 fresh purchase requests as part of its own execution, meaning running it after Locust would always produce 200 failures since inventory was already depleted. The script was rewritten to query the databases directly without generating any load.
 
**Redis backend not switching.** When switching between `postgres` and `redis_postgres` for Experiment 3, the `deploy.sh` script was reading the current task definition and re-registering it with the existing environment variables, overwriting the change that Terraform had just applied. The fix required manually registering a new task definition via the AWS CLI to force the correct `INVENTORY_BACKEND` value.
