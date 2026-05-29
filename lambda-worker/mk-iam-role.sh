#!/usr/bin/env bash
# mk-iam-role.sh — creates the IAM role that Temporal Cloud assumes to invoke
# this Lambda function. Run once during initial setup.
#
# Usage: ./mk-iam-role.sh <stack-name> <external-id> <lambda-arn>
#   stack-name:  a unique name for the CloudFormation stack (e.g. "serverless-demo-role")
#   external-id: the External ID shown in your Temporal Cloud namespace serverless config
#   lambda-arn:  the ARN of your Lambda function

set -euo pipefail

STACK_NAME="${1:?Usage: $0 <stack-name> <external-id> <lambda-arn>}"
EXTERNAL_ID="${2:?Usage: $0 <stack-name> <external-id> <lambda-arn>}"
LAMBDA_ARN="${3:?Usage: $0 <stack-name> <external-id> <lambda-arn>}"

# Temporal Cloud's AWS account ID — this is who assumes the role.
TEMPORAL_AWS_ACCOUNT_ID="<your-account-id>" # TODO: replace with Temporal Cloud's actual account ID

cat > /tmp/trust-policy.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::${TEMPORAL_AWS_ACCOUNT_ID}:root"
      },
      "Action": "sts:AssumeRole",
      "Condition": {
        "StringEquals": {
          "sts:ExternalId": "${EXTERNAL_ID}"
        }
      }
    }
  ]
}
EOF

ROLE_NAME="${STACK_NAME}-temporal-invoke-role"

echo "Creating IAM role: ${ROLE_NAME}"
ROLE_ARN=$(aws iam create-role \
  --role-name "${ROLE_NAME}" \
  --assume-role-policy-document file:///tmp/trust-policy.json \
  --query 'Role.Arn' \
  --output text)

echo "Attaching Lambda invoke permission..."
aws iam put-role-policy \
  --role-name "${ROLE_NAME}" \
  --policy-name "InvokeLambda" \
  --policy-document "{
    \"Version\": \"2012-10-17\",
    \"Statement\": [{
      \"Effect\": \"Allow\",
      \"Action\": \"lambda:InvokeFunction\",
      \"Resource\": \"${LAMBDA_ARN}\"
    }]
  }"

echo ""
echo "Done! Role ARN to configure in Temporal Cloud:"
echo "  ${ROLE_ARN}"
