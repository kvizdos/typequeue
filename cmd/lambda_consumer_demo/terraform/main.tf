provider "aws" {
  region  = var.aws_region
  profile = var.aws_profile
}

resource "aws_sqs_queue" "typequeue_lambda_demo_dlq" {
  name = "typequeue-lambda-demo-dlq"
}

resource "aws_sqs_queue" "typequeue_lambda_demo" {
  name = "typequeue-lambda-demo"

  visibility_timeout_seconds = 30

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.typequeue_lambda_demo_dlq.arn,
    maxReceiveCount     = 2
  })
}

resource "aws_iam_role" "lambda_role" {
  name = "lambda_execution_role"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "lambda.amazonaws.com" }
    }]
  })
}

resource "aws_iam_policy" "lambda_policy" {
  name        = "lambda_policy"
  description = "Policy for lambda execution and SQS access"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:*:*:*"
      },
      {
        Effect = "Allow"
        Action = [
          "sqs:ReceiveMessage",
          "sqs:DeleteMessage",
          "sqs:GetQueueAttributes"
        ]
        Resource = aws_sqs_queue.typequeue_lambda_demo.arn
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "lambda_policy_attachment" {
  role       = aws_iam_role.lambda_role.name
  policy_arn = aws_iam_policy.lambda_policy.arn
}

resource "aws_lambda_function" "typequeue_lambda" {
  function_name = "typequeue_lambda_demo"
  handler       = "bootstrap"
  runtime       = "provided.al2023"
  architectures = ["arm64"]
  role          = aws_iam_role.lambda_role.arn

  # Ensure your build output is packaged as function.zip in your working directory.
  filename         = "bootstrap.zip"
  source_code_hash = filebase64sha256("bootstrap.zip")
}

resource "aws_lambda_event_source_mapping" "sqs_trigger" {
  event_source_arn = aws_sqs_queue.typequeue_lambda_demo.arn
  function_name    = aws_lambda_function.typequeue_lambda.arn
  enabled          = true
  batch_size       = 10

  function_response_types = ["ReportBatchItemFailures"] # SUPER IMPORTANT
}
