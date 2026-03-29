resource "aws_mq_broker" "rabbitmq" {
  broker_name = "${var.project_name}-rabbitmq"

  engine_type        = "RabbitMQ"
  engine_version     = "3.13"
  host_instance_type = "mq.t3.micro"

  # Single-instance broker is fine for a load-test project.
  # Swap to CLUSTER_MULTI_AZ for production.
  auto_minor_version_upgrade = true
  deployment_mode = "SINGLE_INSTANCE"

  subnet_ids         = [aws_subnet.private_1.id]
  security_groups    = [aws_security_group.rabbitmq.id]
  publicly_accessible = false

  user {
    username = var.rabbitmq_username
    password = var.rabbitmq_password
  }

  tags = { Name = "${var.project_name}-rabbitmq" }
}
