resource "aws_ecs_cluster" "main" {
  name = "${var.project_name}-cluster"

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = { Name = "${var.project_name}-cluster" }
}

# ─── CloudWatch log groups ───────────────────────────────────────────────────

locals {
  log_groups = [
    "api-gateway",
    "order-service",
    "inventory-service",
    "notification-service",
  ]
}

resource "aws_cloudwatch_log_group" "services" {
  for_each          = toset(local.log_groups)
  name              = "/ecs/${var.project_name}/${each.key}"
  retention_in_days = 7

  tags = { Name = "${var.project_name}-${each.key}-logs" }
}

# ─── Shared env helpers ──────────────────────────────────────────────────────

locals {
  rabbitmq_url = "amqps://${var.rabbitmq_username}:${var.rabbitmq_password}@${trimprefix(aws_mq_broker.rabbitmq.instances[0].endpoints[0], "amqps://")}"
}

# ─── API Gateway ─────────────────────────────────────────────────────────────

resource "aws_ecs_task_definition" "api_gateway" {
  family                   = "${var.project_name}-api-gateway"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn

  container_definitions = jsonencode([{
    name  = "api-gateway"
    image = var.api_gateway_image

    portMappings = [{ containerPort = 8080, protocol = "tcp" }]

    environment = [
      { name = "ORDER_SERVICE_URL", value = "http://order-service.${var.project_name}.local:8080" },
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.services["api-gateway"].name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "ecs"
      }
    }
  }])
}

resource "aws_ecs_service" "api_gateway" {
  name            = "${var.project_name}-api-gateway"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.api_gateway.arn
  desired_count   = 2
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = [aws_subnet.private_1.id, aws_subnet.private_2.id]
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.api_gateway.arn
    container_name   = "api-gateway"
    container_port   = 8080
  }

  depends_on = [aws_lb_listener.http]
}

# ─── Order Service ───────────────────────────────────────────────────────────

resource "aws_ecs_task_definition" "order_service" {
  family                   = "${var.project_name}-order-service"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "512"
  memory                   = "1024"
  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn

  container_definitions = jsonencode([{
    name  = "order-service"
    image = var.order_service_image

    portMappings = [{ containerPort = 8080, protocol = "tcp" }]

    environment = [
      { name = "POSTGRES_URL", value = "postgresql://${var.db_username}:${var.db_password}@${aws_db_instance.orders.address}:5432/orders" },
      { name = "INVENTORY_SERVICE_URL", value = "http://inventory-service.${var.project_name}.local:8080" },
      { name = "RABBITMQ_URL", value = local.rabbitmq_url },
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.services["order-service"].name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "ecs"
      }
    }
  }])
}

resource "aws_ecs_service" "order_service" {
  name            = "${var.project_name}-order-service"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.order_service.arn
  desired_count   = 2
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = [aws_subnet.private_1.id, aws_subnet.private_2.id]
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false
  }

  service_registries {
  registry_arn = aws_service_discovery_service.order_service.arn
}
}

# ─── Inventory Service ───────────────────────────────────────────────────────

resource "aws_ecs_task_definition" "inventory_service" {
  family                   = "${var.project_name}-inventory-service"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn

  container_definitions = jsonencode([{
    name  = "inventory-service"
    image = var.inventory_service_image

    portMappings = [{ containerPort = 8080, protocol = "tcp" }]

    # INVENTORY_BACKEND controls Experiment 3: set to "redis" or "postgres"
    environment = [
      { name = "INVENTORY_BACKEND", value = var.inventory_backend },
      { name = "REDIS_URL", value = "redis://${aws_elasticache_cluster.inventory.cache_nodes[0].address}:6379" },
      { name = "POSTGRES_URL", value = "postgresql://${var.db_username}:${var.db_password}@${aws_db_instance.inventory.address}:5432/inventory" },
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.services["inventory-service"].name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "ecs"
      }
    }
  }])
}

resource "aws_ecs_service" "inventory_service" {
  name            = "${var.project_name}-inventory-service"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.inventory_service.arn
  desired_count   = 2
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = [aws_subnet.private_1.id, aws_subnet.private_2.id]
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false
  }

  service_registries {
  registry_arn = aws_service_discovery_service.inventory_service.arn
}
}

# ─── Notification Service ────────────────────────────────────────────────────

resource "aws_ecs_task_definition" "notification_service" {
  family                   = "${var.project_name}-notification-service"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn

  container_definitions = jsonencode([{
    name  = "notification-service"
    image = var.notification_service_image

    # No port mapping — this service only consumes from RabbitMQ
    environment = [
      { name = "RABBITMQ_URL", value = local.rabbitmq_url },
      { name = "POSTGRES_URL", value = "postgresql://${var.db_username}:${var.db_password}@${aws_db_instance.orders.address}:5432/orders" },
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.services["notification-service"].name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "ecs"
      }
    }
  }])
}

resource "aws_ecs_service" "notification_service" {
  name            = "${var.project_name}-notification-service"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.notification_service.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = [aws_subnet.private_1.id, aws_subnet.private_2.id]
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false
  }
}
