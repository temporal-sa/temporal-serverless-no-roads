.PHONY: build deploy cfn-execution-role

FUNCTION_NAME    ?= serverless-webinar-worker
EXECUTION_ROLE   ?= $(error Set EXECUTION_ROLE to the ARN from cfn-execution-role)
REGION           ?= us-east-1
AWS_PROFILE      ?= <your-profile>

CFN_STACK_EXEC   ?= temporal-webinar-execution-role

# Step 1: build the binary only
build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap .

# Step 2: deploy CloudFormation stack for the Lambda execution role
# Creates: <function-name>-execution-role IAM role
cfn-execution-role:
	aws cloudformation deploy \
	  --stack-name $(CFN_STACK_EXEC) \
	  --template-file cfn/execution-role.yaml \
	  --parameter-overrides FunctionName=$(FUNCTION_NAME) \
	  --capabilities CAPABILITY_NAMED_IAM \
	  --region $(REGION) \
	  --profile $(AWS_PROFILE)
	@echo ""
	@echo "Execution role ARN:"
	@aws cloudformation describe-stacks \
	  --stack-name $(CFN_STACK_EXEC) \
	  --query 'Stacks[0].Outputs[?OutputKey==`RoleArn`].OutputValue' \
	  --output text \
	  --region $(REGION) \
	  --profile $(AWS_PROFILE)

# Step 3: create or update the Lambda function and build/package code.
# Set EXECUTION_ROLE to the ARN output from cfn-execution-role.
#
# After this, complete setup in the Temporal Cloud UI:
# Workers → Serverless → Create Worker Deployment
# The UI provides a CloudFormation template for the invocation role.
deploy:
	./deploy-lambda.sh $(FUNCTION_NAME) $(EXECUTION_ROLE) $(REGION)
