output "alb_dns_name" {
  description = "Point Locust at this URL"
  value       = aws_lb.main.dns_name
}

output "ecr_repository_urls" {
  description = "Push images to these URIs before deploying"
  value       = { for k, v in aws_ecr_repository.services : k => v.repository_url }
}

output "orders_db_endpoint" {
  description = "Orders RDS endpoint"
  value       = aws_db_instance.orders.address
  sensitive   = true
}

output "inventory_db_endpoint" {
  description = "Inventory RDS endpoint (Postgres backend)"
  value       = aws_db_instance.inventory.address
  sensitive   = true
}

output "redis_endpoint" {
  description = "ElastiCache Redis endpoint (Redis backend)"
  value       = aws_elasticache_cluster.inventory.cache_nodes[0].address
}

output "rabbitmq_endpoint" {
  description = "AmazonMQ AMQP endpoint"
  value       = aws_mq_broker.rabbitmq.instances[0].endpoints[0]
}
