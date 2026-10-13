#!/usr/bin/env bash
set -euo pipefail

echo "==> [LocalStack Init] Deploying SQS FIFO Infrastructure via CloudFormation..."

AWS_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
STACK_NAME="jungle-messaging-stack"
TEMPLATE_PATH="/etc/localstack/init/ready.d/sqs-template.yaml"

# Deploy declarative CloudFormation stack
awslocal cloudformation deploy \
  --region "${AWS_REGION}" \
  --stack-name "${STACK_NAME}" \
  --template-file "${TEMPLATE_PATH}"

echo "==> [LocalStack Init] CloudFormation stack '${STACK_NAME}' successfully deployed!"
echo "--> Provisioned SQS Queues:"
awslocal sqs list-queues --region "${AWS_REGION}"
