resource "aws_elasticache_subnet_group" "main" {
  name = "${var.project_name}-redis-subnet-group"
  subnet_ids = [
    aws_subnet.private_1.id,
    aws_subnet.private_2.id,
  ]

  tags = { Name = "${var.project_name}-redis-subnet-group" }
}

resource "aws_elasticache_cluster" "inventory" {
  cluster_id           = "${var.project_name}-inventory"
  engine               = "redis"
  node_type            = "cache.t3.micro"
  num_cache_nodes      = 1
  parameter_group_name = "default.redis7"
  engine_version       = "7.1"
  port                 = 6379

  subnet_group_name  = aws_elasticache_subnet_group.main.name
  security_group_ids = [aws_security_group.redis.id]

  tags = { Name = "${var.project_name}-redis" }
}
