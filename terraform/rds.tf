resource "aws_db_subnet_group" "main" {
  name = "${var.project_name}-db-subnet-group"
  subnet_ids = [
    aws_subnet.private_1.id,
    aws_subnet.private_2.id,
  ]

  tags = { Name = "${var.project_name}-db-subnet-group" }
}

# Orders DB — owned by the Order Service
resource "aws_db_instance" "orders" {
  identifier     = "${var.project_name}-orders"
  engine         = "postgres"
  engine_version = "15.7"
  instance_class = "db.t3.micro"

  allocated_storage = 20

  db_name  = "orders"
  username = var.db_username
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  publicly_accessible = false
  skip_final_snapshot = true

  tags = { Name = "${var.project_name}-orders-db" }
}

# Inventory DB — used by Inventory Service when inventory_backend = "postgres"
resource "aws_db_instance" "inventory" {
  identifier     = "${var.project_name}-inventory"
  engine         = "postgres"
  engine_version = "15.7"
  instance_class = "db.t3.micro"

  allocated_storage = 20

  db_name  = "inventory"
  username = var.db_username
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  publicly_accessible = false
  skip_final_snapshot = true

  tags = { Name = "${var.project_name}-inventory-db" }
}
