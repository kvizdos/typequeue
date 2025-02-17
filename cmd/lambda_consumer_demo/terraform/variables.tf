variable "aws_region" {
  type    = string
  default = "us-west-2"
}

variable "aws_account_id" {
  type      = string
  sensitive = true
}

variable "aws_profile" {
  type      = string
  sensitive = true

}
