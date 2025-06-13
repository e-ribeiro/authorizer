# Configuração da infraestrutura AWS para o Sistema de Autorização de Transações
# Implementa a arquitetura serverless completa com observabilidade

terraform {
  required_version = ">= 1.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

# Variáveis de configuração
variable "aws_region" {
  description = "Região AWS para deploy"
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Ambiente (dev, staging, prod)"
  type        = string
  default     = "dev"
}

variable "project_name" {
  description = "Nome do projeto"
  type        = string
  default     = "itau-authorizer"
}

# Tags padrão para todos os recursos
locals {
  common_tags = {
    Project     = var.project_name
    Environment = var.environment
    Terraform   = "true"
    Team        = "payments"
  }
}

# === DynamoDB Tables ===

# Tabela de Clientes com limites de crédito
resource "aws_dynamodb_table" "clientes" {
  name           = "${var.project_name}-clientes-${var.environment}"
  billing_mode   = "PAY_PER_REQUEST"
  hash_key       = "id"

  attribute {
    name = "id"
    type = "S"
  }

  # Global Secondary Index para queries por email
  global_secondary_index {
    name            = "email-index"
    hash_key        = "email"
    projection_type = "ALL"
  }

  attribute {
    name = "email"
    type = "S"
  }

  # Configuração de backup
  point_in_time_recovery {
    enabled = true
  }

  # Encryption at rest
  server_side_encryption {
    enabled = true
  }

  tags = local.common_tags
}

# Tabela de Transações com TTL para cleanup automático
resource "aws_dynamodb_table" "transacoes" {
  name           = "${var.project_name}-transacoes-${var.environment}"
  billing_mode   = "PAY_PER_REQUEST"
  hash_key       = "id"

  attribute {
    name = "id"
    type = "S"
  }

  # Global Secondary Index para queries por cliente
  global_secondary_index {
    name            = "cliente-id-index"
    hash_key        = "cliente_id"
    range_key       = "timestamp"
    projection_type = "ALL"
  }

  attribute {
    name = "cliente_id"
    type = "S"
  }

  attribute {
    name = "timestamp"
    type = "S"
  }

  # TTL para limpeza automática de dados antigos (90 dias)
  ttl {
    attribute_name = "ttl"
    enabled        = true
  }

  point_in_time_recovery {
    enabled = true
  }

  server_side_encryption {
    enabled = true
  }

  tags = local.common_tags
}

# === SNS Topic e SQS Queues ===

# Tópico SNS principal para eventos de transação
resource "aws_sns_topic" "transacoes" {
  name = "${var.project_name}-transacoes-${var.environment}"

  # Encryption at rest
  kms_master_key_id = "alias/aws/sns"

  tags = local.common_tags
}

# SQS Queue para sistema de Faturamento
resource "aws_sqs_queue" "faturamento" {
  name = "${var.project_name}-faturamento-${var.environment}"

  # Dead letter queue configuration
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.faturamento_dlq.arn
    maxReceiveCount     = 3
  })

  # Encryption
  kms_master_key_id = "alias/aws/sqs"

  tags = local.common_tags
}

resource "aws_sqs_queue" "faturamento_dlq" {
  name = "${var.project_name}-faturamento-dlq-${var.environment}"

  tags = local.common_tags
}

# SQS Queue para sistema de Notificações
resource "aws_sqs_queue" "notificacoes" {
  name = "${var.project_name}-notificacoes-${var.environment}"

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.notificacoes_dlq.arn
    maxReceiveCount     = 5
  })

  kms_master_key_id = "alias/aws/sqs"

  tags = local.common_tags
}

resource "aws_sqs_queue" "notificacoes_dlq" {
  name = "${var.project_name}-notificacoes-dlq-${var.environment}"

  tags = local.common_tags
}

# SQS Queue para sistema de Pontos
resource "aws_sqs_queue" "pontos" {
  name = "${var.project_name}-pontos-${var.environment}"

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.pontos_dlq.arn
    maxReceiveCount     = 3
  })

  kms_master_key_id = "alias/aws/sqs"

  tags = local.common_tags
}

resource "aws_sqs_queue" "pontos_dlq" {
  name = "${var.project_name}-pontos-dlq-${var.environment}"

  tags = local.common_tags
}

# Subscriptions SNS -> SQS
resource "aws_sns_topic_subscription" "faturamento" {
  topic_arn = aws_sns_topic.transacoes.arn
  protocol  = "sqs"
  endpoint  = aws_sqs_queue.faturamento.arn

  filter_policy = jsonencode({
    event_type = ["transaction.approved"]
  })
}

resource "aws_sns_topic_subscription" "notificacoes" {
  topic_arn = aws_sns_topic.transacoes.arn
  protocol  = "sqs"
  endpoint  = aws_sqs_queue.notificacoes.arn

  filter_policy = jsonencode({
    event_type = ["transaction.approved", "transaction.rejected"]
  })
}

resource "aws_sns_topic_subscription" "pontos" {
  topic_arn = aws_sns_topic.transacoes.arn
  protocol  = "sqs"
  endpoint  = aws_sqs_queue.pontos.arn

  filter_policy = jsonencode({
    event_type = ["transaction.approved"]
  })
}

# === IAM Roles e Policies ===

# IAM Role para Lambda
resource "aws_iam_role" "lambda_role" {
  name = "${var.project_name}-lambda-role-${var.environment}"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })

  tags = local.common_tags
}

# Policy para DynamoDB
resource "aws_iam_policy" "dynamodb_policy" {
  name = "${var.project_name}-dynamodb-policy-${var.environment}"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "dynamodb:GetItem",
          "dynamodb:PutItem",
          "dynamodb:UpdateItem",
          "dynamodb:Query",
          "dynamodb:Scan"
        ]
        Resource = [
          aws_dynamodb_table.clientes.arn,
          aws_dynamodb_table.transacoes.arn,
          "${aws_dynamodb_table.clientes.arn}/index/*",
          "${aws_dynamodb_table.transacoes.arn}/index/*"
        ]
      }
    ]
  })
}

# Policy para SNS
resource "aws_iam_policy" "sns_policy" {
  name = "${var.project_name}-sns-policy-${var.environment}"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "sns:Publish"
        ]
        Resource = aws_sns_topic.transacoes.arn
      }
    ]
  })
}

# Attach policies to Lambda role
resource "aws_iam_role_policy_attachment" "lambda_basic" {
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
  role       = aws_iam_role.lambda_role.name
}

resource "aws_iam_role_policy_attachment" "lambda_xray" {
  policy_arn = "arn:aws:iam::aws:policy/AWSXRayDaemonWriteAccess"
  role       = aws_iam_role.lambda_role.name
}

resource "aws_iam_role_policy_attachment" "lambda_dynamodb" {
  policy_arn = aws_iam_policy.dynamodb_policy.arn
  role       = aws_iam_role.lambda_role.name
}

resource "aws_iam_role_policy_attachment" "lambda_sns" {
  policy_arn = aws_iam_policy.sns_policy.arn
  role       = aws_iam_role.lambda_role.name
}

# === Lambda Function ===

resource "aws_lambda_function" "authorizer" {
  filename         = "../bootstrap.zip"
  function_name    = "${var.project_name}-${var.environment}"
  role            = aws_iam_role.lambda_role.arn
  handler         = "bootstrap"
  runtime         = "provided.al2"
  timeout         = 30
  memory_size     = 256

  # Environment variables
  environment {
    variables = {
      CLIENTES_TABLE_NAME    = aws_dynamodb_table.clientes.name
      TRANSACOES_TABLE_NAME  = aws_dynamodb_table.transacoes.name
      SNS_TOPIC_ARN          = aws_sns_topic.transacoes.arn
      ENVIRONMENT            = var.environment
    }
  }

  # X-Ray tracing
  tracing_config {
    mode = "Active"
  }

  tags = local.common_tags
}

# CloudWatch Log Group para Lambda
resource "aws_cloudwatch_log_group" "lambda_logs" {
  name              = "/aws/lambda/${aws_lambda_function.authorizer.function_name}"
  retention_in_days = 30

  tags = local.common_tags
}

# === API Gateway ===

resource "aws_api_gateway_rest_api" "main" {
  name = "${var.project_name}-api-${var.environment}"

  endpoint_configuration {
    types = ["REGIONAL"]
  }

  tags = local.common_tags
}

# Resource: /transacoes
resource "aws_api_gateway_resource" "transacoes" {
  rest_api_id = aws_api_gateway_rest_api.main.id
  parent_id   = aws_api_gateway_rest_api.main.root_resource_id
  path_part   = "transacoes"
}

# Method: POST /transacoes
resource "aws_api_gateway_method" "post_transacoes" {
  rest_api_id   = aws_api_gateway_rest_api.main.id
  resource_id   = aws_api_gateway_resource.transacoes.id
  http_method   = "POST"
  authorization = "NONE"
}

# Integration: Lambda
resource "aws_api_gateway_integration" "lambda_integration" {
  rest_api_id = aws_api_gateway_rest_api.main.id
  resource_id = aws_api_gateway_resource.transacoes.id
  http_method = aws_api_gateway_method.post_transacoes.http_method

  integration_http_method = "POST"
  type                   = "AWS_PROXY"
  uri                    = aws_lambda_function.authorizer.invoke_arn
}

# Lambda permission for API Gateway
resource "aws_lambda_permission" "api_gateway" {
  statement_id  = "AllowExecutionFromAPIGateway"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.authorizer.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_api_gateway_rest_api.main.execution_arn}/*/*"
}

# Deployment
resource "aws_api_gateway_deployment" "main" {
  depends_on = [
    aws_api_gateway_integration.lambda_integration
  ]

  rest_api_id = aws_api_gateway_rest_api.main.id
  stage_name  = var.environment

  variables = {
    deployed_at = timestamp()
  }
}

# === CloudWatch Dashboards e Alarms ===

resource "aws_cloudwatch_dashboard" "main" {
  dashboard_name = "${var.project_name}-${var.environment}"

  dashboard_body = jsonencode({
    widgets = [
      {
        type   = "metric"
        width  = 12
        height = 6
        properties = {
          metrics = [
            ["AWS/Lambda", "Duration", "FunctionName", aws_lambda_function.authorizer.function_name],
            ["AWS/Lambda", "Errors", "FunctionName", aws_lambda_function.authorizer.function_name],
            ["AWS/Lambda", "Invocations", "FunctionName", aws_lambda_function.authorizer.function_name]
          ]
          period = 300
          stat   = "Average"
          region = var.aws_region
          title  = "Lambda Metrics"
        }
      },
      {
        type   = "metric"
        width  = 12
        height = 6
        properties = {
          metrics = [
            ["AWS/DynamoDB", "ConsumedReadCapacityUnits", "TableName", aws_dynamodb_table.clientes.name],
            ["AWS/DynamoDB", "ConsumedWriteCapacityUnits", "TableName", aws_dynamodb_table.clientes.name]
          ]
          period = 300
          stat   = "Sum"
          region = var.aws_region
          title  = "DynamoDB Capacity"
        }
      }
    ]
  })
}

# Alarm para alta latência
resource "aws_cloudwatch_metric_alarm" "high_latency" {
  alarm_name          = "${var.project_name}-high-latency-${var.environment}"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "2"
  metric_name         = "Duration"
  namespace           = "AWS/Lambda"
  period              = "60"
  statistic           = "Average"
  threshold           = "5000"  # 5 segundos
  alarm_description   = "This metric monitors lambda duration"

  dimensions = {
    FunctionName = aws_lambda_function.authorizer.function_name
  }

  tags = local.common_tags
}

# === Outputs ===

output "api_gateway_url" {
  description = "URL da API Gateway"
  value       = "https://${aws_api_gateway_rest_api.main.id}.execute-api.${var.aws_region}.amazonaws.com/${var.environment}"
}

output "dynamodb_tables" {
  description = "Nomes das tabelas DynamoDB"
  value = {
    clientes   = aws_dynamodb_table.clientes.name
    transacoes = aws_dynamodb_table.transacoes.name
  }
}

output "sns_topic_arn" {
  description = "ARN do tópico SNS"
  value       = aws_sns_topic.transacoes.arn
}

output "lambda_function_name" {
  description = "Nome da função Lambda"
  value       = aws_lambda_function.authorizer.function_name
} 