#!/bin/bash
set -euo pipefail

PROJECT="flash-sale-system"
REGION=$(aws configure get region)
ACCOUNT=$(aws sts get-caller-identity --query Account --output text)
CLUSTER="${PROJECT}-cluster"
TAG=$(git rev-parse --short HEAD)

aws ecr get-login-password --region "$REGION" \
  | docker login --username AWS --password-stdin \
      "$ACCOUNT.dkr.ecr.$REGION.amazonaws.com"

for SERVICE in api-gateway order-service inventory-service notification-service; do
  REPO="$ACCOUNT.dkr.ecr.$REGION.amazonaws.com/$PROJECT/$SERVICE"
  IMAGE="$REPO:$TAG"

  echo "==> Building $SERVICE ($TAG)"
  docker build --platform linux/amd64 -t "$IMAGE" -t "$REPO:latest" "services/$SERVICE"
  docker push "$IMAGE"
  docker push "$REPO:latest"

  TASK_FAMILY="${PROJECT}-${SERVICE}"

  # Read the current task definition and swap in the new image
  CURRENT=$(aws ecs describe-task-definition \
    --task-definition "$TASK_FAMILY" \
    --query taskDefinition)

  NEW=$(echo "$CURRENT" | jq \
    --arg img "$IMAGE" \
    '.containerDefinitions[0].image = $img
     | del(.taskDefinitionArn, .revision, .status,
           .requiresAttributes, .compatibilities,
           .registeredAt, .registeredBy)')

  NEW_ARN=$(aws ecs register-task-definition \
    --cli-input-json "$NEW" \
    --query 'taskDefinition.taskDefinitionArn' \
    --output text)

  aws ecs update-service \
    --cluster "$CLUSTER" \
    --service "${PROJECT}-${SERVICE}" \
    --task-definition "$NEW_ARN" \
    --force-new-deployment \
    --output json > /dev/null

  echo "    deployed: $NEW_ARN"
done
