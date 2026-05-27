data "aws_ami" "used" {
  most_recent = true
}
data "aws_vpc" "unused" {
  default = true
}
