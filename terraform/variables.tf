variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "project_name" {
  description = "Name prefix for all resources"
  type        = string
  default     = "flash-sale-system"
}

variable "db_username" {
  description = "Master username for RDS instances"
  type        = string
  default     = "flashsale"
  sensitive   = true
}

variable "db_password" {
  description = "Master password for RDS instances"
  type        = string
  sensitive   = true
}

variable "rabbitmq_username" {
  description = "RabbitMQ broker username"
  type        = string
  default     = "flashsale"
  sensitive   = true
}

variable "rabbitmq_password" {
  description = "RabbitMQ broker password"
  type        = string
  sensitive   = true
}

variable "api_gateway_image" {
  description = "ECR image URI for the API Gateway service"
  type        = string
}

variable "order_service_image" {
  description = "ECR image URI for the Order Service"
  type        = string
}

variable "inventory_service_image" {
  description = "ECR image URI for the Inventory Service"
  type        = string
}

variable "notification_service_image" {
  description = "ECR image URI for the Notification Service"
  type        = string
}

variable "inventory_backend" {
  description = "Inventory storage backend: 'redis' or 'postgres'"
  type        = string
  default     = "postgres"

  validation {
    condition     = contains(["redis", "postgres"], var.inventory_backend)
    error_message = "inventory_backend must be 'redis' or 'postgres'."
  }
}
