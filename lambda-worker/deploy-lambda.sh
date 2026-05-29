#!/usr/bin/env bash
# deploy-lambda.sh — builds the Go binary for Lambda (Linux/amd64), packages
# it with config files, and creates or updates the Lambda function.
#
# Usage:
#   ./deploy-lambda.sh <function-name> <execution-role-arn> [region]
#
#   function-name:        Name for the Lambda function (e.g. serverless-demo-worker)
#   execution-role-arn:   ARN of the execution role (output of cfn/execution-role.yaml)
#   region:               AWS region (default: us-east-1)
#
# On first run, this script creates the Lambda function.
# On subsequent runs, it updates the function code only.
#
# Prerequisites:
#   - AWS CLI configured with <your-profile> profile
#     (see README — use `access account --aws-account-id 429214323166 --write`)
#   - Temporal Cloud mTLS certs stored in AWS Secrets Manager at:
#       temporal/serverless-demo/client-cert  (PEM)
#       temporal/serverless-demo/client-key   (PEM)
#   - temporal.toml updated with your namespace address

set -euo pipefail

FUNCTION_NAME="${1:?Usage: $0 <function-name> <execution-role-arn> [region]}"
EXECUTION_ROLE_ARN="${2:?Usage: $0 <function-name> <execution-role-arn> [region]}"
REGION="${3:-us-east-1}"
AWS_PROFILE="${AWS_PROFILE:-<your-profile>}"
BUILD_DIR="$(mktemp -d)"
ZIP_PATH="${BUILD_DIR}/lambda.zip"

echo "Building for linux/amd64..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -o "${BUILD_DIR}/bootstrap" .

echo "Packaging..."
cp temporal.toml "${BUILD_DIR}/temporal.toml"
cd "${BUILD_DIR}"
zip -j "${ZIP_PATH}" bootstrap temporal.toml
cd - > /dev/null

# Check whether the function already exists
FUNCTION_EXISTS=false
if aws lambda get-function \
    --function-name "${FUNCTION_NAME}" \
    --region "${REGION}" \
    --profile "${AWS_PROFILE}" \
    > /dev/null 2>&1; then
  FUNCTION_EXISTS=true
fi

if [ "${FUNCTION_EXISTS}" = "false" ]; then
  echo "Creating Lambda function: ${FUNCTION_NAME}..."
  FUNCTION_ARN=$(aws lambda create-function \
    --function-name "${FUNCTION_NAME}" \
    --runtime provided.al2023 \
    --handler bootstrap \
    --role "${EXECUTION_ROLE_ARN}" \
    --zip-file "fileb://${ZIP_PATH}" \
    --timeout 600 \
    --memory-size 256 \
    --region "${REGION}" \
    --profile "${AWS_PROFILE}" \
    --environment "Variables={WORKER_MAX_CONCURRENT_ACTIVITIES=5,WORKER_MAX_CONCURRENT_WORKFLOWS=5}" \
    --query 'FunctionArn' \
    --output text)
  echo ""
  echo "Lambda function created."
  echo "  Function ARN: ${FUNCTION_ARN}"
  echo ""
  echo "Next step: create the Temporal invocation role with:"
  echo "  aws cloudformation create-stack \\"
  echo "    --stack-name temporal-invoke-role \\"
  echo "    --template-body file://cfn/temporal-invoke-role.yaml \\"
  echo "    --parameters \\"
  echo "      ParameterKey=FunctionName,ParameterValue=${FUNCTION_NAME} \\"
  echo "      ParameterKey=FunctionArn,ParameterValue=${FUNCTION_ARN} \\"
  echo "      ParameterKey=ExternalId,ParameterValue=<external-id-from-temporal-cloud> \\"
  echo "      ParameterKey=TemporalCloudAwsAccountId,ParameterValue=<temporal-cloud-aws-account-id> \\"
  echo "    --capabilities CAPABILITY_NAMED_IAM \\"
  echo "    --region ${REGION} \\"
  echo "    --profile ${AWS_PROFILE}"
else
  echo "Updating Lambda function code: ${FUNCTION_NAME}..."
  aws lambda update-function-code \
    --function-name "${FUNCTION_NAME}" \
    --zip-file "fileb://${ZIP_PATH}" \
    --region "${REGION}" \
    --profile "${AWS_PROFILE}" \
    > /dev/null
  echo "Function code updated."
fi

echo "Done."
rm -rf "${BUILD_DIR}"
