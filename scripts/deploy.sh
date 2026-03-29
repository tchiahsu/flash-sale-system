#!/bin/bash
ACCOUNT="835637956575"
REGION="us-east-1"
CLUSTER="flash-sale-system-cluster"
PROJECT="flash-sale-system"

aws ecr get-login-password --region $REGION | docker login --username AWS --password-stdin $ACCOUNT.dkr.ecr.$REGION.amazonaws.com

for SERVICE in api-gateway order-service inventory-service notification-service; do
  IMAGE="$ACCOUNT.dkr.ecr.$REGION.amazonaws.com/$PROJECT/$SERVICE:latest"
  docker build -t $IMAGE services/$SERVICE
  docker push $IMAGE
  aws ecs update-service --cluster $CLUSTER --service $PROJECT-$SERVICE --force-new-deployment
done