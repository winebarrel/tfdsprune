data "aws_ami" "used" {
  most_recent = true
  owners      = ["amazon"]
}
data "aws_vpc" "unused" {
  default = true
}
data "aws_subnet" "also_unused" {
  vpc_id = "vpc-123"
}
resource "aws_instance" "web" {
  ami = data.aws_ami.used.id
}
